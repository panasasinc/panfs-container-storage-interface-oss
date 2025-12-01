#!/bin/bash

export GREEN="\033[32m"
export RESET="\033[0m"
export YELLOW="\033[33m"
export RED="\033[31m"
export CYAN="\033[36m"
export GRAY="\033[90m"
export BOLD="\033[1m"

trap 'echo -e "\n${RED}Interrupted. Exiting.${RESET}"; exit 130' SIGINT SIGTERM

print() {
    printf "%b\n" "$1"
}

rollout_status() {
    NAMESPACE=csi-panfs
    resource_type="$1" # "deployment" or "daemonset"
    resource_name="$2"  

    FAIL_IMAGE='(ImagePullBackOff|ErrImagePull|InvalidImageName|RegistryUnavailable|Init:ImagePullBackOff|Init:ErrImagePull)'
    FAIL_CRASH='(CrashLoopBackOff|Init:CrashLoopBackOff)'
    FAIL_CONFIG='(CreateContainerConfigError|CreateContainerError|CreatePodSandboxError|Init:Error|Error|ContainerCannotRun|RunContainerError|StartError)'
    FAIL_SCHEDULING='(FailedScheduling|Unschedulable)'
    FAIL_VOLUME='(FailedMount|FailedAttachVolume)'
    FAIL_CREATE='(FailedCreate|DeadlineExceeded)'

    ALL_FAIL="(${FAIL_IMAGE}|${FAIL_CRASH}|${FAIL_CONFIG}|${FAIL_SCHEDULING}|${FAIL_VOLUME}|${FAIL_CREATE})"

    while true; do
        pods=$(kubectl -n "$NAMESPACE" get pods -l "app=${resource_name}" --no-headers 2>/dev/null)

        if echo "$pods" | grep -Eq "$ALL_FAIL"; then
            print ""
            if echo "$pods" | grep -Eq "$FAIL_IMAGE"; then
                print "${RED}ERROR: ${resource_type} ${resource_name} has pods with image pull errors. Please check the image name and registry access.${RESET}"
            fi
            if echo "$pods" | grep -Eq "$FAIL_CRASH"; then
                print "${RED}ERROR: ${resource_type} ${resource_name} has pods in CrashLoopBackOff state. Please check the pod logs for more details.${RESET}"
            fi
            if echo "$pods" | grep -Eq "$FAIL_CONFIG"; then
                print "${RED}ERROR: ${resource_type} ${resource_name} has pods with container configuration errors. Please check the pod logs for more details.${RESET}"
            fi
            if echo "$pods" | grep -Eq "$FAIL_SCHEDULING"; then
                print "${RED}ERROR: ${resource_type} ${resource_name} has pods with scheduling issues. Please check node resources and taints.${RESET}"
            fi
            if echo "$pods" | grep -Eq "$FAIL_VOLUME"; then
                print "${RED}ERROR: ${resource_type} ${resource_name} has pods with volume attachment/mount issues. Please check the volume status.${RESET}"
            fi
            if echo "$pods" | grep -Eq "$FAIL_CREATE"; then
                print "${RED}ERROR: ${resource_type} ${resource_name} has pods with creation errors. Please check the pod logs for more details.${RESET}"
            fi

            print "Affected pods:"
            kubectl -n "$NAMESPACE" get pods -l "app=${resource_name}"
            exit 1
        fi

        # Check if rollout completed
        if kubectl -n $NAMESPACE rollout status "${resource_type}/${resource_name}" --timeout=10s 2>/dev/null; then
            print "\n${GREEN}✔ ${resource_type} ${resource_name} is successfully rolled out.${RESET}\n"
            print "${resource_type} ${resource_name} status:"
            kubectl -n $NAMESPACE get "${resource_type}" "${resource_name}" -o wide
            echo
            break
        fi
        sleep 5
    done
}


print "${BOLD}Waiting for the PanFS CSI Controller deployment to be ready...${RESET}"
rollout_status "deployment" "csi-panfs-controller"

if kubectl get module panfs -n csi-panfs >/dev/null 2>&1; then
    print "\n${BOLD}Waiting for PanFS module loader to converge...${RESET}"

    availableNumber="-1"
    nodesMatchingSelectorNumber="-2"

    while [ "$availableNumber" != "$nodesMatchingSelectorNumber" ]; do
        nodesMatchingSelectorNumber=$(kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.nodesMatchingSelectorNumber}')
        availableNumber=$(kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.availableNumber}')

        nodesMatchingSelectorNumber=${nodesMatchingSelectorNumber:-0}
        availableNumber=${availableNumber:-0}

        if [ "$nodesMatchingSelectorNumber" -eq 0 ]; then
            EXPECTED_LABEL=$(kubectl get module panfs -n csi-panfs -o jsonpath='{.spec.selector}' | jq -r 'to_entries[] | .key + "=" + .value')
                print "${RED}ERROR: No nodes match the PanFS Module selector (${EXPECTED_LABEL}).${RESET}"

            NODE_COUNT=$(kubectl get nodes -l "${EXPECTED_LABEL}" --no-headers 2>/dev/null | wc -l)

            if [ "$NODE_COUNT" -eq 0 ]; then
                print "${RED}Action required: Please ensure that your worker nodes have the label: '${EXPECTED_LABEL}'.${RESET}"
            else
                print "${RED}Nodes matching the selector exist, but the module loader has not yet recognized them or the KMM controller is unhealthy.${RESET}"
            fi
            exit 1
        fi

        print "Waiting... $availableNumber/$nodesMatchingSelectorNumber"
        if [ "$availableNumber" = "$nodesMatchingSelectorNumber" ]; then
            break
        fi

        sleep 5
    done

    kubectl get module panfs -n csi-panfs -o yaml | sed -n '/^status:/,$p' | sed 's/status:/Kernel module status:/'
else
    print "\n${CYAN}KMM module 'panfs' not found. Skipping KMM readiness check.${RESET}"
fi

print "\n${BOLD}Waiting for the PanFS CSI Node daemonset to be ready...${RESET}"
rollout_status "daemonset" "csi-panfs-node"
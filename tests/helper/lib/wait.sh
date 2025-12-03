#!/bin/bash

export GREEN="\033[32m"
export RESET="\033[0m"
export YELLOW="\033[33m"
export RED="\033[31m"
export CYAN="\033[36m"
export GRAY="\033[90m"
export BOLD="\033[1m"

trap 'echo -e "\n${RED}Interrupted. Exiting.${RESET}"; exit 130' SIGINT SIGTERM

# Output formatted message
print() {
    printf "%b\n" "$1"
}

NAMESPACE=csi-panfs

FAIL_IMAGE='(ImagePullBackOff|ErrImagePull|InvalidImageName|RegistryUnavailable|Init:ImagePullBackOff|Init:ErrImagePull)'
FAIL_CRASH='(CrashLoopBackOff|Init:CrashLoopBackOff)'
FAIL_CONFIG='(CreateContainerConfigError|CreateContainerError|CreatePodSandboxError|Init:Error|Error|ContainerCannotRun|RunContainerError|StartError)'
FAIL_SCHEDULING='(FailedScheduling|Unschedulable)'
FAIL_VOLUME='(FailedMount|FailedAttachVolume)'
FAIL_CREATE='(FailedCreate|DeadlineExceeded)'
ALL_FAIL="(${FAIL_IMAGE}|${FAIL_CRASH}|${FAIL_CONFIG}|${FAIL_SCHEDULING}|${FAIL_VOLUME}|${FAIL_CREATE})"

# Check the status of pods with the given label
pod_status() {
    label="$1"

    pods=$(kubectl -n "$NAMESPACE" get pods -l "$label" --no-headers 2>/dev/null)

    # If no failures at all — exit early
    if ! echo "$pods" | grep -Eq "$ALL_FAIL"; then
        return 0
    fi

    echo

    # Loop through each failure type and use case/esac
    for fail in IMAGE CRASH CONFIG SCHEDULING VOLUME CREATE; do
        case "$fail" in
            IMAGE)
                pattern="$FAIL_IMAGE"
                msg="${RED}ERROR: There are pods with image pull errors. Please check the image name and registry access.${RESET}"
                ;;
            CRASH)
                pattern="$FAIL_CRASH"
                msg="${RED}ERROR: There are pods in CrashLoopBackOff state. Please check the pod logs for more details.${RESET}"
                ;;
            CONFIG)
                pattern="$FAIL_CONFIG"
                msg="${RED}ERROR: There are pods with container configuration errors. Please check the pod logs for more details.${RESET}"
                ;;
            SCHEDULING)
                pattern="$FAIL_SCHEDULING"
                msg="${RED}ERROR: There are pods with scheduling issues. Please check node resources and taints.${RESET}"
                ;;
            VOLUME)
                pattern="$FAIL_VOLUME"
                msg="${RED}ERROR: There are pods with volume attachment/mount issues. Please check the volume status.${RESET}"
                ;;
            CREATE)
                pattern="$FAIL_CREATE"
                msg="${RED}ERROR: There are pods with creation errors. Please check the pod logs for more details.${RESET}"
                ;;
        esac

        if echo "$pods" | grep -Eq "$pattern"; then
            print "$msg"
            break
        fi
    done

    echo
    echo "Affected pods:"
    echo "$pods"
    echo

    return 1
}

# Wait for the rollout of a deployment or daemonset to complete
rollout_status() {
    NAMESPACE=csi-panfs
    resource_type="$1" # "deployment" or "daemonset"
    resource_name="$2"  


    while true; do
        pod_status "app=${resource_name}" || exit 1

        # Check if rollout completed
        if kubectl -n $NAMESPACE rollout status "${resource_type}/${resource_name}" --timeout=10s >/dev/null 2>&1; then
            print "\n${GREEN}✔ ${resource_type} ${resource_name} is successfully rolled out.${RESET}\n"
            print "  ${resource_type^} '${resource_name}' details:"
            kubectl -n $NAMESPACE get "${resource_type}" "${resource_name}" -o wide | sed 's/^/  /'
            echo
            break
        fi
        sleep 5
    done
}

print "\n${BOLD}Waiting for the PanFS CSI Controller deployment to be ready...${RESET}"
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

        pod_status "kmm.node.kubernetes.io/image-owner=panfs" || exit 1 # KMM image pull status
        pod_status "kmm.node.kubernetes.io/module.name=panfs" || exit 1 # KMM panfs module status
        print "Waiting... $availableNumber/$nodesMatchingSelectorNumber"
        if [ "$availableNumber" = "$nodesMatchingSelectorNumber" ]; then
            break
        fi

        sleep 5
    done

    print "\n${GREEN}✔ KMM module 'panfs' is successfully loaded.${RESET}\n"
    kubectl get module panfs -n csi-panfs -o yaml | sed -n '/^status:/,$p' | sed 's/status:/Kernel module status:/' | sed 's/^/  /'
else
    print "\n${CYAN}KMM module 'panfs' not found. Skipping KMM readiness check.${RESET}"
fi

print "\n${BOLD}Waiting for the PanFS CSI Node daemonset to be ready...${RESET}"
rollout_status "daemonset" "csi-panfs-node"
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

print "${BOLD}Waiting for the PanFS CSI Controller deployment to be ready...${RESET}"
kubectl -n csi-panfs rollout status deployment csi-panfs-controller
kubectl -n csi-panfs get deploy csi-panfs-controller -o wide

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
kubectl -n csi-panfs rollout status daemonset csi-panfs-node
kubectl -n csi-panfs get ds csi-panfs-node -o wide
#!/bin/sh
# Copyright 2025 VDURA Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# Define colors for output
export GREEN="\033[32m"
export RESET="\033[0m"
export YELLOW="\033[33m"
export RED="\033[31m"
export GRAY="\033[90m"

print() {
    printf "%b\n" "$1"
}

print "Verifying PanFS CSI Driver installation..."
print

print "${GRAY}Checking PanFS CSI Driver Controller status...${RESET}"
if ! kubectl get deploy -n csi-panfs csi-panfs-controller >/dev/null 2>&1; then
    print "${RED}  ✗ PanFS CSI Driver Controller is not deployed in csi-panfs namespace${RESET}"
    print
else
    export CONTROLLER_IMAGE=$(kubectl get deploy -n csi-panfs csi-panfs-controller -o jsonpath='{.spec.template.spec.containers[?(@.name=="csi-panfs-plugin")].image}')
    if [ "$CONTROLLER_IMAGE" = "${CSIDRIVER_IMAGE}" ]; then
        print "  ✓ CSI Driver is set correctly: ${GREEN}$CONTROLLER_IMAGE${RESET}"
    else
        print "${RED}  ✗ CSI Driver image mismatch:${RESET}"
        print "${RED}      expected: ${CSIDRIVER_IMAGE}${RESET}"
        print "${RED}      found:    ${CONTROLLER_IMAGE}${RESET}"
        print
    fi

    print "      ${GRAY}Controller Leases:"
    kubectl get leases.coordination.k8s.io -n csi-panfs | sed 's|^|        |'
    print "${RESET}"
    print

    export CONTROLLER_PODS_READY=$(kubectl get deploy -n csi-panfs csi-panfs-controller -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
    export CONTROLLER_PODS=$(kubectl get deploy -n csi-panfs csi-panfs-controller -o jsonpath='{.status.replicas}')
    if [ "$CONTROLLER_PODS_READY" -eq "$CONTROLLER_PODS" ]; then
        print "  ✓ All ${GREEN}$CONTROLLER_PODS_READY/$CONTROLLER_PODS${RESET} controller pods are ready"
        print "      ${GRAY}Controller Pods:      $CONTROLLER_PODS_READY ready, $CONTROLLER_PODS total${RESET}"
        print "      ${GRAY}Controller Replicas:  $(kubectl get deploy -n csi-panfs csi-panfs-controller -o jsonpath='{.spec.replicas}')${RESET}"
    else
        print "${RED}   ✗ Not all controller pods are ready: $CONTROLLER_PODS_READY ready, $CONTROLLER_PODS total${RESET}"
    fi
    print "      ${GRAY}Controller Pods:"
    kubectl get pods -n csi-panfs -l app=csi-panfs-controller | sed 's|^|        |'
    print
    print "      ${GRAY}Details:"
    kubectl get pods -n csi-panfs -l app=csi-panfs-controller -o json | jq -r '(.items | map([.metadata.name,.spec.nodeName,.status.phase,(.spec.containers[]? |select(.name == "csi-provisioner") | .name),(.spec.containers[]? |select(.name == "csi-provisioner") | .image),(.status.containerStatuses[]? | select(.name == "csi-provisioner") | .ready),(.status.containerStatuses[]? | select(.name == "csi-provisioner") | .restartCount),(.spec.containers[]? |select(.name == "csi-attacher") | .name),(.spec.containers[]? |select(.name == "csi-attacher") | .image),(.status.containerStatuses[]? | select(.name == "csi-attacher") | .ready),(.status.containerStatuses[]? | select(.name == "csi-attacher") | .restartCount),(.spec.containers[]? |select(.name == "csi-resizer") | .name),(.spec.containers[]? |select(.name == "csi-resizer") | .image),(.status.containerStatuses[]? | select(.name == "csi-resizer") | .ready),(.status.containerStatuses[]? | select(.name == "csi-resizer") | .restartCount),(.spec.containers[]? |select(.name == "csi-panfs-plugin") | .name),(.spec.containers[]? |select(.name == "csi-panfs-plugin") | .image),(.status.containerStatuses[]? | select(.name == "csi-panfs-plugin") | .ready),(.status.containerStatuses[]? | select(.name == "csi-panfs-plugin") | .restartCount)])) as $rows| $rows| .[]| @tsv' | awk -v attacher_leader=$(kubectl get leases.coordination.k8s.io -n csi-panfs | grep attacher | awk '{print $2}') -v resizer_leader=$(kubectl get leases.coordination.k8s.io -n csi-panfs | grep resizer | awk '{print $2}') 'BEGIN {FS="\t"} {printf("%8s%s:\n", " ", $1);printf("%10sNode: %s, Pod Phase: %s\n", " ", $2, $3);printf("%10s%s:\n", " ", $4);printf("%12sImage: %s, Ready: %s, Restarts: %s\n", " ", $5, $6, $7);if ($1 == attacher_leader) {printf("%10s%s: <- leader\n", " ", $8);} else {printf("%10s%s:\n", " ", $8);}printf("%12sImage: %s, Ready: %s, Restarts: %s\n", " ", $9, $10, $11);if ($1 == resizer_leader) {printf("%10s%s: <- leader\n", " ", $12);} else {printf("%10s%s:\n", " ", $12);}printf("%12sImage: %s, Ready: %s, Restarts: %s\n", " ", $13, $14, $15);printf("%10s%s:\n", " ", $16);printf("%12sImage: %s, Ready: %s, Restarts: %s\n", " ", $17, $18, $19);printf("\n")}';\
    print "${RESET}"
fi

print "${GRAY}Checking PanFS CSI Node status...${RESET}"
if ! kubectl get ds -n csi-panfs csi-panfs-node >/dev/null 2>&1; then
    print "${RED}  ✗ PanFS CSI Node is not deployed in csi-panfs namespace${RESET}"
    print
else
    export DFC_IMAGE=$(kubectl get module -n csi-panfs panfs -o jsonpath='{.spec.moduleLoader.container.kernelMappings[?(@.literal=="'${KERNEL_VERSION}'")].containerImage}')
    export WORKER_NODES=$(kubectl get nodes -l node-role.kubernetes.io/worker= --no-headers | wc -l | xargs echo)
    export NODE_IMAGE=$(kubectl get ds -n csi-panfs csi-panfs-node -o jsonpath='{.spec.template.spec.containers[?(@.name=="csi-panfs-plugin")].image}')
    export NODE_DFC_IMAGE=$(kubectl get ds -n csi-panfs csi-panfs-node -o jsonpath='{.spec.template.spec.initContainers[?(@.name=="get-dfc-bin")].image}')
    if [  "$NODE_IMAGE" = "${CSIDRIVER_IMAGE}" ]; then
        print "  ✓ CSI Plugin image are set correctly:      ${GREEN}$NODE_IMAGE${RESET}"
    else
        print "${RED}  ✗ CSI Plugin image mismatch:${RESET}"
        print "${RED}      expected: ${CSIDRIVER_IMAGE}${RESET}"
        print "${RED}      found:    ${NODE_IMAGE}${RESET}"
        print
    fi

    if [  "$NODE_DFC_IMAGE" = "${CSIDFCKMM_IMAGE}" ]; then
        print "  ✓ DFC mount helper image is set correctly: ${GREEN}$NODE_DFC_IMAGE${RESET}"
    else
        print "${RED}  ✗ DFC mount helper image mismatch:${RESET}"
        print "${RED}      expected: ${CSIDFCKMM_IMAGE}${RESET}"
        print "${RED}      found:    ${NODE_DFC_IMAGE}${RESET}"
        print
    fi

    if [  "$DFC_IMAGE" = "$NODE_DFC_IMAGE" ]; then
        print "  ✓ DFC mount helper image (initContainer) corresponds to DFC KMM Module image"
    else
        print "${RED}  ✗ DFC mount helper image (initContainer) corresponds to DFC KMM Module image:${RESET}"
        print "${RED}      DFC mount helper image: ${NODE_DFC_IMAGE}${RESET}"
        print "${RED}      DFC KMM Module image:   ${DFC_IMAGE}${RESET}"
        print
    fi
    print

    NODE_READY=$(kubectl get ds -n csi-panfs csi-panfs-node -o jsonpath='{.status.numberReady}')
    if [ "$WORKER_NODES" -eq "$NODE_READY" ]; then
        print "  ✓ All ${GREEN}$NODE_READY/$WORKER_NODES${RESET} worker nodes are ready for PanFS CSI Driver (Node)"
        print "      ${GRAY}Worker Nodes (Node Driver Pods): $WORKER_NODES total, $NODE_READY ready${RESET}"
    else
        print "${RED}   ✗ Not all worker nodes are ready for PanFS CSI Driver: $WORKER_NODES expected, $NODE_READY ready${RESET}"
    fi
    print "      ${GRAY}Node Pods:"
    kubectl get pods -n csi-panfs -l app=csi-panfs-node | sed 's|^|        |'
    echo 
    print "      ${GRAY}Details:"
    kubectl get pods -n csi-panfs -l app=csi-panfs-node -o json | jq -r '(.items | map([.metadata.name,.spec.nodeName,.status.phase,(.spec.containers[]? |select(.name == "csi-driver-registrar") | .name),(.spec.containers[]? |select(.name == "csi-driver-registrar") | .image),(.status.containerStatuses[]? | select(.name == "csi-driver-registrar") | .ready),(.status.containerStatuses[]? | select(.name == "csi-driver-registrar") | .restartCount),(.spec.containers[]? |select(.name == "csi-panfs-plugin") | .name),(.spec.containers[]? |select(.name == "csi-panfs-plugin") | .image),(.status.containerStatuses[]? | select(.name == "csi-panfs-plugin") | .ready),(.status.containerStatuses[]? | select(.name == "csi-panfs-plugin") | .restartCount)])) as $rows | $rows| .[]| @tsv' | sort -k2 | awk '{printf("%8s%s:\n", " ", $1);printf("%10sNode: %s, Pod Phase: %s\n", " ", $2, $3);printf("%10s%s:\n", " ", $4);printf("%12sImage: %s, Ready: %s, Restarts: %s\n", " ", $5, $6, $7);printf("%10s%s:\n", " ", $8);printf("%12sImage: %s, Ready: %s, Restarts: %s\n", " ", $9, $10, $11);printf("\n")}'
    print "${RESET}"
fi

print "${GRAY}Checking PanFS DFC Module status...${RESET}"

if ! kubectl get module -n csi-panfs panfs >/dev/null 2>&1; then
    print "${RED}  ✗ DFC Module is not deployed in csi-panfs namespace${RESET}"
    print
else
    if [ "$DFC_IMAGE" = "${CSIDFCKMM_IMAGE}" ]; then
        print "  ✓ DFC Module image is set correctly: ${GREEN}$DFC_IMAGE${RESET}"
    else
        print "${RED}  ✗ DFC Module image mismatch:${RESET}"
        print "${RED}      expected: ${CSIDFCKMM_IMAGE}${RESET}"
        print "${RED}      found:    ${DFC_IMAGE}${RESET}"
        print
    fi
    print "      ${GRAY}Kernel Version:       ${KERNEL_VERSION}${RESET}"
    print "      ${GRAY}DFC Module Node Selector:"
    kubectl get module -n csi-panfs panfs -o jsonpath='{.spec.selector}' | jq -r 'to_entries[] | "\(.key):\(.value)"' | sed 's|^|        - |'
    print "${RESET}"
    export DFC_LOADED=$(kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.availableNumber}')
    export DFC_DESIRED=$(kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.desiredNumber}')
    if [ "${DFC_LOADED:-0}" -eq "${DFC_DESIRED:-0}" ]; then
        print "  ✓ DFC Module is fully loaded on all nodes: ${GREEN}$DFC_LOADED/$DFC_DESIRED${RESET}"
    else
        print "${RED}  ✗ DFC Module is not fully loaded: $DFC_LOADED/$DFC_DESIRED${RESET}"
    fi
    print "      ${GRAY}DFC Module Load Status:"
    kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader}' | jq -r 'to_entries[] | "\(.key):\(.value)"' | sed 's|^|        |'
    print "${RESET}"
    print "      ${GRAY}Cluster Details:"
    kubectl get nodes -o wide | sed 's|^|        |'
    print "${RESET}"
fi

print "      ${GRAY}Check versions of PanFS module loaded on cluster nodes:"
for node in $(kubectl get nodes -o name); do
    version=$(kubectl debug "$node" -it --image=busybox --quiet -- chroot /host cat /sys/module/panfs/version 2>/dev/null | tr -d '\r' | sed 's/.*No such file or directory.*/No panfs module/')
    print "        $node  $version"
done
print "${RESET}"
print "     ${GRAY}Cleanup debugger pods..."
kubectl get pods | grep debugger | awk '{print $1}' | xargs -IF kubectl delete pod F | sed 's|^|        |'
print "${RESET}"
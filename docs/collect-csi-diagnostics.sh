#!/bin/bash
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


# PanFS CSI Driver Diagnostic Information Collection Script
# This script collects comprehensive diagnostic information for troubleshooting CSI driver issues
# Run this script when experiencing issues and send the output to the support team

set -e

TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
OUTPUT_DIR="panfs-csi-diagnostics-${TIMESTAMP}"
mkdir -p "${OUTPUT_DIR}"

echo "=========================================="
echo "PanFS CSI Driver Diagnostic Collection"
echo "Timestamp: $(date)"
echo "Output directory: ${OUTPUT_DIR}"
echo "=========================================="

# Function to run command and save output
run_cmd() {
    local cmd="$1"
    local output_file="$2"
    local description="$3"
    
    echo ">>> ${description}"
    echo "Command: ${cmd}"
    echo "${cmd}" > "${OUTPUT_DIR}/${output_file}"
    echo "=== Command: ${cmd} ===" >> "${OUTPUT_DIR}/${output_file}"
    eval "${cmd}" >> "${OUTPUT_DIR}/${output_file}" 2>&1 || echo "Command failed with exit code $?" >> "${OUTPUT_DIR}/${output_file}"
    echo "" >> "${OUTPUT_DIR}/${output_file}"
    echo "---"
}

# Basic cluster information
echo "Collecting basic cluster information..."
run_cmd "kubectl cluster-info" "01-cluster-info.txt" "Kubernetes cluster information"
run_cmd "kubectl version --client --output=yaml" "02-kubectl-version.txt" "kubectl version information"
run_cmd "kubectl get nodes -o wide" "03-nodes-wide.txt" "Node information"
run_cmd "kubectl get nodes --show-labels" "04-nodes-labels.txt" "Node labels"
run_cmd "kubectl get nodes -o jsonpath='{.items[*].status.nodeInfo.kernelVersion}'" "05-kernel-versions.txt" "Kernel versions across nodes"

# Namespace and resource checks
echo "Collecting namespace and resource information..."
run_cmd "kubectl get namespace csi-panfs" "06-csi-namespace.txt" "CSI namespace existence"
run_cmd "kubectl get pods -n csi-panfs -o wide" "07-csi-pods.txt" "CSI pods status"
run_cmd "kubectl get all -n csi-panfs" "08-csi-all-resources.txt" "All CSI resources"
run_cmd "kubectl get pods -n kmm-operator-system" "09-kmm-pods.txt" "KMM operator pods"

# CRDs and drivers
echo "Collecting CRD and driver information..."
run_cmd "kubectl get crd | grep -E '(modules|nodemodulesconfigs|csidrivers)'" "10-relevant-crds.txt" "Relevant CRDs"
run_cmd "kubectl get csidrivers" "11-csi-drivers.txt" "CSI drivers"
run_cmd "kubectl get csinode" "12-csi-nodes.txt" "CSI node information"

# Storage classes and volumes
echo "Collecting storage and volume information..."
run_cmd "kubectl get storageclass" "13-storage-classes.txt" "Storage classes"
run_cmd "kubectl get pv" "14-persistent-volumes.txt" "Persistent volumes"
run_cmd "kubectl get volumeattachment" "15-volume-attachments.txt" "Volume attachments"

# Service accounts and RBAC
echo "Collecting RBAC and security information..."
run_cmd "kubectl get serviceaccount -n csi-panfs" "16-service-accounts.txt" "Service accounts"
run_cmd "kubectl get clusterrolebinding | grep csi-panfs" "17-clusterrolebindings.txt" "Cluster role bindings"
run_cmd "kubectl get secrets -n csi-panfs" "18-secrets.txt" "Secrets (names only)"

# KMM module information
echo "Collecting KMM module information..."
run_cmd "kubectl get module panfs -n csi-panfs -o yaml" "19-kmm-module.yaml" "KMM module configuration"
run_cmd "kubectl get nmc -A -o wide" "20-node-module-configs.txt" "Node Module Configs"

# Events
echo "Collecting events..."
run_cmd "kubectl get events -n csi-panfs --sort-by='.lastTimestamp'" "21-csi-events.txt" "CSI namespace events"
run_cmd "kubectl get events --all-namespaces --sort-by='.lastTimestamp' | tail -100" "22-recent-events.txt" "Recent cluster events"
run_cmd "kubectl get events -n csi-panfs --field-selector reason=ModuleLoaded" "23-module-loaded-events.txt" "Module loaded events"
run_cmd "kubectl get events -n csi-panfs --field-selector reason=ModuleLoadFailed" "24-module-failed-events.txt" "Module load failed events"

# Container logs
echo "Collecting container logs..."
run_cmd "kubectl logs -n csi-panfs -l app=csi-panfs-controller --all-containers --tail=1000" "25-controller-logs.txt" "CSI controller logs"
run_cmd "kubectl logs -n csi-panfs -l app=csi-panfs-node --all-containers --tail=1000" "26-node-logs.txt" "CSI node logs"
run_cmd "kubectl logs -n csi-panfs -l app=panfs-kmm-module --tail=1000" "27-kmm-module-logs.txt" "KMM module logs"

# Previous container logs (if available)
run_cmd "kubectl logs -n csi-panfs -l app=csi-panfs-controller --all-containers --previous --tail=1000" "28-controller-previous-logs.txt" "CSI controller previous logs"
run_cmd "kubectl logs -n csi-panfs -l app=csi-panfs-node --all-containers --previous --tail=1000" "29-node-previous-logs.txt" "CSI node previous logs"

# Network policies
echo "Collecting network information..."
run_cmd "kubectl get networkpolicy -A" "30-network-policies.txt" "Network policies"
run_cmd "kubectl get pods -n kube-system | grep -E '(calico|flannel|weave|cilium)'" "31-cni-pods.txt" "CNI plugin pods"

# Node resource usage
echo "Collecting resource usage information..."
run_cmd "kubectl top nodes" "32-node-resources.txt" "Node resource usage"
run_cmd "kubectl top pods -n csi-panfs" "33-csi-pod-resources.txt" "CSI pod resource usage"
run_cmd "kubectl describe nodes | grep -A 5 'Allocated resources'" "34-node-allocated-resources.txt" "Node allocated resources"

# Detailed pod descriptions
echo "Collecting detailed pod information..."
for pod in $(kubectl get pods -n csi-panfs -o name | cut -d/ -f2); do
    run_cmd "kubectl describe pod ${pod} -n csi-panfs" "35-pod-${pod}-describe.txt" "Pod ${pod} detailed information"
done

# Detailed storage class descriptions
echo "Collecting storage class details..."
for sc in $(kubectl get storageclass -o name | cut -d/ -f2); do
    if kubectl get storageclass "${sc}" -o yaml | grep -q "com.vdura.csi.panfs"; then
        run_cmd "kubectl describe storageclass ${sc}" "36-storageclass-${sc}-describe.txt" "Storage class ${sc} details"
        run_cmd "kubectl get storageclass ${sc} -o yaml" "37-storageclass-${sc}.yaml" "Storage class ${sc} YAML"
    fi
done

# Check for specific node module loading
echo "Collecting node-specific module information..."
NODE_COUNT=0
for node in $(kubectl get nodes -o name | cut -d/ -f2); do
    NODE_COUNT=$((NODE_COUNT + 1))
    if [ $NODE_COUNT -le 5 ]; then  # Limit to first 5 nodes to avoid excessive output
        run_cmd "kubectl debug node/${node} -it --image=busybox -- chroot /host lsmod | grep panfs" "38-node-${node}-lsmod.txt" "Node ${node} loaded modules"
        run_cmd "kubectl debug node/${node} -it --image=busybox -- chroot /host dmesg | grep panfs | tail -50" "39-node-${node}-dmesg.txt" "Node ${node} kernel messages"
    fi
done

# Performance and timing information
echo "Collecting performance information..."
run_cmd "kubectl get events --all-namespaces --sort-by='.lastTimestamp' | grep -E '(ProvisioningSucceeded|AttachVolume|MountVolume)' | tail -50" "40-volume-events.txt" "Volume operation events"

# Collect any PVCs that might be in problematic states
echo "Collecting PVC information..."
run_cmd "kubectl get pvc -A" "41-all-pvcs.txt" "All PVCs in cluster"
for pvc in $(kubectl get pvc -A -o jsonpath='{range .items[*]}{.metadata.namespace}{"/"}{.metadata.name}{"\n"}{end}' | head -10); do
    namespace=$(echo $pvc | cut -d/ -f1)
    pvc_name=$(echo $pvc | cut -d/ -f2)
    run_cmd "kubectl describe pvc ${pvc_name} -n ${namespace}" "42-pvc-${namespace}-${pvc_name}-describe.txt" "PVC ${pvc_name} in namespace ${namespace}"
done

# Collect failed workload pods information
echo "Collecting failed workload pods information..."
run_cmd "kubectl get pods -A -o wide | grep -E '(Pending|Failed|CrashLoopBackOff|ContainerCreating|ImagePullBackOff|Error)'" "43-failed-workload-pods.txt" "Failed workload pods across all namespaces"

# Collect details for failed pods (limit to avoid excessive output)
FAILED_POD_COUNT=0
kubectl get pods -A -o json | jq -r '.items[] | select(.status.phase != "Running" and .status.phase != "Succeeded") | "\(.metadata.namespace)/\(.metadata.name)"' | head -15 | while read pod_info; do
    FAILED_POD_COUNT=$((FAILED_POD_COUNT + 1))
    namespace=$(echo $pod_info | cut -d/ -f1)
    pod_name=$(echo $pod_info | cut -d/ -f2)
    
    # Skip CSI pods as they're already collected
    if [ "$namespace" != "csi-panfs" ] && [ "$namespace" != "kmm-operator-system" ]; then
        run_cmd "kubectl describe pod ${pod_name} -n ${namespace}" "44-failed-pod-${namespace}-${pod_name}-describe.txt" "Failed pod ${pod_name} in namespace ${namespace}"
        run_cmd "kubectl logs ${pod_name} -n ${namespace} --tail=200 --all-containers" "45-failed-pod-${namespace}-${pod_name}-logs.txt" "Failed pod ${pod_name} logs"
        run_cmd "kubectl get events -n ${namespace} --field-selector involvedObject.name=${pod_name}" "46-failed-pod-${namespace}-${pod_name}-events.txt" "Failed pod ${pod_name} events"
    fi
done

# Collect namespace security information for namespaces with failed pods
echo "Collecting namespace security information..."
kubectl get namespaces -o name | cut -d/ -f2 | grep -v -E '^(kube-system|kube-public|kube-node-lease|csi-panfs|kmm-operator-system)$' | head -10 | while read ns; do
    run_cmd "kubectl describe namespace ${ns}" "47-namespace-${ns}-describe.txt" "Namespace ${ns} details"
    run_cmd "kubectl get namespace ${ns} -o yaml" "48-namespace-${ns}.yaml" "Namespace ${ns} YAML configuration"
    run_cmd "kubectl get resourcequota -n ${ns}" "49-namespace-${ns}-resourcequota.txt" "Namespace ${ns} resource quotas"
    run_cmd "kubectl get limitrange -n ${ns}" "50-namespace-${ns}-limitrange.txt" "Namespace ${ns} limit ranges"
    run_cmd "kubectl get networkpolicy -n ${ns}" "51-namespace-${ns}-networkpolicy.txt" "Namespace ${ns} network policies"
    run_cmd "kubectl get rolebinding -n ${ns}" "52-namespace-${ns}-rolebinding.txt" "Namespace ${ns} role bindings"
done

# Collect Pod Security Standards and Security Context Constraints
echo "Collecting security policies information..."
run_cmd "kubectl get podsecuritypolicy" "53-pod-security-policies.txt" "Pod Security Policies (if present)"
run_cmd "kubectl get securitycontextconstraints" "54-security-context-constraints.txt" "Security Context Constraints (OpenShift)"
run_cmd "kubectl get validatingadmissionwebhook" "55-validating-admission-webhooks.txt" "Validating Admission Webhooks"
run_cmd "kubectl get mutatingadmissionwebhook" "56-mutating-admission-webhooks.txt" "Mutating Admission Webhooks"

# System information
echo "Collecting system information..."
run_cmd "kubectl get --raw /metrics | grep etcd_request_duration | head -20" "57-etcd-metrics.txt" "etcd performance metrics"
run_cmd "kubectl get --raw /metrics | grep apiserver_request_duration | head -20" "58-apiserver-metrics.txt" "API server performance metrics"

# Create summary file
echo "Creating summary file..."
cat > "${OUTPUT_DIR}/00-SUMMARY.txt" << EOF
PanFS CSI Driver Diagnostic Collection Summary
==============================================

Collection Date: $(date)
Kubernetes Version: $(kubectl version --short --client 2>/dev/null || echo "Unable to determine")
Cluster Info: $(kubectl cluster-info | head -1 | grep -o 'https://[^[:space:]]*' || echo "Unable to determine")

This directory contains diagnostic information collected for troubleshooting PanFS CSI Driver issues.

Key Files:
- 01-cluster-info.txt: Basic cluster information
- 07-csi-pods.txt: CSI pod status
- 19-kmm-module.yaml: KMM module configuration
- 21-csi-events.txt: CSI namespace events
- 25-controller-logs.txt: CSI controller logs
- 26-node-logs.txt: CSI node logs
- 43-failed-workload-pods.txt: Failed workload pods across all namespaces
- 44-47-*: Failed workload pod details, logs, and events
- 47-52-*: Namespace security and configuration details
- 53-56-*: Security policies and admission controllers

Instructions for Support Team:
1. Review the summary files first (01-08)
2. Check CSI pod status and events (07, 21-24)
3. Analyze controller and node logs (25-29)
4. Review KMM module configuration and logs (19, 27)
5. Check storage class configuration (36-37)
6. Review node-specific information if module loading issues (38-39)
7. Analyze failed workload pods and their logs (43-46)
8. Review namespace security and policies (47-56)

Please include this entire directory when submitting support requests.
EOF

echo "=========================================="
echo "Diagnostic collection completed successfully!"
echo "Output directory: ${OUTPUT_DIR}"
echo "Files collected: $(ls -1 ${OUTPUT_DIR} | wc -l)"
echo ""
echo "Please compress this directory and send to support:"
echo "  tar -czf panfs-csi-diagnostics-${TIMESTAMP}.tar.gz ${OUTPUT_DIR}"
echo "=========================================="

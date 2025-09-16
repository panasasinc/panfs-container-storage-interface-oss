<!-- 
  Copyright 2025 VDURA Inc.

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
-->

# PanFS CSI Driver Diagnostic Commands Reference

This document contains all diagnostic commands extracted from the Troubleshooting.md file that customers should execute when experiencing issues with the PanFS CSI Driver. The output from these commands should be sent to VDURA Technical Support for investigation.

## Automated Collection Script

For convenience, use the provided [cripts/collect-csi-diagnostics.sh](collect-csi-diagnostics.sh) script:

```bash
bash collect-csi-diagnostics.sh
```

## Manual Command Collection

If you prefer to run commands manually or the script does not work in your environment, use the following commands:

### 1. Basic Cluster Information

```bash
# Verify Kubernetes cluster is accessible
kubectl cluster-info

# Check if CSI Driver namespace exists
kubectl get namespace csi-panfs

# Verify KMM is installed and running
kubectl get pods -n kmm-operator-system

# Check if required CRDs are installed
kubectl get crd | grep -E "(modules|nodemodulesconfigs|csidrivers)"

# Verify node readiness
kubectl get nodes -o wide

# Check node labels
kubectl get nodes --show-labels

# Check kernel versions across nodes
kubectl get nodes -o jsonpath='{.items[*].status.nodeInfo.kernelVersion}'
```

### 2. CSI Driver Status

```bash
# Check pod status and identify failing pods
kubectl get pods -n csi-panfs -o wide

# Get comprehensive status
kubectl get all -n csi-panfs

# Check CSI Driver registration
kubectl get csidrivers

# Verify node driver registration
kubectl get csinode
```

### 3. Pod Detailed Information

```bash
# Get detailed pod information for ALL CSI pods
kubectl describe pod <pod-name> -n csi-panfs

# Get detailed pod information for a single CSI pod (replace <pod-name> with actual pod name from step 2)
kubectl describe pod <pod-name> -n csi-panfs 
```

### 4. Container Logs

```bash
# Get CSI Driver Controller logs
kubectl logs -n csi-panfs -l app=csi-panfs-controller --all-containers --tail=1000

# Get CSI Driver Node logs
kubectl logs -n csi-panfs -l app=csi-panfs-node --all-containers --tail=1000

# Get previous logs if pods have restarted
kubectl logs -n csi-panfs -l app=csi-panfs-controller --all-containers --previous --tail=1000
kubectl logs -n csi-panfs -l app=csi-panfs-node --all-containers --previous --tail=1000

# Individual container logs (replace container names as needed)
kubectl logs <pod-name> -c csi-panfs-driver -n csi-panfs --tail=1000
kubectl logs <pod-name> -c csi-provisioner -n csi-panfs --tail=1000
kubectl logs <pod-name> -c csi-attacher -n csi-panfs --tail=1000
```

### 5. KMM Module Information

```bash
# Check module CR status
kubectl get module panfs -n csi-panfs -o yaml

# Check NodeModuleConfig status across all nodes
kubectl get nmc -A -o wide

# Check module loading events
kubectl get events -n csi-panfs --field-selector reason=ModuleLoaded
kubectl get events -n csi-panfs --field-selector reason=ModuleLoadFailed

# Check module pod logs
kubectl logs -n csi-panfs -l app=panfs-kmm-module --tail=1000

# Check node labels for KMM
kubectl get nodes --show-labels | grep -E "(kmm|kernel)"

# Verify module node selector
kubectl get module panfs -n csi-panfs -o yaml | grep -A 5 nodeSelector

# Check if nodes meet kernel requirements
kubectl get module panfs -n csi-panfs -o yaml | grep -A 10 kernelMappings
```

### 6. Node-Specific Module Status

```bash
# Check if module loaded successfully on nodes (run for each node)
for node in $(kubectl get nodes -o name); do
  echo "=== $node ==="
  kubectl debug $node -it --image=busybox -- chroot /host lsmod | grep panfs
done

# Check module version compatibility (run for each problematic node)
kubectl debug node/<node-name> -it --image=busybox -- chroot /host cat /sys/module/panfs/version

# Verify module dependencies (run for each problematic node)
kubectl debug node/<node-name> -it --image=busybox -- chroot /host modinfo panfs

# Check kernel messages for panfs (run for each problematic node)
kubectl debug node/<node-name> -it --image=busybox -- chroot /host dmesg | grep panfs | tail -100

# Verify kernel headers availability (run for each problematic node)
kubectl debug node/<node-name> -it --image=registry.redhat.io/rhel8/support-tools -- chroot /host rpm -qa | grep kernel-headers
```

### 7. Storage Class and Volume Information

```bash
# List all storage classes
kubectl get storageclass

# Check specific storage class details (for each PanFS storage class)
kubectl describe storageclass <storage-class-name>
kubectl get storageclass <storage-class-name> -o yaml

# Check PV status
kubectl get pv
kubectl describe pv <pv-name>

# Check volume attachment status
kubectl get volumeattachment
kubectl describe volumeattachment <attachment-name>

# Check all PVCs
kubectl get pvc -A
```

### 8. PVC and Pod Volume Issues

```bash
# Check PVC status and events (for each problematic PVC)
kubectl describe pvc <pvc-name> -n <namespace>

# Check pod events (for each pod with volume issues)
kubectl describe pod <pod-name> -n <namespace>

# Verify mount point permissions (for each problematic node)
kubectl debug node/<node-name> -it --image=busybox -- chroot /host ls -la /var/lib/kubelet/pods/
```

### 8.1. Failed Workload Pods and Application Analysis

```bash
# Get all pods in problematic namespaces with their status
kubectl get pods -A -o wide | grep -E "(Pending|Failed|CrashLoopBackOff|ContainerCreating|ImagePullBackOff)"

# Get detailed information for failed workload pods
kubectl describe pod <failed-pod-name> -n <namespace>

# Get logs from failed workload pods
kubectl logs <failed-pod-name> -n <namespace> --tail=500
kubectl logs <failed-pod-name> -n <namespace> --previous --tail=500

# For multi-container pods, get logs from all containers
kubectl logs <failed-pod-name> -n <namespace> --all-containers --tail=500
kubectl logs <failed-pod-name> -n <namespace> --all-containers --previous --tail=500

# Check if pods are stuck due to volume mounting issues
kubectl get events -n <namespace> --field-selector involvedObject.name=<pod-name>

# Get pod YAML for troubleshooting volume configurations
kubectl get pod <failed-pod-name> -n <namespace> -o yaml

# Check pod's volume mounts and volume definitions
kubectl get pod <failed-pod-name> -n <namespace> -o jsonpath='{.spec.volumes}' | jq .
kubectl get pod <failed-pod-name> -n <namespace> -o jsonpath='{.spec.containers[*].volumeMounts}' | jq .

# Check if PVCs are properly bound for the workload
kubectl get pvc -n <namespace>
kubectl describe pvc <pvc-name> -n <namespace>
```

### 8.2. Namespace Security and Pod Security Standards

```bash
# Get namespace details and labels
kubectl describe namespace <namespace>
kubectl get namespace <namespace> -o yaml

# Check Pod Security Standards (if enabled)
kubectl get namespace <namespace> -o jsonpath='{.metadata.labels}' | grep -E "(pod-security|security)"

# Check for Pod Security Policies (deprecated but might still be present)
kubectl get podsecuritypolicy
kubectl describe podsecuritypolicy <policy-name>

# Check Security Context Constraints (OpenShift)
kubectl get securitycontextconstraints
kubectl describe securitycontextconstraints <scc-name>

# Check namespace resource quotas that might affect pod creation
kubectl get resourcequota -n <namespace>
kubectl describe resourcequota -n <namespace>

# Check limit ranges that might affect pod creation
kubectl get limitrange -n <namespace>
kubectl describe limitrange -n <namespace>

# Check network policies affecting the namespace
kubectl get networkpolicy -n <namespace>
kubectl describe networkpolicy -n <namespace>

# Check RBAC for the namespace
kubectl get rolebinding -n <namespace>
kubectl get role -n <namespace>
kubectl describe rolebinding -n <namespace>
kubectl describe role -n <namespace>

# Check service accounts in the namespace
kubectl get serviceaccount -n <namespace>
kubectl describe serviceaccount <sa-name> -n <namespace>

# Check if namespace has any admission controllers or webhook configurations
kubectl get validatingadmissionwebhook
kubectl get mutatingadmissionwebhook
```

### 9. Authentication and Secret Information

```bash
# Check storage class configuration
kubectl get sc | grep com.vdura.csi.panfs
kubectl describe sc <storage-class-name>

# Get secret details from storage class (run these commands in sequence)
NAMESPACE=$(kubectl get sc <SC_NAME> -o jsonpath="{.parameters.csi\.storage\.k8s\.io/provisioner-secret-namespace}")
SECRET_NAME=$(kubectl get sc <SC_NAME> -o jsonpath="{.parameters.csi\.storage\.k8s\.io/provisioner-secret-name}")
echo "Secret: $SECRET_NAME in namespace: $NAMESPACE"

# Verify secret exists
kubectl get secret -n $NAMESPACE $SECRET_NAME

# Check credential values (SENSITIVE - handle carefully)
kubectl get secret -n $NAMESPACE $SECRET_NAME -o jsonpath='{.data.realm_ip}' | base64 -d
kubectl get secret -n $NAMESPACE $SECRET_NAME -o jsonpath='{.data.user}' | base64 -d

# Check CSI Driver logs for auth errors
kubectl logs -n csi-panfs -l app=csi-panfs-controller --all-containers | grep -i "auth\|cert\|credential" | tail -50
```

### 10. RBAC and Security

```bash
# Check ServiceAccount and ClusterRoleBindings
kubectl get serviceaccount -n csi-panfs
kubectl get clusterrolebinding | grep csi-panfs

# Verify permissions
kubectl auth can-i create persistentvolumes --as=system:serviceaccount:csi-panfs:csi-panfs-controller-sa

# Check secrets (names only for security)
kubectl get secrets -n csi-panfs

# Check image pull secrets
kubectl get secrets -n csi-panfs | grep dockerconfigjson
```

### 11. Network and Connectivity

```bash
# Check for network policies
kubectl get networkpolicy -A

# Check if policies affect csi-panfs namespace
kubectl describe networkpolicy -n csi-panfs

# Check CNI plugin status
kubectl get pods -n kube-system | grep -E "(calico|flannel|weave|cilium)"

# Check node network configuration
kubectl describe nodes | grep -A 10 "PodCIDR\|InternalIP"

# Check cluster DNS configuration
kubectl get configmap coredns -n kube-system -o yaml
```

### 12. Resource Usage and Performance

```bash
# Check CSI Driver resource usage
kubectl top pods -n csi-panfs
kubectl top nodes

# Check node resources
kubectl describe nodes | grep -A 5 "Allocated resources"

# Check pod resource requests
kubectl describe pods -n csi-panfs | grep -A 10 "Requests:\|Limits:"

# Check if pods are being throttled
kubectl describe pods -n csi-panfs | grep -A 5 -B 5 "throttling"

# Review current resource allocations
kubectl get pods -n csi-panfs -o yaml | grep -A 10 resources:

# Monitor volume operation times
kubectl get events --sort-by='.lastTimestamp' | grep -E "(ProvisioningSucceeded|AttachVolume|MountVolume)" | tail -50

# Check CSI logs for slow operations
kubectl logs -n csi-panfs -l app=csi-panfs-controller --tail=100 | grep -E "took|duration|timeout"

# Check etcd performance
kubectl get --raw /metrics | grep etcd_request_duration | head -20

# Check API server response times
kubectl get --raw /metrics | grep apiserver_request_duration | head -20
```

### 13. Node Taints and Tolerations

```bash
# Verify DaemonSet node selector
kubectl get daemonset csi-panfs-node -n csi-panfs -o jsonpath='{.spec.template.spec.nodeSelector}'

# Verify DaemonSet node tolerations
kubectl get daemonset csi-panfs-node -n csi-panfs -o jsonpath='{.spec.template.spec.tolerations}'

# Get Worker Node(s) taints (run for each node)
kubectl get node <NODE_NAME> -o jsonpath='{.spec.taints}'
```

### 14. Events Collection

```bash
# View recent events in CSI Driver workloads
kubectl get events -n csi-panfs --sort-by='.lastTimestamp'

# View recent events in custom workloads
kubectl get events -n default --sort-by='.lastTimestamp'

# Get all recent cluster events
kubectl get events --all-namespaces --sort-by='.lastTimestamp' | tail -100
```

## Network Connectivity Testing (Optional)

These commands can help test network connectivity to the PanFS realm but should only be run if network issues are suspected:

```bash
# Test basic connectivity (replace <realm-ip> with actual IP)
kubectl run connectivity-test --image=busybox --rm -it -- ping <realm-ip>

# Test specific ports
kubectl run port-test --image=nicolaka/netshoot --rm -it -- nc -zv <realm-ip> <port>

# Check DNS resolution
kubectl run dns-test --image=busybox --rm -it -- nslookup <realm-hostname>

# Test from multiple nodes
for node in $(kubectl get nodes -o name | cut -d/ -f2); do
  echo "Testing from node: $node"
  kubectl debug node/$node -it --image=nicolaka/netshoot -- ping -c 3 <realm-ip>
done
```

## Important Notes

1. **Security**: Be careful when collecting secret information. Never share decoded passwords or SSH keys.

2. **Node Access**: Some commands require node debugging access. Ensure you have appropriate permissions.

3. **Resource Usage**: The collection process may impact cluster performance temporarily.

4. **Sensitive Information**: Review all collected logs before sharing to ensure no sensitive information is included.

5. **Large Outputs**: Some commands may produce large outputs. Consider using `--tail` or `head` to limit output size.

## Packaging for Support

After collecting the diagnostic information:

```bash
# Create a compressed archive
tar -czf panfs-csi-diagnostics-$(date +%Y%m%d_%H%M%S).tar.gz <output-directory>

# Or use the automated script output
tar -czf panfs-csi-diagnostics-$(date +%Y%m%d_%H%M%S).tar.gz panfs-csi-diagnostics-*
```

Send the compressed archive to the support team along with a description of the issue and steps to reproduce.

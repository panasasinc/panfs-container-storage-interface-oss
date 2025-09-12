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

# PanFS CSI Driver Troubleshooting Guide

This guide provides comprehensive troubleshooting steps for common issues encountered when using the PanFS CSI Driver in Kubernetes environments.

## Table of Contents

- [Pre-Flight Checks](#pre-flight-checks)
- [Common Issues and Solutions](#common-issues-and-solutions)
  - [1. Driver Installation Issues](#1-driver-installation-issues)
  - [2. Volume Mounting Issues](#2-volume-mounting-issues)
  - [3. KMM Module Loading Issues](#3-kmm-module-loading-issues)
  - [4. Authentication Issues](#4-authentication-issues)
  - [5. Performance Issues](#5-performance-issues)
  - [6. Storage Class Issues](#6-storage-class-issues)
  - [7. Network Connectivity Issues](#7-network-connectivity-issues)
- [Diagnostic Commands](#diagnostic-commands)
- [Log Analysis](#log-analysis)
- [Getting Help](#getting-help)

---

## Pre-Flight Checks

Before troubleshooting specific issues, verify these basic requirements:

```bash
# 1. Verify Kubernetes cluster is accessible
kubectl cluster-info

# 2. Check if CSI Driver namespace exists
kubectl get namespace csi-panfs

# 3. Verify KMM is installed and running
kubectl get pods -n kmm-operator-system

# 4. Check if required CRDs are installed
kubectl get crd | grep -E "(modules|nodemodulesconfigs|csidrivers)"

# 5. Verify node readiness
kubectl get nodes -o wide
```

## Common Issues and Solutions

#### 1. Driver Installation Issues

**Problem**: CSI Driver pods are not starting

**Diagnosis**:
```bash
# Check pod status and identify failing pods
kubectl get pods -n csi-panfs -o wide

# Get detailed pod information
kubectl describe pod <pod-name> -n csi-panfs

# Check container logs (replace container names as needed)
kubectl logs <pod-name> -c csi-panfs-driver -n csi-panfs
kubectl logs <pod-name> -c csi-provisioner -n csi-panfs --previous
```

**Common causes and solutions**:

- **Image pull failures**:
  ```bash
  # Check if image pull secret exists
  kubectl get secrets -n csi-panfs | grep dockerconfigjson
  
  # Verify registry credentials
  kubectl get secret <image-pull-secret> -n csi-panfs -o yaml
  
  # Test image pull manually
  docker pull <csi-driver-image>
  ```

- **Insufficient RBAC permissions**:
  ```bash
  # Check ServiceAccount and ClusterRoleBindings
  kubectl get serviceaccount -n csi-panfs
  kubectl get clusterrolebinding | grep csi-panfs
  
  # Verify permissions
  kubectl auth can-i create persistentvolumes --as=system:serviceaccount:csi-panfs:csi-panfs-controller-sa
  ```

- **Node selector mismatch**:
  ```bash
  # Check node labels
  kubectl get nodes --show-labels
  
  # Verify DaemonSet node selector
  kubectl get daemonset csi-panfs-node -n csi-panfs -o jsonpath='{.spec.template.spec.nodeSelector}'

  # Verify DaemonSet node tolerations
  kubectl get daemonset csi-panfs-node -n csi-panfs -o jsonpath='{.spec.template.spec.tolerations}'

  # Get Worker Node(s) taints:
  kubectl get node <NODE_NAME> -o jsonpath='{.spec.taints}'
  ```

- **Resource constraints**:
  ```bash
  # Check node resources
  kubectl describe nodes | grep -A 5 "Allocated resources"
  
  # Check pod resource requests
  kubectl describe pod <pod-name> -n csi-panfs | grep -A 10 "Requests:"
  ```

#### 2. Volume Mounting Issues

**Problem**: Volumes fail to mount to pods

**Diagnosis**:
```bash
# Check PVC status and events
kubectl describe pvc <pvc-name> -n <namespace>

# Check PV status
kubectl get pv | grep <pvc-name>
kubectl describe pv <pv-name>

# Check volume attachment status
kubectl get volumeattachment
kubectl describe volumeattachment <attachment-name>

# Check pod events
kubectl describe pod <pod-name> -n <namespace>
```

**Common causes and solutions**:

- **PVC stuck in Pending state**:
  ```bash
  # Check storage class exists
  kubectl get storageclass
  
  # Verify storage class parameters
  kubectl describe storageclass <storage-class-name>
  
  # Check provisioner logs
  kubectl logs -n csi-panfs -l app=csi-panfs-controller -c csi-provisioner
  ```

- **Mount failures on nodes**:
  ```bash
  # Check node CSI Driver logs
  kubectl logs -n csi-panfs -l app=csi-panfs-node --all-containers
  
  # Check if PanFS module is loaded on the node
  kubectl debug node/<node-name> -it --image=busybox -- chroot /host lsmod | grep panfs
  
  # Verify mount point permissions
  kubectl debug node/<node-name> -it --image=busybox -- chroot /host ls -la /var/lib/kubelet/pods/
  ```

- **Volume attachment failures**:
  ```bash
  # Check CSI attacher logs
  kubectl logs -n csi-panfs -l app=csi-panfs-controller -c csi-attacher
  
  # Verify node driver registration
  kubectl get csinode
  kubectl describe csinode <node-name>
  ```

#### 3. KMM Module Loading Issues

**Problem**: PanFS kernel module fails to load

**Diagnosis**:
```bash
# Check module CR status
kubectl get module panfs -n csi-panfs -o yaml

# Check NodeModuleConfig status across all nodes
kubectl get nmc -A -o wide

# Check module loading events
kubectl get events -n csi-panfs --field-selector reason=ModuleLoaded
kubectl get events -n csi-panfs --field-selector reason=ModuleLoadFailed

# Check kernel compatibility
kubectl get nodes -o jsonpath='{.items[*].status.nodeInfo.kernelVersion}'
```

**Common causes and solutions**:

- **Kernel module build failures**:
  ```bash
  # Check module pod logs
  kubectl logs -n csi-panfs -l app=panfs-kmm-module
  
  # Verify kernel headers availability
  kubectl debug node/<node-name> -it --image=registry.redhat.io/rhel8/support-tools -- chroot /host rpm -qa | grep kernel-headers
  
  # Check build container logs
  kubectl logs -n csi-panfs <module-build-pod-name>
  ```

- **Module loading failures**:
  ```bash
  # Check if module loaded successfully on nodes
  for node in $(kubectl get nodes -o name); do
    echo "=== $node ==="
    kubectl debug $node -it --image=busybox -- chroot /host lsmod | grep panfs
  done
  
  # Check module version compatibility
  kubectl debug node/<node-name> -it --image=busybox -- chroot /host cat /sys/module/panfs/version
  
  # Verify module dependencies
  kubectl debug node/<node-name> -it --image=busybox -- chroot /host modinfo panfs
  ```

- **Node selector issues**:
  ```bash
  # Check node labels for KMM
  kubectl get nodes --show-labels | grep -E "(kmm|kernel)"
  
  # Verify module node selector
  kubectl get module panfs -n csi-panfs -o yaml | grep -A 5 nodeSelector
  
  # Check if nodes meet kernel requirements
  kubectl get module panfs -n csi-panfs -o yaml | grep -A 10 kernelMappings
  ```

**Manual verification on worker nodes**:
```bash
# SSH to worker node and check module status
ssh <worker-node>
sudo lsmod | grep panfs
sudo cat /sys/module/panfs/version
sudo dmesg | grep panfs | tail -100
```

#### 4. Authentication Issues

**Problem**: Cannot connect to PanFS realm

**Diagnosis**:
```bash
# Check storage class configuration
kubectl get sc | grep com.vdura.csi.panfs
kubectl describe sc <storage-class-name>

# Get secret details from storage class
NAMESPACE=$(kubectl get sc <SC_NAME> -o jsonpath="{.parameters.csi\.storage\.k8s\.io/provisioner-secret-namespace}")
SECRET_NAME=$(kubectl get sc <SC_NAME> -o jsonpath="{.parameters.csi\.storage\.k8s\.io/provisioner-secret-name}")

echo "Secret: $SECRET_NAME in namespace: $NAMESPACE"

# Verify secret exists
kubectl get secret -n $NAMESPACE $SECRET_NAME
```

**Common causes and solutions**:

- **Missing or incorrect credentials**:
  ```bash
  # Check credential values
  kubectl get secret -n $NAMESPACE $SECRET_NAME -o jsonpath='{.data.realm_ip}' | base64 -d
  kubectl get secret -n $NAMESPACE $SECRET_NAME -o jsonpath='{.data.user}' | base64 -d
  
  # Verify password is set to correct value
  kubectl get secret -n $NAMESPACE $SECRET_NAME -o jsonpath='{.data.password}' | base64 -d
  
  # Check if SSH key set correctly
  kubectl get secret -n $NAMESPACE $SECRET_NAME -o jsonpath='{.data.private_key}' | base64 -d
  kubectl get secret -n $NAMESPACE $SECRET_NAME -o jsonpath='{.data.private_key_passphrase}' | base64 -d
  ```

- **Network connectivity issues**:
  ```bash
  # Test connectivity from a node
  kubectl run test-connectivity --image=busybox --rm -it -- ping <realm-ip>
  
  # Test specific ports (replace <realm-ip> with actual PanFS port)
  kubectl run test-connectivity --image=nicolaka/netshoot --rm -it -- telnet <realm-ip> <port>
  
  # Check firewall rules and routing
  kubectl run test-connectivity --image=nicolaka/netshoot --rm -it -- traceroute <realm-ip>
  ```

- **DNS resolution issues**:
  ```bash
  # Test DNS resolution
  kubectl run test-dns --image=busybox --rm -it -- nslookup <realm-hostname>
  
  # Check cluster DNS configuration
  kubectl get configmap coredns -n kube-system -o yaml
  ```

- **Certificate/authentication errors**:
  ```bash
  # Check CSI Driver logs for auth errors
  kubectl logs -n csi-panfs -l app=csi-panfs-controller --all-containers | grep -i "auth\|cert\|credential"
  
  # Verify realm accessibility with curl
  kubectl run test-auth --image=curlimages/curl --rm -it -- curl -v <realm-endpoint>
  ```

**Verification steps**:
- Verify credentials are correct and not expired
- Check network connectivity to PanFS realm
- Validate firewall rules and required ports (typically 106, 988, and 7406)
- Ensure realm address is resolvable from cluster nodes
- Confirm user has appropriate permissions on PanFS realm

#### 5. Performance Issues

**Problem**: Slow volume operations

**Diagnosis**:
```bash
# Check CSI Driver resource usage
kubectl top pods -n csi-panfs
kubectl describe pods -n csi-panfs | grep -A 10 "Requests:\|Limits:"

# Monitor volume operation times
kubectl get events --sort-by='.lastTimestamp' | grep -E "(ProvisioningSucceeded|AttachVolume|MountVolume)"

# Check node performance
kubectl top nodes
```

**Common causes and solutions**:

- **Resource constraints on CSI pods**:
  ```bash
  # Check if pods are being throttled
  kubectl describe pods -n csi-panfs | grep -A 5 -B 5 "throttling"
  
  # Review current resource allocations
  kubectl get pods -n csi-panfs -o yaml | grep -A 10 resources:
  
  # Consider increasing resource limits
  # Edit the deployment/daemonset resource specifications
  ```

- **Network latency to PanFS backend**:
  ```bash
  # Test network latency from nodes to PanFS realm
  kubectl run network-test --image=nicolaka/netshoot --rm -it -- ping -c 10 <realm-ip>
  
  # Check bandwidth
  kubectl run network-test --image=nicolaka/netshoot --rm -it -- iperf3 -c <realm-ip> -p <port>
  ```

- **PanFS realm performance**:
  ```bash
  # Check CSI logs for slow operations
  kubectl logs -n csi-panfs -l app=csi-panfs-controller --tail=100 | grep -E "took|duration|timeout"
  
  # Monitor realm-specific metrics (if available)
  # This will depend on your PanFS monitoring setup
  ```

- **Kubernetes cluster performance**:
  ```bash
  # Check etcd performance
  kubectl get --raw /metrics | grep etcd_request_duration
  
  # Check API server response times
  kubectl get --raw /metrics | grep apiserver_request_duration
  ```

#### 6. Storage Class Issues

**Problem**: StorageClass configuration errors

**Diagnosis**:
```bash
# List all StorageClasses
kubectl get storageclass

# Check specific StorageClass details
kubectl describe storageclass <storage-class-name>

# Verify provisioner is correct
kubectl get storageclass <storage-class-name> -o yaml | grep provisioner
```

**Common causes and solutions**:

- **Missing or incorrect provisioner**:
  ```bash
  # Verify the provisioner name matches CSI Driver
  kubectl get csidriver
  kubectl get storageclass <storage-class-name> -o yaml | grep provisioner
  
  # Should be: provisioner: com.vdura.csi.panfs
  ```

- **Invalid parameters**:
  ```bash
  # Check required parameters
  kubectl get storageclass <storage-class-name> -o yaml | grep -A 20 parameters:
  
  # Verify secret references exist
  kubectl get secret -n <secret-namespace> <secret-name>
  ```

- **Volume binding mode issues**:
  ```bash
  # Check volume binding mode
  kubectl get storageclass <storage-class-name> -o yaml | grep volumeBindingMode
  
  # Should be either "Immediate" or "WaitForFirstConsumer"
  ```

#### 7. Network Connectivity Issues

**Problem**: Network connectivity between cluster nodes and PanFS realm

**Diagnosis**:
```bash
# Test basic connectivity
kubectl run connectivity-test --image=busybox --rm -it -- ping <realm-ip>

# Test specific ports
kubectl run port-test --image=nicolaka/netshoot --rm -it -- nc -zv <realm-ip> <port>

# Check DNS resolution
kubectl run dns-test --image=busybox --rm -it -- nslookup <realm-hostname>
```

**Common causes and solutions**:

- **Firewall blocking connections**:
  ```bash
  # Test from multiple nodes
  for node in $(kubectl get nodes -o name | cut -d/ -f2); do
    echo "Testing from node: $node"
    kubectl debug node/$node -it --image=nicolaka/netshoot -- ping -c 3 <realm-ip>
  done
  
  # Check required ports (common PanFS ports: 106, 988, 7406
  kubectl run port-test --image=nicolaka/netshoot --rm -it -- nc -zv <realm-ip> 988
  ```

- **Network policies blocking traffic**:
  ```bash
  # Check for network policies
  kubectl get networkpolicy -A
  
  # Check if policies affect csi-panfs namespace
  kubectl describe networkpolicy -n csi-panfs
  ```

- **CNI-related issues**:
  ```bash
  # Check CNI plugin status
  kubectl get pods -n kube-system | grep -E "(calico|flannel|weave|cilium)"
  
  # Check node network configuration
  kubectl describe nodes | grep -A 10 "PodCIDR\|InternalIP"
  ```

### Diagnostic Commands

```bash
# Get comprehensive status
kubectl get all -n csi-panfs

# Check CSI Driver registration
kubectl get csidrivers

# Inspect StorageClass configuration
kubectl describe sc <storageclass-name>

# View recent events in CSI Driver workloads
kubectl get events -n csi-panfs --sort-by='.lastTimestamp'

# View recent events in custom workloads
kubectl get events -n default --sort-by='.lastTimestamp'

# Get CSI Driver controller logs:
kubectl logs -n csi-panfs -l app=csi-panfs-controller --all-containers

# Get CSI Driver node logs:
kubectl logs -n csi-panfs -l app=csi-panfs-node --all-containers
```

### Getting Help

- **KMM Issues**: Check module status (`kubectl get module panfs -n csi-panfs`) and node labels if modules fail to load
- **Registry Errors**: Ensure `$REGISTRY_CREDS_FILE` is valid and accessible
- **Additional Resources**:
  - [kmm.md](./kmm.md): KMM configuration details
  - [usage-guide.md](./usage-guide.md): Workload deployment examples

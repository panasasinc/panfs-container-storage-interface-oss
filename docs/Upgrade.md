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

# PanFS CSI Driver Upgrade Guide

This document provides comprehensive guidance for upgrading the PanFS Container Storage Interface (CSI) driver and all related components in Kubernetes environments. The guide covers essential upgrade procedures, critical configuration decisions, and validation requirements.

The PanFS CSI driver consists of three primary components that may require updates during an upgrade:

- **CSI Driver**: Provides storage interface for Kubernetes workloads (controller and node components)
- **KMM (Kernel Module Management)**: Manages kernel modules required for PanFS functionality on worker nodes
- **PanFS StorageClass**: Backend PanFS Realm configuration and authentication credentials

## Table of Contents

1. [Pre-Upgrade Requirements](#1-pre-upgrade-requirements)
   - 1.1 [Image Registry Access](#11-image-registry-access)
   - 1.2 [Version Compatibility](#12-version-compatibility)
   - 1.3 [Cluster State Assessment](#13-cluster-state-assessment)

2. [Upgrade Process Overview](#2-upgrade-process-overview)

3. [Upgrade Detailed Guide](#3-upgrade-detailed-guide)
   - 3.1 [Preparation](#31-preparation)
        - 3.1.1 [CSI Driver Configuration Backup](#311-csi-driver-configuration-backup)
        - 3.1.2 [KMM Image Assessment](#312-kmm-image-assessment)
        - 3.1.3 [Workload Pod Management](#313-workload-pod-management)
        - 3.1.4 [Configuration Data Backup (PV)](#314-configuration-data-backup-pv)
   - 3.2 [CSI Driver and KMM Module Update](#32-csi-driver-and-kmm-module-update)
        - 3.2.1 [Update Deployment Manifests](#321-update-deployment-manifests)
        - 3.2.2 [Configuration Review Checklist](#322-configuration-review-checklist)
        - 3.2.3 [Pre-Deployment Validation](#323-pre-deployment-validation)
        - 3.2.4 [Execute CSI Driver and KMM Module Update](#324-execute-csi-driver-and-kmm-module-update)
        - 3.2.5 [Validate CSI Driver and KMM Module Update](#325-validate-csi-driver-and-kmm-module-update)
   - 3.3 [StorageClass Update](#33-storageclass-update)
        - 3.3.1 [Update StorageClass Manifest](#331-update-storageclass-manifest)
        - 3.3.2 [Apply StorageClass Configuration](#332-apply-storageclass-configuration)
        - 3.3.3 [Validate StorageClass](#333-validate-storageclass)
   - 3.4 [Post-Deployment Functional Tests](#34-post-deployment-functional-tests)

4. [Success Criteria](#4-success-criteria)
   - 4.1 [CSI Driver and KMM Module Update](#41-csi-driver-and-kmm-module-update)
   - 4.2 [StorageClass Configuration](#42-storageclass-configuration)

5. [Troubleshooting](#5-troubleshooting)
   - 5.1 [Common Issues](#51-common-issues)
        - 5.1.1 [Image Pull Failures](#511-image-pull-failures)
        - 5.1.2 [KMM Module Load Failures](#512-kmm-module-load-failures)
        - 5.1.3 [Stage Sequencing Issues](#513-stage-sequencing-issues)
        - 5.1.4 [Pod Scheduling Issues](#514-pod-scheduling-issues)
        - 5.1.5 [Volume Mount Failures During Upgrade](#515-volume-mount-failures-during-upgrade)
        - 5.1.6 [Authentication Issues](#516-authentication-issues)
        - 5.1.7 [Workload Restoration Issues](#517-workload-restoration-issues)
        - 5.1.8 [Data Integrity Issues](#518-data-integrity-issues)
   - 5.2 [Diagnostic Commands](#52-diagnostic-commands)
   - 5.3 [Rollback Procedure](#53-rollback-procedure)
        - 5.3.1 [Determine Rollback Scope](#531-determine-rollback-scope)
        - 5.3.2 [Stop All Workloads](#532-stop-all-workloads-if-not-already-done-for-kmm-rollback)
        - 5.3.3 [Rollback CSI Driver Configuration](#533-rollback-csi-driver-configuration)
        - 5.3.4 [Validate Rollback Success](#534-validate-rollback-success)

6. [Support and Escalation](#6-support-and-escalation)

## 1. Pre-Upgrade Requirements

Before initiating any upgrade process, ensure the following requirements are met:

### 1.1 Image Registry Access
- [ ] Verify that new container images are accessible from your Kubernetes cluster
- [ ] Validate registry credentials and network connectivity
- [ ] Confirm image pull policies are configured correctly
- [ ] Test image availability with: `kubectl run test-pull --image=<new-image> --rm -it --restart=Never -- /bin/sh`

### 1.2 Version Compatibility
- [ ] Ensure CSI driver and KMM images are from the same release version
- [ ] Verify deployment manifests match the target release version
- [ ] Check Kubernetes version compatibility with the new CSI driver version
- [ ] Review release notes for breaking changes or deprecations

### 1.3 Cluster State Assessment
- [ ] Identify active PanFS volumes: 
  ```sh
  kubectl get pv -o jsonpath='{.items[?(@.spec.csi.driver=="com.vdura.csi.panfs")].metadata.name}'
  ```
- [ ] List pods using PanFS volumes:  
  ```sh
  # Get all PVCs bound to PanFS PVs
  panfs_pvcs=$(kubectl get pv -o json | 
    jq -r '
      .items[] | 
      select(.spec.csi.driver=="com.vdura.csi.panfs") | 
      "\(.spec.claimRef.namespace)/\(.spec.claimRef.name)"
    '
  )
  # List pods using those PVCs
  kubectl get pods --all-namespaces -o json | jq -r --argjson panfs_pvcs "$(printf '%s\n' "$panfs_pvcs" | jq -R . | jq -s .)" '
    .items[] |
    . as $pod |
    [ 
      .spec.volumes[]? | 
      select(.persistentVolumeClaim) | 
      "\(.persistentVolumeClaim.claimNamespace // $pod.metadata.namespace)/\(.persistentVolumeClaim.claimName)"
    ] as $pod_pvcs |
    select($pod_pvcs | 
    map(. as $pvc | $panfs_pvcs | index($pvc)) | any) |
    "\(.metadata.namespace)/\(.metadata.name)"
  '
  ```
- [ ] Verify KMM module status on target nodes

## 2. Upgrade Process Overview

**Phase 1: Preparation**

- Assess if KMM image has changed (determines scope of update)
- Backup application data (only if KMM image update required)
- Stop workloads using PanFS volumes (only if KMM image update required)
- Backup CSI driver configurations
- Plan maintenance window

**Phase 2: Component Updates**

Updates the controller and node driver components along with the KMM module:

- Update deployment manifests
- Apply CSI driver and KMM module configuration changes
- Monitor controller and node pod rollouts
- Verify all pods are ready
- Verify all nodes have loaded the `panfs` kernel module

**Phase 3: StorageClass Upgrade**

- Review and update StorageClass configuration if needed
- Apply any parameter changes or credential updates
- Validate StorageClass functionality

## 3. Upgrade Detailed Guide

### 3.1 Preparation

#### 3.1.1 CSI Driver Configuration Backup

**Create CSI Driver Configuration Backup:**

Sample steps for reference:
```bash
# Create backup directory with timestamp
BACKUP_DIR="panfs-backup-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"

# Backup current configurations
kubectl get deployment csi-panfs-controller -n csi-panfs -o yaml > "$BACKUP_DIR/csi-panfs-controller-backup.yaml"
kubectl get daemonset csi-panfs-node -n csi-panfs -o yaml > "$BACKUP_DIR/csi-panfs-node-backup.yaml"
kubectl get configmap csi-panfs-config -n csi-panfs -o yaml > "$BACKUP_DIR/csi-panfs-config-backup.yaml" 2>/dev/null || echo "No csi-panfs-config configmap found"
kubectl get module panfs -n csi-panfs -o yaml > "$BACKUP_DIR/panfs-kmm-module.yaml"

# Backup storage classes
kubectl get storageclass -o yaml > "$BACKUP_DIR/storageclass-backup.yaml"

# Backup current PV state
kubectl get pv -o yaml > "$BACKUP_DIR/all-pv-backup.yaml"
```

#### 3.1.2 KMM Image Assessment

##### Determine if a KMM Update is Required

Run the following command to check the currently deployed Kernel Module Management (KMM) image:

```bash
# Check current KMM module image
(
  echo -e "Kernel\tContainerImage";
  kubectl get module panfs -n csi-panfs -o json | 
  jq -r '
    .spec.moduleLoader.container.kernelMappings[] | 
    [
      .literal, 
      .containerImage
    ] | @tsv
  '
) | column -t -s $'\t'
```

Compare the output against the target KMM container image for the new release.

##### Important Decision Point

**1. If the KMM container image is unchanged:**

**Required Actions:**
  - Skip KMM module updates
  - Skip [Workload Pod Management](#313-workload-pod-management)
  - Proceed directly to [CSI Driver and KMM Module Update](#32-csi-driver-and-kmm-module-update)

**Expected Downtime:** Minimal (2-5 minutes)
**Workload Impact:** No interruption to running applications

**Impact:**
  - **Existing Mounted Volumes:** Remain accessible throughout upgrade
  - **New PVC Requests:** Temporary failure during controller restart (auto-recovery)
  - **Data Backup:** Not required for this upgrade type

**2. If the KMM container image is different:**

**Required Actions:** Follow complete upgrade process, including:
  - Phase 1: Backup workloads and delete pods using the old kernel module
  - Phase 2: Update the CSI driver components

**Expected Downtime:** Extended (15-30 minutes + application restart time)
**Workload Impact:** Full application termination required

**Impact:**
  - **All PanFS Workloads:** Must be terminated before upgrade begins
  - **Data Protection:** Application-level backup strongly recommended
  - **Recovery Process:** Workloads restored after successful upgrade completion

#### 3.1.3 Workload Pod Management

This step is only required when the KMM image is being updated. **All pods using PanFS volumes must be terminated to release volume mounts, while preserving PVCs for seamless restoration**. When pods are recreated, they will automatically reconnect to their existing PVCs, maintaining data continuity regardless of reclaim policy.

#### 3.1.4 Configuration Data Backup (PV)

This step is only required if the KMM image is being updated. If only CSI driver components are being updated (KMM image unchanged), you can skip this backup step.

### 3.2 CSI Driver and KMM Module Update

This phase ensures that both the CSI driver components and KMM module are updated to the target release version.

#### 3.2.1 Update Deployment Manifests

**Update Configuration Parameters in the Deployment Manifest:**

Update the settings in the deployment manifest according to your cluster specification and available image tags in your private registry:

- `<KERNEL_VERSION>` - Worker Node kernel version, should correspond to PANFS_KMM_IMAGE, e.g: `4.18.0-553.el8_10.x86_64`
- `<PANFS_KMM_IMAGE>` - PanFS KMM module image, e.g: `<your private registry>/panfs-dfc-kmm:4.18.0-553.el8_10.x86_64-11.1.0.a-1860775.2`
- `<PANFS_CSI_DRIVER_IMAGE>` - PanFS CSI Driver image, e.g: `<your private registry>/panfs-csi-driver:1.0.3`
- `<IMAGE_PULL_SECRET_NAME>` - Image pull secret for fetching PanFS CSI Driver images from your private registry

Review other settings relevant to your Kubernetes infrastructure, such as:
- `replicas` - Controller replica count
- `tolerations` - Node tolerations for scheduling
- `nodeSelector` - Node selection criteria
- Resource requests and limits
- Security contexts and service account permissions

#### 3.2.2 Configuration Review Checklist
- [ ] Compare current cluster configuration with new requirements
- [ ] Update node selectors, tolerations, and affinity rules as needed
- [ ] Verify resource requests and limits are appropriate for your cluster
- [ ] Check for new configuration options or deprecated settings
- [ ] Validate security contexts and service account permissions
- [ ] Review storage class parameters for any updates

#### 3.2.3 Pre-Deployment Validation
```bash
# Validate deployment manifests (server-side dry run)
kubectl apply --dry-run=server -f deploy/k8s/csi-panfs-driver.yaml

# Validate deployment manifests (client-side dry run)
kubectl apply --dry-run=client -f deploy/k8s/csi-panfs-driver.yaml

# Check for any validation errors or warnings
kubectl apply --validate=true --dry-run=client -f deploy/k8s/csi-panfs-driver.yaml
```

#### 3.2.4 Execute CSI Driver and KMM Module Update

##### Deployment:
```bash
# Apply updated configurations for CSI driver components only
# (KMM module should already be updated in Stage 1 if applicable)
kubectl apply -f deploy/k8s/csi-panfs-driver.yaml

# Monitor rollout status for controller
kubectl rollout status deployment/csi-panfs-controller -n csi-panfs --timeout=300s

# Monitor rollout status for node daemonset
kubectl rollout status daemonset/csi-panfs-node -n csi-panfs --timeout=600s
```

#### 3.2.5 Validate CSI Driver and KMM Module Update

**CSI Driver Components Validation:**
```bash
# Verify all pods are in Ready state
kubectl wait --for=condition=Ready pod -l app=csi-panfs-controller -n csi-panfs --timeout=300s
kubectl wait --for=condition=Ready pod -l app=csi-panfs-node -n csi-panfs --timeout=300s

# Ensure all pods are running and volumes are mounted
kubectl get pods -n csi-panfs | grep -E "(Pending|Error|CrashLoopBackOff)"

# Controller logs
kubectl logs -n csi-panfs deployment/csi-panfs-controller -c csi-provisioner --tail=50
kubectl logs -n csi-panfs deployment/csi-panfs-controller -c csi-attacher --tail=50
kubectl logs -n csi-panfs deployment/csi-panfs-controller -c csi-resizer --tail=50

# Node logs
kubectl logs -n csi-panfs daemonset/csi-panfs-node -c csi-driver --tail=50
kubectl logs -n csi-panfs daemonset/csi-panfs-node -c node-driver-registrar --tail=50
```

**KMM Module Validation:**
```bash
# Verify KMM module is loaded and functional on all target nodes
kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.availableNumber}/{.status.moduleLoader.desiredNumber}'

# Confirm module loading on individual worker nodes
lsmod | grep panfs
```

**CRITICAL CHECKPOINT: Verify CSI Driver Components Before Proceeding:**
- [ ] All controller pods are in Ready state
- [ ] All node pods are in Ready state
- [ ] No errors in CSI driver logs
- [ ] CSI driver is properly registered
- [ ] All target nodes have successfully loaded the `panfs` module
- [ ] KMM module status shows `availableNumber` equals `desiredNumber`
- [ ] No KMM-related errors in logs (Worker Node events)

### 3.3 StorageClass Update

#### 3.3.1 Update StorageClass Manifest

Update the following placeholders in your StorageClass manifest according to your environment:

#### StorageClass Name

| Parameter | Description | Example |
|-----------|-------------|---------|
| `<STORAGE_CLASS_NAME>` | Storage Class identifier | `csi-panfs-storage-class` |

StorageClass is not a namespaced resource. But Namespace with the same name is used to keep secret with Realm access credentials.

#### Backed Realm Access Credentials

| Parameter | Description | Example |
|-----------|-------------|---------|
| `<REALM_ADDRESS>` | PanFS backend server address | `panfs.example.com` |
| `<REALM_USERNAME>` | PanFS Realm service account | `admin` |
| `<REALM_PASSWORD>` | Authentication password | `your-secure-password` |
| `<REALM_PRIVATE_KEY>` | Private key for key-based auth | Leave empty if not using |
| `<REALM_PRIVATE_KEY_PASSPHRASE>` | Passphrase for encrypted keys | Leave empty if no encryption |

#### Configuration of PV Binding Mode and Reclaim Policy

| Environment Type | Recommended Binding Mode | Recommended Reclaim Policy | Rationale |
|------------------|-------------------------|---------------------------|-----------|
| **Production** | `WaitForFirstConsumer` | `Retain` | Data safety + optimal scheduling |
| **Staging/UAT** | `WaitForFirstConsumer` | `Retain` | Production-like behavior |
| **Development** | `Immediate` | `Delete` | Fast iteration + cost efficiency |
| **CI/CD Testing** | `Immediate` | `Delete` | Speed + automatic cleanup |

> ⚠️ **IMPORTANT CONFIGURATION IMPACT NOTICE**
> 
> Changes to `volumeBindingMode` and `reclaimPolicy` only affect **NEW** PVCs created after the update.
> Existing PVs retain their original reclaim policy.

##### Volume Binding Mode Configuration

**WaitForFirstConsumer Mode:**
- **Advantages:**
  - Optimizes pod scheduling to nodes with available storage
  - Prevents volume creation until pod is scheduled
  - Better resource utilization in multi-zone clusters
- **Disadvantages:**
  - Slight delay in pod startup (waits for scheduling)
  - More complex troubleshooting for binding issues

**Immediate Mode:**
- **Advantages:**
  - Faster pod startup (volume pre-created)
  - Immediate feedback on storage availability
  - Simpler troubleshooting workflow
- **Disadvantages:**
  - May create volumes on nodes where pods cannot be scheduled
  - Less optimal resource utilization

##### Reclaim Policy Configuration

**Retain Policy:**
- **Advantages:**
  - Data protection - requires manual cleanup
  - Enables data recovery and forensic analysis
  - Prevents accidental data loss
- **Disadvantages:**
  - Manual storage management overhead
  - Potential storage resource waste
  - Requires operational procedures for cleanup

**Delete Policy:**
- **Advantages:**
  - Automatic cleanup and resource reclamation
  - Reduced administrative overhead
- **Disadvantages:**
  - Risk of accidental data loss
  - No recovery option after PVC deletion
  - Requires careful application design

#### 3.3.2 Apply StorageClass Configuration:

```bash
# Update the StorageClass manifest with your chosen parameters
# Edit deploy/k8s/csi-panfs-storage-class.yaml with appropriate values

# Apply the updated configuration
kubectl apply -f deploy/k8s/csi-panfs-storage-class.yaml
```

#### 3.3.3 Validate StorageClass:

```bash
# Verify StorageClass is available
kubectl get sc csi-panfs-storage-class

# Check StorageClass parameters
kubectl describe sc csi-panfs-storage-class

# Verify associated secret exists and is accessible
NAMESPACE=$(kubectl get sc csi-panfs-storage-class -o jsonpath="{.parameters.csi\.storage\.k8s\.io/provisioner-secret-namespace}")
SECRET_NAME=$(kubectl get sc csi-panfs-storage-class -o jsonpath="{.parameters.csi\.storage\.k8s\.io/provisioner-secret-name}")

# Check that the credentials to Realm applied correctly:
kubectl get secret -n $NAMESPACE $SECRET_NAME -o yaml
```

### 3.4 Post-Deployment Functional Tests

Create test resources using the following sample configuration:

```yaml
kind: Pod
apiVersion: v1
metadata:
  name: functional-test-pod
  labels:
    app: functional-test
spec:
  containers:
    - name: main
      image: busybox
      command:
        - sh
        - -c
        - |
          set -e
          mount | grep /data
          df -T /data | grep panfs
          echo "Volume mount to /data as type 'panfs' is successful"
          echo "some data written to volume" > /data/sample-file.txt
          grep "some data written to volume" /data/sample-file.txt
          echo "Functional test completed successfully"
      volumeMounts:
      - mountPath: "/data"
        name: functional-test-pod-volume
  volumes:
  - name: functional-test-pod-volume
    persistentVolumeClaim:
      claimName: functional-test-pvc
  restartPolicy: Never
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: functional-test-pvc
  labels:
    app: functional-test
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

**Verification:**

```bash
# Verify that resources created successfully
kubectl get po,pvc -l app=functional-test

# Inspect logs from pod
kubectl logs functional-test-pod
# ...
# Look for: Functional test completed successfully

# Clean up test workload and its PVC
kubectl delete po,pvc -l app=functional-test
```

## 4. Success Criteria

The upgrade is considered successful when all of the following conditions are met:

### 4.1 CSI Driver and KMM Module Update
- [ ] All controller pod containers are running without errors
- [ ] All node pod containers are running without errors
- [ ] CSI node pods are scheduled only on nodes with loaded KMM modules
- [ ] No critical errors in CSI driver logs
- [ ] KMM module successfully loaded on all designated nodes with correct kernel versions
- [ ] `availableNumber` equals `desiredNumber` in KMM module status
- [ ] All target worker nodes show panfs.ko module loaded via `lsmod`
- [ ] No KMM-related errors in operator logs

### 4.2 StorageClass Configuration
- [ ] PV created as requested by test PVC
- [ ] Test workload pod created successfully
- [ ] Test workload logs report that volume type is `panfs`
- [ ] Test workload logs report that write/read operations are successful
- [ ] StorageClass volumeBindingMode behaves as expected:
  - [ ] **WaitForFirstConsumer**: PVC remains Pending until pod is created
  - [ ] **Immediate**: PVC binds immediately upon creation
- [ ] StorageClass reclaimPolicy is correctly applied to new PVs
- [ ] No unexpected volume provisioning delays or failures
- [ ] Storage class parameters match intended configuration

## 5. Troubleshooting

### 5.1 Common Issues

#### 5.1.1 Image Pull Failures
```bash
# Check if image pull secret exists
kubectl get secrets -n csi-panfs | grep dockerconfigjson

# Verify registry credentials
kubectl get secret <image-pull-secret> -n csi-panfs -o yaml

# Test image pull manually
docker pull <csi-driver-image>
```

#### 5.1.2 KMM Module Load Failures
```bash
# Check module CR status
kubectl get module panfs -n csi-panfs -o yaml

# Verify availableNumber vs desiredNumber
kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.availableNumber}/{.status.moduleLoader.desiredNumber}'

# Check kernel compatibility
kubectl get nodes -o jsonpath='{.items[*].status.nodeInfo.kernelVersion}'

# Check if module loaded successfully on nodes
for node in $(kubectl get nodes -l node-role.kubernetes.io/worker -o name); do
  echo "=== $node ==="
  kubectl debug $node -it --image=busybox -- chroot /host lsmod | grep panfs
done

# Check KMM operator logs for errors
kubectl logs -n kmm-operator-system -l control-plane=controller --tail=100

# Manual verification on worker nodes
# SSH to worker node and check module status
# ssh <worker-node>
# sudo lsmod | grep panfs
# sudo dmesg | grep panfs | tail -100
```

#### 5.1.3 Stage Sequencing Issues
```bash
# Verify completion of KMM module update before proceeding to CSI driver update
kubectl wait --for=jsonpath='{.status.moduleLoader.availableNumber}'=$(kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.desiredNumber}') module/panfs -n csi-panfs --timeout=600s

# Check if CSI node pods are trying to start before KMM modules load
kubectl get pods -n csi-panfs -l app=csi-panfs-node -o wide
kubectl describe pods -n csi-panfs -l app=csi-panfs-node | grep -A 5 "Events:"
```

#### 5.1.4 Pod Scheduling Issues
```bash
# Check node labels
kubectl get nodes --show-labels

# Verify DaemonSet node selector
kubectl get daemonset csi-panfs-node -n csi-panfs -o jsonpath='{.spec.template.spec.nodeSelector}'

# Verify DaemonSet node tolerations
kubectl get daemonset csi-panfs-node -n csi-panfs -o jsonpath='{.spec.template.spec.tolerations}'

# Get Worker Node(s) taints
kubectl get node <NODE_NAME> -o jsonpath='{.spec.taints}'

# Check node resources
kubectl describe nodes | grep -A 5 "Allocated resources"
```

#### 5.1.5 Volume Mount Failures During Upgrade
```bash
# Check if any pods are still using PanFS volumes
kubectl get pods --all-namespaces -o json | 
  jq -r '
    .items[] | 
    select(.spec.volumes[]?.persistentVolumeClaim or .spec.volumes[]?.csi.driver=="com.vdura.csi.panfs") | 
    "\(.metadata.namespace)/\(.metadata.name)"
  '

# Force delete stuck pods if necessary
kubectl delete pod <pod-name> -n <namespace> --force --grace-period=0

# Check for stuck PVC mounts
kubectl describe pv | grep -A 5 -B 5 "com.vdura.csi.panfs"

# Check volume attachment status
kubectl get volumeattachment
kubectl describe volumeattachment <attachment-name>
```

#### 5.1.6 Authentication Issues
```bash
# Check storage class configuration
kubectl get sc | grep com.vdura.csi.panfs
kubectl describe sc <storage-class-name>

# Get secret details from storage class
NAMESPACE=$(kubectl get sc <SC_NAME> -o jsonpath="{.parameters.csi\.storage\.k8s\.io/provisioner-secret-namespace}")
SECRET_NAME=$(kubectl get sc <SC_NAME> -o jsonpath="{.parameters.csi\.storage\.k8s\.io/provisioner-secret-name}")

# Verify secret exists and check credentials
kubectl get secret -n $NAMESPACE $SECRET_NAME
kubectl get secret -n $NAMESPACE $SECRET_NAME -o jsonpath='{.data.realm_ip}' | base64 -d
kubectl get secret -n $NAMESPACE $SECRET_NAME -o jsonpath='{.data.user}' | base64 -d

# Test connectivity from a node
kubectl run test-connectivity --image=busybox --rm -it -- ping <realm-ip>
```

#### 5.1.7 Workload Restoration Issues
```bash
# Check PVC status after restoration
kubectl get pvc --all-namespaces | grep -v Bound

# Verify storage class availability
kubectl get storageclass csi-panfs-storage-class

# Check for permission issues
kubectl auth can-i create persistentvolumeclaims --as=system:serviceaccount:<namespace>:<serviceaccount>
```

#### 5.1.8 Data Integrity Issues
```bash
# Verify volume mounts in pods
kubectl exec <pod-name> -n <namespace> -- mount | grep panfs

# Check file system health
kubectl exec <pod-name> -n <namespace> -- df -h /mnt/panfs

# Test read/write access
kubectl exec <pod-name> -n <namespace> -- touch /mnt/panfs/test-file
kubectl exec <pod-name> -n <namespace> -- rm /mnt/panfs/test-file
```

### 5.2 Diagnostic Commands

```bash
# Get comprehensive status
kubectl get all -n csi-panfs

# Check CSI driver registration
kubectl get csidrivers

# Inspect StorageClass configuration
kubectl describe sc <storageclass-name>

# View recent events in CSI Driver workloads
kubectl get events -n csi-panfs --sort-by='.lastTimestamp'

# Get CSI Driver Controller logs
kubectl logs -n csi-panfs -l app=csi-panfs-controller --all-containers

# Get CSI Driver Node logs
kubectl logs -n csi-panfs -l app=csi-panfs-node --all-containers

# Two-Stage Upgrade Diagnostics
# Check KMM module status
kubectl get module panfs -n csi-panfs -o yaml

# Verify module loading alignment between KMM and CSI components
echo "=== KMM Module Status ==="
kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.availableNumber}/{.status.moduleLoader.desiredNumber}'

echo "=== CSI Node Pod Placement ==="
kubectl get pods -n csi-panfs -l app=csi-panfs-node -o custom-columns=NAME:.metadata.name,NODE:.spec.nodeName,STATUS:.status.phase

echo "=== Nodes with PanFS Module Loaded ==="
kubectl get nodes -l node-role.kubernetes.io/worker -o name | while read node; do
  echo -n "$node: "
  kubectl debug $node -it --image=busybox -- chroot /host lsmod | grep panfs >/dev/null 2>&1 && echo "✓ Loaded" || echo "✗ Not Loaded"
done
```

### 5.3 Rollback Procedure

If the upgrade fails at any stage, follow these steps to rollback:

#### 5.3.1 Determine Rollback Scope

**Identify which component failed:**
- **KMM Module Failure**: Rollback KMM module and potentially CSI components
- **CSI Driver Failure**: Rollback CSI components only

#### 5.3.2 Stop All Workloads (if not already done for KMM rollback)
```bash
# Only required if rolling back KMM module components
# Ensure all workloads using PanFS volumes are stopped
kubectl scale deployment <deployment-name> --replicas=0 -n <namespace>
kubectl delete pod <standalone-pod-name> -n <namespace>
```

#### 5.3.3 Rollback CSI Driver Configuration
```bash
# Navigate to backup directory
cd panfs-backup-*

# Restore previous configuration (includes both KMM and CSI components)
kubectl apply -f csi-panfs-controller-backup.yaml
kubectl apply -f csi-panfs-node-backup.yaml
kubectl apply -f csi-panfs-config-backup.yaml

# Monitor rollback status for CSI components
kubectl rollout status deployment/csi-panfs-controller -n csi-panfs --timeout=300s
kubectl rollout status daemonset/csi-panfs-node -n csi-panfs --timeout=600s

# Monitor KMM module rollback (if KMM was involved in the upgrade)
kubectl get module panfs -n csi-panfs -w

# Verify KMM module rollback completion
kubectl wait --for=jsonpath='{.status.moduleLoader.availableNumber}'=$(kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.desiredNumber}') module/panfs -n csi-panfs --timeout=600s
```

#### 5.3.4 Validate Rollback Success

```bash
# Test CSI driver functionality
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rollback-test-pvc
  namespace: default
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-panfs-storage-class
EOF

# Verify PVC creation
kubectl get pvc rollback-test-pvc
kubectl delete pvc rollback-test-pvc
```

## 6. Support and Escalation

For issues during upgrade:

1. **Collect Diagnostic Information**
   - Use the diagnostic commands provided in this guide
   - Gather logs from CSI driver components
   - Document the exact error messages and symptoms

2. **Review Documentation**
   - [PanFS CSI Driver Overview](./Overview.md) - Detailed information about PanFS CSI driver architecture
   - [Usage Guide](./usage-guide.md) - Workload deployment examples and best practices
   - [Troubleshooting Guide](./Troubleshooting.md) - Comprehensive troubleshooting for common issues
   - [Diagnostics Guide](./Diagnostic.md) - Advanced diagnostic procedures and log analysis
   - Product release notes for breaking changes or known issues

3. **Contact Support**
   - Contact VDURA support with detailed logs and environment information
   - Include cluster information, error logs, and steps already attempted
   - Provide backup files and configuration details when relevant

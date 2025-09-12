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

# PanFS CSI Driver: Usage Guide

This guide provides examples for deploying workloads using the **PanFS CSI Driver** with dynamic and static provisioning. It demonstrates how to use PersistentVolumeClaims (PVCs) with the PanFS StorageClass in various scenarios, including ReadWriteMany (RWX), ReadWriteOnce (RWO), single pods, StatefulSets, and static provisioning.

---

## Overview

The PanFS CSI Driver enables Kubernetes to manage PanFS-backed persistent volumes. This guide assumes the driver and its dependencies are installed and configured. The examples use YAML manifests from the `examples/k8s` folder to illustrate common use cases.

---

## Prerequisites

Ensure the following are in place before proceeding:

- **KMM Engine**: Installed and configured to load PanFS kernel modules on worker nodes
- **PanFS CSI Driver**: Deployed and operational in the `csi-panfs` namespace
- **PanFS StorageClass**: Configured to connect to the PanFS Realm backend
- **kubectl**: Installed for interacting with the Kubernetes cluster

> **Tip**: Refer to the [Architecture Overview](./architecture-overview.md) for setup instructions.

---

## Assumptions

- All examples use the `csi-panfs-storage-class` StorageClass for dynamic provisioning.
- YAML files are located in the `examples/k8s` folder of the repository.
- Commands are run with appropriate cluster permissions (e.g., cluster-admin).

---

## Usage Scenarios

The following sections demonstrate common workload deployment scenarios using the PanFS CSI Driver. Each includes steps to deploy, validate, and understand the behavior of the workload.

### 1. Deployment with ReadWriteMany (RWX)

This scenario deploys a workload where multiple pods share a single PanFS volume in **ReadWriteMany (RWX)** mode, ideal for applications requiring shared storage.

#### Deploy the Workload

```bash
kubectl apply -f examples/k8s/dynamic-provisioning/deployment-with-rwx.yaml
```
Expected output:
```
deployment.apps/sample-deployment-with-rwx created
persistentvolumeclaim/csi-pvc-rwx created
```

#### Validate the Deployment

- Check pod status (should show 5 running pods):
  ```bash
  kubectl get pods
  ```
  Expected output:
  ```
  NAME                                         READY   STATUS    RESTARTS   AGE
  sample-deployment-with-rwx-xxxxxxxxxx-xxxxx   1/1     Running   0          15m
  ... (total 5 pods)
  ```

- Check PVC status:
  ```bash
  kubectl get pvc
  ```
  Expected output:
  ```
  NAME          STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS                AGE
  csi-pvc-rwx   Bound    pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   5Gi        RWX            csi-panfs-storage-class     16m
  ```

- Check PV status:
  ```bash
  kubectl get pv
  ```
  Expected output:
  ```
  NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                 STORAGECLASS                AGE
  pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   5Gi        RWX            Delete           Bound    default/csi-pvc-rwx   csi-panfs-storage-class     16m
  ```

#### Notes
- All pods mount the same `/data` path as a shared PanFS volume in RWX mode.
- Use RWX for workloads like content management systems or shared data stores.

### 2. Deployment with ReadWriteOnce (RWO)

This scenario deploys a workload where only one pod at a time can access a PanFS volume in **ReadWriteOnce (RWO)** mode. Additional pods remain in `Pending` state until the volume is released.

#### Deploy the Workload

```bash
kubectl apply -f examples/k8s/dynamic-provisioning/deployment-with-rwo.yaml
```
Expected output:
```
deployment.apps/sample-deployment-with-rwo created
persistentvolumeclaim/csi-pvc-rwo created
```

#### Validate the Deployment

- Check pod status (one pod running, others pending):
  ```bash
  kubectl get pods
  ```
  Expected output:
  ```
  NAME                                         READY   STATUS    RESTARTS   AGE
  sample-deployment-with-rwo-xxxxxxxxxx-xxxxx   1/1     Running   0          10s
  sample-deployment-with-rwo-xxxxxxxxxx-xxxxx   0/1     Pending   0          10s
  ```

- Check PVC status:
  ```bash
  kubectl get pvc
  ```
  Expected output:
  ```
  NAME          STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS                AGE
  csi-pvc-rwo   Bound    pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   5Gi        RWO            csi-panfs-storage-class     10s
  ```

- Check PV status:
  ```bash
  kubectl get pv
  ```
  Expected output:
  ```
  NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                 STORAGECLASS                AGE
  pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   5Gi        RWO            Delete           Bound    default/csi-pvc-rwo   csi-panfs-storage-class     10s
  ```

#### Notes
- Only one pod can mount the `/data` volume at a time due to RWO restrictions.
- Use RWO for workloads like databases requiring exclusive access.

### 3. Single Pod with RWO

This scenario deploys a single pod mounting a PanFS volume in **ReadWriteOnce (RWO)** mode, suitable for simple applications.

#### Deploy the Workload

```bash
kubectl apply -f examples/k8s/dynamic-provisioning/single-pod.yaml
```
Expected output:
```
pod/busybox-sleep created
persistentvolumeclaim/csi-pvc-2 created
```

#### Validate the Deployment

- Check pod status:
  ```bash
  kubectl get pod busybox-sleep
  ```
  Expected output:
  ```
  NAME            READY   STATUS    RESTARTS   AGE
  busybox-sleep   1/1     Running   0          5m
  ```

- Check PVC status:
  ```bash
  kubectl get pvc csi-pvc-2
  ```
  Expected output:
  ```
  NAME         STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS                AGE
  csi-pvc-2    Bound    pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   5Gi        RWO            csi-panfs-storage-class     5m
  ```

#### Notes
- The pod mounts `/data-1` as a PanFS volume in RWO mode.
- Ideal for lightweight, single-instance applications.

### 4. StatefulSet with RWO

This scenario deploys a **StatefulSet** where each pod gets a unique PanFS volume in **ReadWriteOnce (RWO)** mode, suitable for stateful applications like databases.

#### Deploy the Workload

```bash
kubectl apply -f examples/k8s/dynamic-provisioning/stateful-set.yaml
```
Expected output:
```
statefulset.apps/sample-statefulset created
```

#### Validate the Deployment

- Check pod status:
  ```bash
  kubectl get pods -l app=sample-statefulset
  ```
  Expected output:
  ```
  NAME                   READY   STATUS    RESTARTS   AGE
  sample-statefulset-0   1/1     Running   0          2m
  sample-statefulset-1   1/1     Running   0          2m
  sample-statefulset-2   1/1     Running   0          2m
  ```

- Check PVC status:
  ```bash
  kubectl get pvc -l app=sample-statefulset
  ```
  Expected output:
  ```
  NAME                        STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS                AGE
  csi-volume-sts-10gb-0       Bound    pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   10Gi       RWO            csi-panfs-storage-class     2m
  csi-volume-sts-10gb-1       Bound    pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   10Gi       RWO            csi-panfs-storage-class     2m
  csi-volume-sts-10gb-2       Bound    pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   10Gi       RWO            csi-panfs-storage-class     2m
  ```

#### Notes
- Each pod in the StatefulSet gets a unique `/data` volume in RWO mode.
- Use for stateful applications requiring persistent, pod-specific storage.

### 5. Static Provisioning

This scenario demonstrates **static provisioning**, where a PersistentVolume (PV) and PersistentVolumeClaim (PVC) are manually created and bound to a pod, bypassing dynamic provisioning.

#### Deploy the Workload

```bash
kubectl apply -f examples/k8s/static-provisioning/pod-with-static-volume.yaml
```
Expected output:
```
pod/app created
persistentvolumeclaim/panfs-static-volume-claim created
persistentvolume/panfs-static-volume-pv created
```

#### Validate the Deployment

- Check pod status:
  ```bash
  kubectl get pod app
  ```
  Expected output:
  ```
  NAME   READY   STATUS    RESTARTS   AGE
  app    1/1     Running   0          1m
  ```

- Check PVC status:
  ```bash
  kubectl get pvc panfs-static-volume-claim
  ```
  Expected output:
  ```
  NAME                        STATUS   VOLUME                    CAPACITY   ACCESS MODES   STORAGECLASS   AGE
  panfs-static-volume-claim   Bound    panfs-static-volume-pv    10Gi       RWO            <none>         1m
  ```

- Check PV status:
  ```bash
  kubectl get pv panfs-static-volume-pv
  ```
  Expected output:
  ```
  NAME                     CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                           STORAGECLASS   AGE
  panfs-static-volume-pv   10Gi       RWO            Retain           Bound    default/panfs-static-volume-claim   <none>     1m
  ```

#### Notes
- The pod mounts `/data` from a statically provisioned PanFS volume.
- Use static provisioning when precise control over volume configuration is needed.

---

## Troubleshooting

- **Pods in Pending State**:
  - Check for volume binding issues: `kubectl describe pod <pod-name>`.
  - Verify PVC and PV status: `kubectl describe pvc` or `kubectl describe pv`.
  - Ensure the PanFS StorageClass is correctly configured.

- **Volume Not Mounting**:
  - Check pod logs: `kubectl logs <pod-name>`.
  - Verify KMM module status: `kubectl get module panfs -n csi-panfs`.
  - Confirm PanFS Realm connectivity and credentials.

- **Additional Resources**:
  - [examples/k8s](../examples/k8s): Sample YAML manifests.
  - [kmm.md](./kmm.md): KMM configuration and troubleshooting.
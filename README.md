## VDURA PanFS Container Storage Interface Driver

The official CSI Driver for integrating **PanFS** storage with Container Orchestration Systems like Kubernetes.

[![Driver deployment](https://github.com/panasasinc/panfs-container-storage-interface-oss/actions/workflows/main.yaml/badge.svg)](https://github.com/panasasinc/panfs-container-storage-interface-oss/actions/workflows/main.yaml)
[![Vulnerability Scan](https://github.com/panasasinc/panfs-container-storage-interface-oss/actions/workflows/vulnerability.yaml/badge.svg)](https://github.com/panasasinc/panfs-container-storage-interface-oss/actions/workflows/vulnerability.yaml)

## Documentation

For more details on specific topics, refer to the following documents:

| Document | Description |
|:---------|:------------|
| [Overview](./docs/Overview.md) | Architecture of the driver: controller and node services, sidecars, KMM integration, and operational workflow |
| [Usage Guide](./docs/Usage-Guide.md) | Deploying workloads with PanFS volumes: RWX/RWO deployments, StatefulSets, and static provisioning examples |
| [Tenant Network](./charts/tenant-network/Readme.md) | Putting worker nodes on the network of a PanFS tenant, and marking the nodes that are ready |
| [KMM](./docs/KMM.md) | Installing and configuring the Kernel Module Management (KMM) engine to load the PanFS kernel module on worker nodes |
| [SELinux](./docs/SELinux.md) | Configuring SELinux across the OS, container runtime, Kubernetes, and CSI driver layers |
| [Upgrade](./docs/Upgrade.md) | Upgrading the CSI driver, KMM module, and StorageClass, including validation and rollback procedures |
| [Troubleshooting](./docs/Troubleshooting.md) | Diagnosing and resolving common issues: installation, volume mounting, KMM, authentication, and networking |
| [Diagnostic](./docs/Diagnostic.md) | Collecting diagnostic data to submit to VDURA Technical Support |

## Contents

* [Documentation](#documentation)
* [Overview](#overview)
* [Capabilities](#capabilities)
  * [CSI Services](#csi-services)
  * [Volume Features](#volume-features)
  * [PanFS mount options](#panfs-mount-options)
  * [Storage class customization](#storage-class-customization-eg-volume-create-parameters)
* [Compatibility](#compatibility)
* [Getting started](#getting-started)
  * [Prerequisites](#prerequisites)
    * [Software Requirements](#software-requirements)
    * [System Requirements](#system-requirements)
    * [Access Requirements](#access-requirements)
    * [PanFS Realm Requirements](#panfs-realm-requirements)
  * [CSI Driver Installation - Quick Start](#csi-driver-installation---quick-start)
    * [1. Ensure dependencies are installed](#1-ensure-dependencies-are-installed)
    * [2. Deploy CSI driver with KMM Module](#2-deploy-csi-driver-with-kmm-module)
      * [2.1 Prerequisites](#21-prerequisites)
      * [2.2 Configure and Deploy](#22-configure-and-deploy)
    * [3. Configure and deploy StorageClass](#3-configure-and-deploy-storageclass)
    * [4. Put the nodes on the tenant network](#4-put-the-nodes-on-the-tenant-network)
* [Deploying Workloads](#deploying-workloads)
* [CSI Driver Uninstallation](#csi-driver-uninstallation)
* [API Documentation](#api-documentation)
  * [How to Use pkgsite](#how-to-use-pkgsite)
* [Testing](#testing)
  * [Kubernetes CSI Sanity tests](#kubernetes-csi-sanity-tests)
    * [Running CSI Sanity Tests with Docker Compose](#running-csi-sanity-tests-with-docker-compose)
    * [Notes](#notes)
  * [Kubernetes CSI E2E tests](#kubernetes-csi-e2e-tests)
    * [Provided E2E Test Setup](#provided-e2e-test-setup)
    * [Running E2E Tests](#running-e2e-tests)
    * [Notes](#notes-1)

## Overview

The Container Storage Interface (CSI) is a standard for exposing arbitrary block and file storage systems to containerized workloads on Container Orchestration Systems like Kubernetes. Using CSI, third-party storage providers can write and deploy plugins exposing new storage systems in Kubernetes without ever having to touch the core Kubernetes code. The Container Storage Interface was designed to help Container Orchestrators replace their existing in-tree storage driver mechanisms - especially vendor-specific plugins.

The PanFS CSI driver allows you to use PanFS volumes in a Kubernetes cluster. It is possible to use volumes from multiple realms within a single CSI driver instance. See the [Deploying Workloads](#deploying-workloads) section for usage examples.

For more details, please see the [Overview documentation](./docs/Overview.md).

## Capabilities

### CSI Services

The driver implements the core CSI services for lifecycle management:

  * **Identity Service**: Used for plugin discovery (`GetPluginInfo`) and health checks (`Probe`).
  * **Controller Service**: Provides full lifecycle management, including:
      * `CreateVolume`
      * `DeleteVolume`
      * `ControllerExpandVolume`
  * **Node Service**: Handles volume attachment, mount, and unmount operations.
  * **Online Volume Expansion**: Supports **resizing volumes** while they are attached and in use by a pod.

### Volume Features

The driver exposes advanced PanFS features through CSI:

  * **Volume Quota Management**: Supports configurable **soft and hard quotas** for PanFS volumes.
  * **Multiple Volume Layouts**: Provisioning support for various PanFS layouts (e.g., `raid6+`, `raid5+`, `raid10+`).
  * **Robust Operation**: Includes extensive parameter validation, unit conversion (e.g., GB/bytes), and mapping of PanAPI errors to CSI-compliant error codes.
  * **Static Provisioning**: A PersistentVolume can name an existing PanFS volume, or a directory inside it.

The driver manages volumes through **PanAPI**, the REST API of the PanFS realm, with a service account token.
Volume encryption is not supported with PanAPI.

### PanFS mount options
The PanFS CSI driver supports custom mount options for PanFS volumes. You can specify mount options in the PersistentVolume manifest using the `mountOptions` field. This allows you to customize the mount behavior according to your requirements.

#### Storage class customization (e.g. volume create parameters)
The PanFS CSI driver supports custom parameters for volume creation in the StorageClass manifest. You can specify parameters such as `storageset`, `layout`, etc., to customize the behavior of the created volumes.
For the full list of supported parameters, see [PanFS Specific Volume Parameters](./charts/storageclass/Readme.md#panfs-specific-volume-parameters).

## Compatibility

This matrix defines the supported versions of Kubernetes and the CSI Specification for each PanFS CSI Driver release:

| PanFS CSI Version | Kubernetes Version | CSI Spec Version |
|:------------------|:-------------------|:-----------------|
| **1.0.0**         | 1.30.1+            | 1.7.0            |
| **1.0.1+**        | 1.30.1+            | 1.11.0           |
| **1.1.0**         | 1.30.1+            | 1.11.0           |
| **1.2.0**         | 1.30.1+            | 1.11.0           |
| **1.2.1**         | 1.30.1+            | 1.11.0           |
| **1.2.2**         | 1.30.1+            | 1.11.0           |
| **1.2.3**         | 1.30.1+            | 1.11.0           |
| **1.2.4**         | 1.30.1+            | 1.11.0           |
| **1.2.5**         | 1.30.1+            | 1.11.0           |
| **1.2.6**         | 1.30.1+            | 1.14.0           |
| **2.0.0**         | 1.30.1+            | 1.14.0           |

## Getting started

### Prerequisites

- Deploy using the kubectl CLI tool
- PanFS CSI driver requires the panfs kernel module to be installed and configured on the host nodes
- Ensure that all required ports are open to allow PanFS to perform mounts (see details in the user guide)


#### Software Requirements

- **Kubernetes Cluster**: Version 1.30.1+ with cluster-admin privileges
- **kubectl**: Compatible with your Kubernetes cluster version. See [Kubernetes docs](https://kubernetes.io/docs/tasks/tools/)

#### System Requirements

- **Linux Kernel**: Compatible with PanFS kernel module (see compatibility matrix below)
- **Container Runtime**: Docker, containerd, or CRI-O
- **Network**: Ensure required ports are open for PanFS communication (refer to PanFS documentation)

#### Access Requirements

- **Container Registry Access**: Credentials for pulling/pushing driver images
- **PanFS Realm Access**: A PanAPI service account token and network connectivity to the PanFS realm
  (see [PanFS Realm Requirements](#panfs-realm-requirements))

#### PanFS Realm Requirements

The driver talks to the realm through PanAPI. It does not use SSH.

- **PanAPI address**: The CSI controller must reach PanAPI over HTTPS (or HTTP, when the address starts with `http://`).
- **Service account token**: A token of a PanAPI service account. The driver works inside the tenant of this token.
- **Storageset**: At least one storageset must be assigned to the tenant.
- **Token permissions**, all for its own tenant:

  | Permission                                         | Used for                                          |
  |:---------------------------------------------------|:--------------------------------------------------|
  | Read the tenant and its storageset assignments     | Choosing the storageset of a new volume           |
  | List, read, create, update and delete volumes      | Volume provisioning, expansion and deletion       |
  | List subnets, read the mount recipe of a subnet    | Finding the address that the nodes mount from     |

  When the token may not read the subnets, the volume is still created, and the nodes mount from the host of the PanAPI address.
- **Tenant network**: A volume of a tenant is reachable only from the network of that tenant.
  Every worker node must be attached to that network before it mounts a volume.
  The driver finds the mount address of the network, but it does not attach the nodes.
  The [PanFS Tenant Network operator](./charts/tenant-network/Readme.md) attaches the nodes that you label for the tenant,
  and it marks the nodes that are ready; see [Tenant network](#4-put-the-nodes-on-the-tenant-network).
  It needs these permissions too: list, read, create and delete attachments of a subnet.

### CSI Driver Installation - Quick Start

For experienced users who want to deploy quickly, here's the essential command sequence:

#### 1. Ensure dependencies are installed

Ensure the following Kubernetes ecosystem tools are installed in your cluster:

  * **cert-manager**: Follow the [installation guide](https://cert-manager.io/docs/installation/).
  * **KMM (Kernel Module Manager)**: Follow the [installation guide](https://kmm.sigs.k8s.io/documentation/install/).

---

#### 2. Deploy CSI driver with KMM Module

Create the namespace, configure registry credentials, and deploy the driver using the Kubernetes manifest file.

##### 2.1 Prerequisites
```bash
# Create PanFS CSI Driver namespace
kubectl create namespace csi-panfs

# Configure secret for Private Registry with PanFS DFC KMM images
# REPLACE placeholders with your actual registry settings:
kubectl create secret docker-registry <your-secret-name> \
  --docker-server=<your-registry-server> \
  --docker-username=<your-username> \
  --docker-password=<your-password> \
  --docker-email=<your-email> \
  --namespace=csi-panfs
```

##### 2.2 Configure and Deploy

This will deploy CSI driver components and the KMM module.

Please update the settings in [deploy/k8s/csi-driver/template-csi-panfs.yaml](deploy/k8s/csi-driver/template-csi-panfs.yaml) according to your cluster specification and available image tags in your private registry:

Edit the deployment manifest at [deploy/k8s/csi-driver/template-csi-panfs.yaml](deploy/k8s/csi-driver/template-csi-panfs.yaml) to configure essential settings:

  * `<IMAGE_PULL_SECRET_NAME>`: The name of the secret created above.
  * `<PANFS_DFC_KMM_PRIVATE_REGISTRY>`: The URL of your private registry hosting the DFC/KMM images.
  * `<DFC_RELEASE_VERSION>`: The specific version tag of the DFC release you are deploying.

> **Note:** Review other settings relevant to your Kubernetes infrastructure, such as:
> - `replicas`
> - `tolerations`
> - `nodeSelector`
> - etc.

Once configured, deploy the driver and KMM module:

```bash
kubectl apply -f deploy/k8s/csi-driver/template-csi-panfs.yaml
```

The expected output will look like:
```bash
poddisruptionbudget.policy/csi-panfs-controller-pdb created
serviceaccount/csi-panfs-controller created
serviceaccount/csi-panfs-node created
clusterrole.rbac.authorization.k8s.io/csi-panfs-controller-provisioner created
clusterrole.rbac.authorization.k8s.io/csi-panfs-controller-attacher created
clusterrole.rbac.authorization.k8s.io/csi-panfs-controller-resizer created
clusterrole.rbac.authorization.k8s.io/csi-panfs-node created
clusterrolebinding.rbac.authorization.k8s.io/csi-panfs-controller-provisioner-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/csi-panfs-controller-attacher-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/csi-panfs-controller-resizer-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/csi-panfs-node-rolebinding created
role.rbac.authorization.k8s.io/csi-panfs-controller-provisioner created
role.rbac.authorization.k8s.io/csi-panfs-controller-attacher created
rolebinding.rbac.authorization.k8s.io/csi-panfs-controller-provisioner-rolebinding created
rolebinding.rbac.authorization.k8s.io/csi-panfs-controller-attacher-rolebinding created
daemonset.apps/csi-panfs-node created
deployment.apps/csi-panfs-controller created
csidriver.storage.k8s.io/com.vdura.csi.panfs created
module.kmm.sigs.x-k8s.io/panfs created
```

**Validate Deployment**:

Check the status of the deployed components:

  * **CSI Driver Registration**:
    ```bash
    kubectl get csidrivers | grep panfs
    ```
  * **Controller Deployment**: (Should show **`READY`** equal to **`UP-TO-DATE`**)
    ```bash
    kubectl get deployment csi-panfs-controller -n csi-panfs
    ```
  * **Node DaemonSet**: (Should show **`READY`** equal to **`DESIRED`**)
    ```bash
    kubectl get daemonset csi-panfs-node -n csi-panfs
    ```
  * **KMM Module Status**:
    ```bash
    kubectl get module panfs -n csi-panfs -o yaml | sed -n '/status:$/,/^$/p'
    ```

---

#### 3. Configure and deploy StorageClass

The StorageClass defines the provisioned volume properties and provides PanFS Realm access credentials.

##### 3.1 Configure PanFS Realm Access Secret

Edit [deploy/k8s/storage-class/template-secret-in-driver-ns.yaml](deploy/k8s/storage-class/template-secret-in-driver-ns.yaml) to replace the following placeholders with your PanFS Realm details.

  * `<STORAGE_CLASS_NAME>`: The desired name for your StorageClass (e.g., `csi-panfs-storage-class`).
  * `<REALM_API>`: The PanAPI address of the PanFS realm: a host name (HTTPS is used) or a full `https://` or `http://` URL
    (e.g., `panfs.example.com`).
  * `<REALM_TOKEN>`: The PanAPI service account token. See [PanFS Realm Requirements](#panfs-realm-requirements) for its permissions.
  * `<STORAGESET>`: The id or the name of the storageset that holds the new volumes. Remove the line to use the first storageset that is assigned to the tenant.

If the realm certificate is signed by a private CA, add the key `ca_cert` with the PEM certificate of that CA to the Secret.
Without it, the driver uses the system trust store. For a self-signed certificate, `ca_cert` is that certificate itself.
If the certificate cannot be verified at all, for example a self-signed certificate that does not name the realm address,
add the key `insecure_skip_tls_verify` with the value `"true"`. The driver then does not verify the realm certificate,
so a host that poses as the realm can read the token. Use it for test realms only.

##### 3.2 Setting the Default StorageClass

To designate this StorageClass as the default option for volume provisioning in your Kubernetes cluster, add the following annotation to the metadata section of its manifest:

```yaml
metadata:
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
```

##### 3.3 Deployment and Validation

**Deploy PanFS Storage Class:**

```bash
kubectl apply -f deploy/k8s/storage-class/template-secret-in-driver-ns.yaml
```

The expected output will look like this:
```bash
namespace/csi-panfs-storage-class created
secret/csi-panfs-storage-class created
storageclass.storage.k8s.io/csi-panfs-storage-class created
```

**Validate StorageClass**:

```bash
kubectl get sc csi-panfs-storage-class
```
Expected output:
```
NAME                      PROVISIONER           RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
csi-panfs-storage-class   com.vdura.csi.panfs   Delete          WaitForFirstConsumer   true                   2m
```

If the StorageClass is missing or misconfigured, verify the configuration variables and check `kubectl logs` for errors.

---

#### 4. Put the nodes on the tenant network

Skip this step when the worker nodes already reach the mount address of the tenant.

A volume of a tenant is reachable only from the network of that tenant.
The PanFS Tenant Network operator puts the nodes that you choose on that network. Two node labels are involved:

| Label                                        | Who sets it        | Meaning                                      |
|:---------------------------------------------|:-------------------|:---------------------------------------------|
| `panfs.vdura.com/tenant=<tenant>`            | You                | The node is meant for this tenant            |
| `panfs.vdura.com/tenant-network=<namespace>` | Tenant network pod | The network of the tenant works on this node |

```bash
# 1. Install the operator, once per cluster
kubectl create namespace panfs-tenant-network
kubectl apply -f deploy/k8s/tenant-network/panfs-tenant-network-operator.yaml

# 2. Label the nodes of the tenant
kubectl label node <NODE_NAME> panfs.vdura.com/tenant=<TENANT>

# 3. Ask for the tenant network, in the namespace of the Secret that holds the PanAPI address and the token.
#    In deploy/k8s/tenant-network/template-tenantnetwork.yaml, replace <NAME>, <TENANT_NAMESPACE>,
#    <SECRET_NAME> and <TENANT> first.
kubectl apply -f deploy/k8s/tenant-network/template-tenantnetwork.yaml

# 4. The TenantNetwork must be Ready; its status lists the nodes, and its events say what happened
kubectl get tenantnetworks -A
kubectl -n <TENANT_NAMESPACE> describe tenantnetwork <NAME>
```

A ready node has the label `panfs.vdura.com/tenant-network=<TENANT_NAMESPACE>`: the value is the namespace of the `TenantNetwork`.

With Helm, step 3 is the value `tenant` of the [StorageClass chart](./charts/storageclass/Readme.md#tenant-network):
it creates the `TenantNetwork`, and it restricts the StorageClass to the ready nodes.

The requirements (Calico setting, namespace, token permissions), the Helm installation, and the way to keep workloads
on the ready nodes are described in the [operator documentation](./charts/tenant-network/Readme.md).

## Deploying Workloads

Refer to [Usage-Guide.md](./docs/Usage-Guide.md) for YAML manifests and examples of deploying workloads with the PanFS CSI Driver.

---

## CSI Driver Uninstallation

A complete uninstallation ensures that all associated resources—workloads, PersistentVolumes, the StorageClass, driver components, and kernel modules—are cleanly removed from the Kubernetes cluster.

### Identify and Terminate Dependent Workloads

Before deleting the driver, you must ensure no running applications are actively using PanFS volumes.

1.  **Identify all PanFS PersistentVolumes (PVs):**
    This command lists the names of all PVs provisioned by the PanFS CSI driver.

    ```bash
    kubectl get pv -o jsonpath='{.items[?(@.spec.csi.driver=="com.vdura.csi.panfs")].metadata.name}'
    ```

2.  **Delete PVCs and Workloads:**
    **Manually delete** all **PersistentVolumeClaims (PVCs)** and the **workloads** (Pods, Deployments, StatefulSets) that rely on the identified PVs.

    > ⚠️ **Important:** Deleting the PVCs will trigger the deletion of the associated **PersistentVolumes (PVs)**, which in turn will cause the CSI Controller to delete the corresponding volume data on the backend PanFS storage, based on the `reclaimPolicy` defined in your StorageClass (typically **`Delete`**).

### Remove StorageClass and Credentials

Delete the StorageClass(es) and the associated Secret containing the PanFS Realm credentials.

```bash
# Delete the StorageClass and the Secret containing the Realm credentials.
# Replace 'panfs-storage-class.yaml' with the actual file name you used.
kubectl delete -f panfs-storage-class.yaml
```

### Uninstall CSI Driver Components and KMM Module

Use the original deployment manifest to remove the Controller, Node DaemonSet, KMM Module, and all associated RBAC and custom resources.

```bash
# Delete the CSI driver Deployment, DaemonSet, CSIDriver object, and KMM Module.
# Replace 'panfs-csi-driver.yaml' with the actual file name you used.
kubectl delete -f panfs-csi-driver.yaml
```

**Remove Node Labels:**

The KMM deployment sets labels on worker nodes. These labels must be removed to signal a complete cleanup.

```bash
# Removes the panfs readiness label from all nodes.
kubectl label node -l node-role.kubernetes.io/worker= node.kubernetes.io/csi-driver.panfs.ready- --overwrite
```

**Verify Label Removal:**

Ensure no nodes still have the `node.kubernetes.io/csi-driver.panfs.ready` label set. The count should be **0**.

```bash
kubectl get nodes --show-labels | grep node.kubernetes.io/csi-driver.panfs.ready | wc -l
0
```

### Clean Up Cluster Resources

Remove the namespace and any labels or secrets created during the prerequisite steps.

1.  **Remove the CSI Driver Namespace:**

    ```bash
    kubectl delete namespace csi-panfs
    ```

2.  **Remove Registry Secret (if it was created in a separate namespace):**
    If you created the private registry secret in a different namespace, delete it now.

    ```bash
    kubectl delete secret <your-secret-name> --namespace=<your-namespace>
    ```

### Remove Dependencies (Optional)

If the cluster was solely dedicated to running the PanFS CSI driver, you may optionally uninstall the dependencies that were installed as prerequisites.

  * **Uninstall KMM (Kernel Module Manager)**: Refer to the [KMM documentation](https://kmm.sigs.k8s.io/documentation/install/).
  * **Uninstall cert-manager**: Refer to the [cert-manager documentation](https://cert-manager.io/docs/installation/).

---

## API Documentation

You can generate and view Go API documentation for this project using the [pkgsite](https://pkg.go.dev/golang.org/x/pkgsite/cmd/pkgsite) tool.

### How to Use pkgsite

1. Install pkgsite:
  ```bash
  go install golang.org/x/pkgsite/cmd/pkgsite@latest
  ```

2. Run pkgsite in the project root:
  ```bash
  pkgsite -open .
  ```

3. Your browser will open automatically; otherwise, navigate to [http://localhost:8080](http://localhost:8080) to view the generated documentation for all Go packages in the repository.

This will provide a browsable interface for all exported types, functions, and documentation comments in your codebase.

## Testing

### Kubernetes CSI Sanity tests

The PanFS CSI driver includes a ready-to-use Docker Compose setup for running CSI Sanity tests, located in `tests/csi_sanity/docker-compose.yaml`.

#### Running CSI Sanity Tests with Docker Compose

1. Build the CSI driver and sanity test images:
   - Set the environment variables `CSI_IMAGE` (your built PanFS CSI driver image) and `CSI_TEST_IMAGE` (the prebuilt test image, see `tests/csi_sanity/Dockerfile`).
   - Example:
     ```bash
     export CSI_IMAGE=your-registry/panfs-csi-driver:latest
     export CSI_TEST_IMAGE=your-registry/panfs-csi-sanity:latest
     docker compose -f tests/csi_sanity/docker-compose.yaml up --build
     ```

2. The compose file will start controller and node plugin containers, then run the sanity test suite in a dedicated container. Results will be shown in the output.

3. You can customize secrets and test parameters by editing `tests/csi_sanity/secrets.yaml` and environment variables in the compose file.

#### Notes

- The sanity test container uses the `csi-sanity` tool and runs tests against the controller and node endpoints exposed by the plugin containers.
- Healthchecks ensure the plugins are ready before tests start.
- You can inspect logs for detailed test output and troubleshooting.
- For more details on CSI Sanity, see the [CSI Sanity documentation](https://github.com/kubernetes-csi/csi-test/tree/master/cmd/csi-sanity).

### Kubernetes CSI E2E tests

End-to-end (E2E) tests validate the PanFS CSI driver in a real Kubernetes cluster, covering full volume lifecycle operations, dynamic provisioning, expansion, and workload integration.

#### Provided E2E Test Setup

- All manifests and configuration are in `tests/e2e/`:
  - `e2e-runner.yaml`: Deploys Namespace, ServiceAccount, RBAC, ConfigMap, and a CronJob to run E2E tests.
  - `test-driver.yaml`: CSI test driver configuration (capabilities, access modes, stress/performance options).
  - `test-sc.yaml`: StorageClass definition for E2E tests.

#### Running E2E Tests

1. Deploy the E2E test environment:
  ```bash
  kubectl apply -f tests/e2e/e2e-runner.yaml
  ```
  This creates the namespace, RBAC, config, and a CronJob that runs daily at 2:00 AM.

2. To run the E2E test job immediately:
  ```bash
  kubectl create job --from=cronjob/tests -n e2e tests-manual
  ```

3. You can customize the test driver and StorageClass by editing `test-driver.yaml` and `test-sc.yaml` in the `tests/e2e/` folder. The ConfigMap in `e2e-runner.yaml` references these files.

4. Review job logs for test results and troubleshooting:
  ```bash
  kubectl logs job/tests-manual -n e2e
  ```

#### Notes

- E2E tests require a working Kubernetes cluster and sufficient permissions.
- The CronJob uses a prebuilt test image and runs tests with the configuration from the ConfigMap.
- For more details, see `tests/e2e/Readme.md` in this repository.
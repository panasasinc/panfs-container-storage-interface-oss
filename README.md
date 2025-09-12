[![Driver deployment](https://github.com/panasasinc/panfs-container-storage-interface/actions/workflows/main.yaml/badge.svg)](https://github.com/panasasinc/panfs-container-storage-interface/actions/workflows/main.yaml)
[![Vulnerability Scan](https://github.com/panasasinc/panfs-container-storage-interface/actions/workflows/vulnerability.yaml/badge.svg)](https://github.com/panasasinc/panfs-container-storage-interface/actions/workflows/vulnerability.yaml)

# Container Storage Interface

## Contents

* [Overview](#overview)
* [Capabilities](#capabilities)
  * [CSI Services](#csi-services)
  * [Volume Features](#volume-features)
  * [PanFS mount options](#panfs-mount-options)
  * [Storage class customisation](#storage-class-customisation-eg-volume-create-parameters)
* [Compatibility](#compatibility)
* [Getting started](#getting-started)
  * [Prerequisites](#prerequisites)
    * [Software Requirements](#software-requirements)
    * [System Requirements](#system-requirements)
    * [Access Requirements](#access-requirements)
  * [CSI Driver Installation - Quick Start](#csi-driver-installation---quick-start)
    * [1. Ensure dependencies are installed](#1-ensure-dependencies-are-installed)
    * [2. Deploy CSI driver with KMM module](#2-deploy-csi-driver-with-kmm-module)
      * [2.1 Prerequisites](#21-prerequisites)
      * [2.2 Configure and Deploy](#22-configure-and-deploy)
    * [3. Configure and deploy StorageClass](#3-configure-and-deploy-storageclass)
* [Deploying Workloads](#deploying-workloads)
* [API Documentation](#api-documentation)
  * [How to Use pkgsite](#how-to-use-pkgsite)
* [Testing](#testing)
  * [Kubernetes CSI Sanity tests](#kubernetes-csi-sanity-tests)
    * [Running CSI Sanity Tests with Docker Compose](#running-csi-sanity-tests-with-docker-compose)
    * [Notes](#notes)
  * [Kubernetes CSI E2E tests](#kubernetes-csi-e2e-tests)
    * [Provided E2E Test Setup](#provided-e2e-test-setup)
    * [Running E2E Tests](#running-e2e-tests)
    * [Notes](#notes)

## Overview

The Container Storage Interface (CSI) is a standard for exposing arbitrary block and file storage systems to containerized workloads on Container Orchestration Systems like Kubernetes. Using CSI, third-party storage providers can write and deploy plugins exposing new storage systems in Kubernetes without ever having to touch the core Kubernetes code. The Container Storage Interface was designed to help Container Orchestrators replace their existing in-tree storage driver mechanisms - especially vendor-specific plugins.

The PanFS CSI driver allows you to use PanFS volumes in a Kubernetes cluster. It is possible to use volumes from multiple realms within a single CSI driver instance. See the [Deploying Workloads](#deploying-workloads) section for usage examples.

For more details, please see the [Overview documentation](./docs/Overview.md)


## Capabilities

### CSI Services

- **Identity Service**: Implements GetPluginInfo, GetPluginCapabilities, and Probe for plugin discovery and health checks.
- **Controller Service**: Implements CreateVolume, DeleteVolume, ControllerExpandVolume, ValidateVolumeCapabilities, and ControllerGetCapabilities for full lifecycle management.
- **Online Volume Expansion**: Supports resizing volumes while they are attached and in use.

### Volume Features

- **Volume Quota Management**: Supports soft and hard quotas for PanFS volumes.
- **Multiple Volume Layouts**: Supports various layouts (raid6+, raid5+, raid10+, etc.) for flexible provisioning.
- **Parameter Validation**: Validates StorageClass parameters, stripe units, and secrets for correctness and security.
- **Error Handling**: Maps PanFS CLI errors to CSI-compliant error codes for robust operation.
- **Unit Conversion**: Handles GB/bytes conversion for quotas and capacity.
- **Testing and Mocking**: Extensive unit tests and mock implementations for controller, identity, pancli, and validators.

### PanFS mount options
The PanFS CSI driver supports custom mount options for PanFS volumes. You can specify mount options in the PersistentVolume manifest using the `mountOptions` field. This allows you to customize the mount behavior according to your requirements.

#### Storage class customisation (e.g. volume create parameters)
The PanFS CSI driver supports custom parameters for volume creation in the StorageClass manifest. You can specify parameters such as `bladeset`, `layout` etc to customize the behavior of the created volumes.
For a full list of supported parameters, refer to the official PanFS documentation for volume creation corresponding to your PanFS version and CSI PanFS driver version.

<a name="compatibility"></a>
## Compatibility

Compatibility matrix between PanFS CSI Driver, Kubernetes, CSI Spec and PanFS versions:

| PanFS CSI Version | Kubernetes Version | CSI Spec Version |
|:------------------|:-------------------|:-----------------|
| 1.0.0             | 1.30.1+            | 1.7.0            |
| 1.0.1+            | 1.30.1+            | 1.11.0           |
| 1.1.0             | 1.30.1+            | 1.11.0           |
| 1.2.0             | 1.30.1+            | 1.11.0           |


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
- **PanFS Realm Access**: Valid credentials and network connectivity to PanFS backend

### CSI Driver Installation - Quick Start

For experienced users who want to deploy quickly, here's the essential command sequence:

#### 1. Ensure dependencies are installed

Please follow installation guides:
- https://cert-manager.io/docs/installation/
- https://kmm.sigs.k8s.io/documentation/install/

#### 2. Deploy CSI driver with KMM module

Create the namespace, configure registry credentials, and deploy the driver using the Kubernetes manifest file

##### 2.1 Prerequisites
```bash
# Create PanFS CSI Driver namespace
kubectl create namespace csi-panfs

# Configure secret for Private Registry with PanFS CSI Driver images
# Example command (replace placeholders with your actual registry settings; do not commit real credentials):
kubectl create secret docker-registry <your-secret-name> \
  --docker-server=<your-registry-server> \
  --docker-username=<your-username> \
  --docker-password=<your-password> \
  --docker-email=<your-email> \
  --namespace=csi-panfs
```

##### 2.2 Configure and Deploy

This will deploy CSI driver components and the KMM module.

Please update the settings in [deploy/k8s/csi-panfs-driver.yaml](deploy/k8s/csi-panfs-driver.yaml) according to your cluster specification and available image tags in your private registry:

- `<KERNEL_VERSION>` - Worker Node kernel version, should correspond to PANFS_KMM_IMAGE, e.g: `4.18.0-553.el8_10.x86_64`
- `<PANFS_KMM_IMAGE>` - PanFS KMM module image, e.g: `<your private registry>/panfs-dfc-kmm:4.18.0-553.el8_10.x86_64-11.1.0.a-1860775.2`
- `<PANFS_CSI_DRIVER_IMAGE>` - PanFS CSI Driver image, e.g: `<your private registry>/panfs-csi-driver:1.0.3`
- `<IMAGE_PULL_SECRET_NAME>` - Image pull secret for fetching PanFS CSI Driver images from your private registry

Review other settings relevant to your Kubernetes infrastructure, such as:
- `replicas`
- `tolerations`
- `nodeSelector`
- etc

```bash
kubectl apply -f deploy/k8s/csi-panfs-driver.yaml
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

- Check CSI driver registration:
  ```bash
  kubectl get csidrivers | grep panfs
  ```
  Expected output:
  ```
  com.vdura.csi.panfs   true   true   false   <unset>   true   Persistent   1m
  ```

- Check controller deployment:
  ```bash
  kubectl get deployment csi-panfs-controller -n csi-panfs
  ```
  Expected output:
  ```
  NAME                   READY   UP-TO-DATE   AVAILABLE   AGE
  csi-panfs-controller   3/3     3            3           1m
  ```

- Check node daemonset:
  ```bash
  kubectl get daemonset csi-panfs-node -n csi-panfs
  ```
  Expected output:
  ```
  NAME             DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR                     AGE
  csi-panfs-node   6         6         6       6            6           node-role.kubernetes.io/worker=   1m
  ```

- Check KMM module status:
  ```bash
  kubectl get module panfs -n csi-panfs
  ```
  Expected output:
  ```
  NAME    AGE
  panfs   1m
  ```
  ```bash
  kubectl get module panfs -n csi-panfs -o custom-columns=NODES:.status.moduleLoader.nodesMatchingSelectorNumber,LOADED:.status.moduleLoader.availableNumber,DESIRED:.status.moduleLoader.desiredNumber
  ```
  Expected output:
  ```
  NODES   LOADED   DESIRED
  6       6        6
  ```

#### 3. Configure and deploy StorageClass

The StorageClass defines how Kubernetes provisions PanFS-backed volumes.

Configure authentication based on your PanFS Realm setup by editing the following placeholders in [deploy/k8s/csi-panfs-storage-class.yaml](deploy/k8s/csi-panfs-storage-class.yaml):

- `<STORAGE_CLASS_NAME>` - Storage Class name, e.g., `csi-panfs-storage-class`
- `<REALM_ADDRESS>` - PanFS backend address, e.g., `panfs.example.com`
- `<REALM_USERNAME>` - PanFS Realm service username, e.g., `admin`
- `<REALM_PASSWORD>` - Password for user/password authentication with the Realm
- `<REALM_PRIVATE_KEY>` - Private key for key-based authentication; leave empty if private key access is not configured
- `<REALM_PRIVATE_KEY_PASSPHRASE>` - Passphrase for encrypted key; leave empty if there's no key encryption passphrase

To make this StorageClass the default for your Kubernetes cluster, set the annotation `storageclass.kubernetes.io/is-default-class: "true"` in this StorageClass manifest.

```bash
# Deploy PanFS Storage Class
kubectl apply -f deploy/k8s/csi-panfs-storage-class.yaml
```

The expected output will look like this:
```bash
namespace/csi-panfs-storage-class unchanged
secret/csi-panfs-storage-class configured
storageclass.storage.k8s.io/csi-panfs-storage-class configured
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

## Deploying Workloads

Refer to [usage-guide.md](./docs/usage-guide.md) for YAML manifests and examples of deploying workloads with the PanFS CSI Driver.

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

3. It opens your browser, or navigate to [http://localhost:8080](http://localhost:8080) to view the generated documentation for all Go packages in the repository.

This will provide a browsable interface for all exported types, functions, and documentation comments in your codebase.

## Testing

### Kubernetes CSI Sanity tests

The PanFS CSI driver includes a ready-to-use Docker Compose setup for running CSI Sanity tests, located in `tests/csi_sanity/docker-compose.yaml`.

#### Running CSI Sanity Tests with Docker Compose

1. Build the CSI driver and sanity test images:
   - Set the environment variables `CSI_IMAGE` (your built PanFS CSI driver image) and `CSI_TEST_IMAGE` (the baked test image, see `tests/csi_sanity/Dockerfile`).
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



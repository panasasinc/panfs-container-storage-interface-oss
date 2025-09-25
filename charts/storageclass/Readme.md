<!--
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
-->

# csi-panfs-storageclass

PanFS Realm Storage Class Chart

![Version: 1.2.2](https://img.shields.io/badge/Version-1.2.2-informational?style=flat-square) ![AppVersion: 1.2.2](https://img.shields.io/badge/AppVersion-1.2.2-informational?style=flat-square)

## What PanFS CSI StorageClass is

The PanFS CSI StorageClass Helm chart deploys a Kubernetes StorageClass resource that leverages the PanFS Container Storage Interface (CSI) driver to provision and manage persistent storage volumes on a PanFS backend. The chart enables you to configure parameters such as the PanFS realm endpoint, authentication credentials, and volume options.

> **Note:** Installation into the `default` namespace is not permitted. You must specify a non-default namespace using the `--namespace <namespace>` option during installation.

### Resources Created by the Helm Chart

- A `StorageClass` resource that uses the PanFS CSI driver to provision and manage persistent storage volumes on a PanFS backend.
- A `Secret` resource containing PanFS realm credentials (address, username, password, and private key).
- If the PanFS realm credentials secret resides in a different namespace than the PanFS CSI driver, the chart creates a `Role` and `RoleBinding` to allow the CSI driver to access the secret.

## Usage

To use this chart, ensure your Kubernetes cluster has the PanFS CSI driver installed. You can then create a StorageClass resource that uses the CSI driver.

### Examples

### 1. Creating a StorageClass with default parameters

Install the chart **in the same namespace** as the PanFS CSI driver.

```bash
helm upgrade --install <STORAGE_CLASS_NAME> ./ \
    --namespace <CSI_DRIVER_NAMESPACE> \
    --set realm.address=panfs-realm.example.com \
    --set realm.username=username \
    --set realm.password=password
```

> Note: The `--create-namespace` flag is not required when installing in an existing namespace.

#### Using Helm overrides:
```yaml
realm:
  address: "panfs-realm.example.com"
  username: "username"
  password: "password"
```

You can also use a YAML file to provide the necessary overrides. Below is an example of such a file:
```yaml
realm:
  address: "panfs-realm.example.com"
  username: "username"
  password: "password"
```

Install the chart using the overrides file:
```bash
helm upgrade --install <STORAGE_CLASS_NAME> ./ \
    --namespace <CSI_DRIVER_NAMESPACE> \
    --values path/to/your/overrides.yaml
```

### 2. Creating a StorageClass with custom parameters

Install the chart **in a different namespace** than the PanFS CSI driver.

```bash
helm upgrade --install <STORAGE_CLASS_NAME> ./ \
    --namespace <REALM_SECRET_NAMESPACE> \
    --create-namespace \
    --set realm.address=panfs-realm.example.com \
    --set realm.username=username \
    --set realm.password=password \
    --set csiPanFSDriver.namespace=<CSI_DRIVER_NAMESPACE>
```

> Note: The `--create-namespace` flag is required since installing in a new namespace.

> Note: If the PanFS realm credentials secret is in a different namespace, the chart creates the necessary permissions for the CSI driver to read the secret.

To identify the PanFS CSI Driver Namespace and its controller Service Account, run the provided command:

```bash
kubectl get csidriver com.vdura.csi.panfs -o yaml | grep csi-driver-
    csi-driver-controller-sa: csi-panfs-controller
    csi-driver-namespace: csi-panfs
```

#### Using Helm overrides:

You can also use a YAML file to provide the necessary overrides. Below is an example of such a file:
```yaml
realm:
  address: "panfs-realm.example.com"
  username: "username"
  password: "password"

csiPanFSDriver:
  namespace: csi-panfs
  controllerServiceAccount: csi-panfs-controller
```

Install the chart using the overrides file:
```bash
helm upgrade --install <STORAGE_CLASS_NAME> ./ \
    --namespace <REALM_SECRET_NAMESPACE> \
    --create-namespace \
    --values path/to/your/overrides.yaml
```

### Verifying the installation

#### 1. Checking the StorageClass

Use the provided command to list StorageClasses.

```bash
kubectl get sc -l product=com.vdura.csi.panfs
```

#### 2. Deploying a Sample Workload

A sample manifest is provided to demonstrate usage.

```yaml
# Sample Pod that uses the PVC to mount the PanFS volume
apiVersion: v1
kind: Pod
metadata:
  name: single-pod
spec:
  containers:
    - name: main-container
      image: ubuntu:latest
      command: [ "sleep", "infinity" ]
      volumeMounts:
        - mountPath: "/data"
          name: panfs-volume
      resources:
        limits:
          memory: 1G
          cpu: "100m"
  volumes:
    - name: panfs-volume
      persistentVolumeClaim:
        claimName: csi-pvc-single-pod
---
# Sample PVC that uses the StorageClass created by this Helm chart
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-pvc-single-pod
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
  storageClassName: <STORAGE_CLASS_NAME>
```

Check the status of pods, PVCs, and PVs using the provided command.

```bash
kubectl get po,pvc,pv
```

If you encounter errors:

- Verify the realm credentials in the StorageClass secret.
- Check the logs of the CSI driver:
  - For controller issues (volume provisioning), review the controller logs:
    ```
    kubectl logs -n <CSI_DRIVER_NAMESPACE> -l app=csi-panfs-controller --all-pods -c csi-panfs-plugin
    kubectl logs -n csi-panfs -l app=csi-panfs-controller --all-pods -c csi-panfs-plugin
    ```
  - For node issues (volume mounting), review the node logs:
    ```
    kubectl logs -n <CSI_DRIVER_NAMESPACE> -l app=csi-panfs-node --all-pods -c csi-panfs-plugin
    kubectl logs -n csi-panfs -l app=csi-panfs-node --all-pods -c csi-panfs-plugin
    ```

### Uninstalling the chart

Use the provided command to uninstall the chart from the specified namespace.

```bash
helm uninstall <STORAGE_CLASS_NAME> \
    --namespace=<NAMESPACE>
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| allowVolumeExpansion | bool | `true` | Allow volume expansion for realms |
| csiPanFSDriver | object | `{...}` | PanFS CSI Driver details |
| csiPanFSDriver.controllerServiceAccount | string | `"csi-panfs-controller"` | Service account used by the PanFS CSI driver controller |
| csiPanFSDriver.namespace | string | `"csi-panfs"` | Namespace where the PanFS CSI driver is deployed |
| mountOptions | list | `[]` |  |
| parameters | object | `{...}` | Optional storage class parameters |
| realm.address | string | `""` | Endpoint address for the backend PanFS realm |
| realm.password | string | `""` | Password for the PanFS backend realm |
| realm.privateKey | string | `""` | Private key for the PanFS backend realm |
| realm.privateKeyPassphrase | string | `""` |  |
| realm.username | string | `""` | Username for the PanFS backend realm |
| setAsDefaultStorageClass | bool | `false` | Whether to set current storage class default for the cluster or not |
| volumeBindingMode | string | `"WaitForFirstConsumer"` | Default volume binding mode |
| volumeReclaimPolicy | string | `Delete` | Default reclaim policy for volumes |

### PanFS Specific Volume Parameters (see PanCLI User Guide for the details)

Refer to the PanCLI User Guide for details on the following parameters:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| parameters."panfs.csi.vdura.com/bladeset" | string | `""` | Name of the bladeset to use for realm volumes |
| parameters."panfs.csi.vdura.com/recoverypriority" | string | `""` | Recovery priority for the realm volumes |
| parameters."panfs.csi.vdura.com/efsa" | string | `""` | EFSA volume mode |
| parameters."panfs.csi.vdura.com/layout" | string | `"raid10+"` | Default layout for the realm volumes |
| parameters."panfs.csi.vdura.com/maxwidth" | int |  | Maximum number of storages to stripe over |
| parameters."panfs.csi.vdura.com/stripeunit" | string | `""` | Stripe unit for the realm volumes |
| parameters."panfs.csi.vdura.com/rgwidth" | int |  | Number of storage nodes to stripe over in a single RAID group |
| parameters."panfs.csi.vdura.com/rgdepth" | int |  | Number of stripes written to a RAID parity group before advancing to the next parity group |
| parameters."panfs.csi.vdura.com/volservice" | string | `""` | Volume service id for the realm volumes |
| parameters."panfs.csi.vdura.com/description" | string | `""` | Description for the realm volumes |
| parameters."panfs.csi.vdura.com/user" | string | `""` | User name or ID |
| parameters."panfs.csi.vdura.com/group" | string | `""` | Group name or ID |
| parameters."panfs.csi.vdura.com/uperm" | string | `""` | User permissions |
| parameters."panfs.csi.vdura.com/gperm" | string | `""` | Group permissions |
| parameters."panfs.csi.vdura.com/operm" | string | `""` | Other permissions |


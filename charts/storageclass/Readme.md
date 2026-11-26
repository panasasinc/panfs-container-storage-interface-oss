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

![Version: 2.0.0](https://img.shields.io/badge/Version-2.0.0-informational?style=flat-square) ![AppVersion: 2.0.0](https://img.shields.io/badge/AppVersion-2.0.0-informational?style=flat-square)

## What PanFS CSI StorageClass is

The PanFS CSI StorageClass Helm chart deploys a Kubernetes StorageClass resource that leverages the PanFS Container Storage Interface (CSI) driver to provision and manage persistent storage volumes on a PanFS backend. The chart enables you to configure parameters such as the PanFS realm API address, the service account token, and volume options.

> **Note:** Installation into the `default` namespace is not permitted. You must specify a non-default namespace using the `--namespace <namespace>` option during installation.

### Resources Created by the Helm Chart

- A `StorageClass` resource that uses the PanFS CSI driver to provision and manage persistent storage volumes on a PanFS backend.
- A `Secret` resource containing the PanFS realm API address and the service account token.
- If the PanFS realm credentials secret resides in a different namespace than the PanFS CSI driver, the chart creates a `Role` and `RoleBinding` to allow the CSI driver to access the secret.

## Requirements on the PanFS realm

- `realm.api` is the PanAPI address of the realm, and `realm.token` is the token of a PanAPI service account.
- A storageset must be assigned to the tenant of the token.
- The token needs these permissions for its own tenant: read the tenant and its storageset assignments;
  list, read, create, update and delete volumes; list the subnets and read the mount recipe of a subnet.
- The worker nodes must be attached to the network of the tenant. The driver does not attach them;
  the [PanFS Tenant Network operator](../tenant-network/Readme.md) does. Its documentation shows the `allowedTopologies`
  value that keeps new volumes on the nodes where the tenant network works.
- If the realm certificate is signed by a private CA, set `realm.caCert` to the PEM certificate of that CA,
  for example with `--set-file realm.caCert=ca.pem`. For a self-signed certificate, use that certificate itself.
- For a test realm whose certificate cannot be verified, set `realm.insecureSkipTLSVerify=true`.
  The driver then does not verify the realm certificate, so a host that poses as the realm can read the token.

## Usage

To use this chart, ensure your Kubernetes cluster has the PanFS CSI driver installed. You can then create a StorageClass resource that uses the CSI driver.

### Examples

### 1. Creating a StorageClass with default parameters

Install the chart **in the same namespace** as the PanFS CSI driver.

```bash
# Getting the namespace where the PanFS CSI Driver is installed
PANFS_CSI_DRIVER_NAMESPACE="$(kubectl get csidrivers.storage.k8s.io com.vdura.csi.panfs -o jsonpath='{.metadata.annotations.csi-driver-namespace}')"

# Installing chart into PanFS CSI Driver namespace
helm upgrade --install <STORAGE_CLASS_NAME> ./ \
    --namespace "${PANFS_CSI_DRIVER_NAMESPACE}" \
    --set realm.api=panfs-realm.example.com \
    --set realm.token="<service account token>" \
    --set-string 'parameters.panfs\.csi\.vdura\.com/storageset=1'
```

> Note: The `--create-namespace` flag is not required when installing in an existing namespace.

#### Using Helm overrides:

You can also use a YAML file to provide the necessary overrides. Below is an example of such a file:
```yaml
# overrides.yaml
realm:
  api: "panfs-realm.example.com"
  token: "<service account token>"

parameters:
  panfs.csi.vdura.com/storageset: "1"
```

Install the chart using the overrides file:
```bash
helm upgrade --install <STORAGE_CLASS_NAME> ./ \
    --namespace "${PANFS_CSI_DRIVER_NAMESPACE}" \
    --values path/to/your/overrides.yaml
```

### 2. Creating a StorageClass with custom parameters

Install the chart **in a different namespace** than the PanFS CSI driver.

```bash
# Getting the namespace where the PanFS CSI Driver is installed
PANFS_CSI_DRIVER_NAMESPACE="$(kubectl get csidrivers.storage.k8s.io com.vdura.csi.panfs -o jsonpath='{.metadata.annotations.csi-driver-namespace}')"

helm upgrade --install <STORAGE_CLASS_NAME> ./ \
    --namespace <REALM_SECRET_NAMESPACE> \
    --create-namespace \
    --set setAsDefaultStorageClass="true" \
    --set realm.api="panfs-realm.example.com" \
    --set realm.token="<service account token>" \
    --set-string 'parameters.panfs\.csi\.vdura\.com/storageset=1' \
    --set csiPanFSDriver.namespace="${PANFS_CSI_DRIVER_NAMESPACE}"
```

> Note:
> - The `--create-namespace` flag is required when installing into a new namespace.
> - The option `setAsDefaultStorageClass=true` is used to set this storage class as the default in your Kubernetes cluster
> - A new namespace is used for keeping the Helm release state, the PanFS realm credentials secret, and the Role/RoleBinding that lets the CSI Driver read it from its namespace
> - You can choose any suitable name for `<REALM_SECRET_NAMESPACE>`, but we advise setting it equal to `<STORAGE_CLASS_NAME>`

#### Using Helm overrides:

You can also use a YAML file to provide the necessary overrides. Below is an example of such a file:
```yaml
# overrides.yaml
realm:
  api: "panfs-realm.example.com"
  token: "<service account token>"

parameters:
  panfs.csi.vdura.com/storageset: "1"

setAsDefaultStorageClass: "true"

csiPanFSDriver:
  namespace: csi-panfs
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
      command:
        - sh
        - -c
        - |
          mount | grep /data
          sleep infinity
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

### Troubleshooting Pod Creation and Runtime Errors

If you encounter errors when creating or running workload Pods, follow these steps to diagnose and resolve common issues:

- Verify the realm credentials in the StorageClass secret.
  ```bash
  # Getting the secret name and its namespace from the StorageClass
  kubectl get sc <STORAGE_CLASS_NAME> -o yaml | grep csi.storage.k8s.io

  # Checking the secret content
  # If you installed the chart in a dedicated namespace, the secret name, its namespace, and the StorageClass name will match the chart release name.
  # If you installed the chart in the same namespace as the CSI driver, the secret name and StorageClass name will match the chart release name, and the secret will reside in the CSI driver's namespace.
  kubectl get secret -n <REALM_SECRET_NAMESPACE> <REALM_SECRET_NAME> -o jsonpath='{.data.api}' | base64 -d
  ```
- Check the logs of the CSI driver:
  - For controller issues (volume provisioning), review the controller logs:
    ```bash
    CSI_DRIVER_NAMESPACE=$(kubectl get csidriver com.vdura.csi.panfs -o yaml | grep csi-driver-namespace | awk '{print $2}')
    kubectl logs -n $CSI_DRIVER_NAMESPACE -l app=csi-panfs-controller --all-pods -c csi-panfs-plugin
    ```
  - For node issues (volume mounting), review the node logs:
    ```bash
    CSI_DRIVER_NAMESPACE=$(kubectl get csidriver com.vdura.csi.panfs -o yaml | grep csi-driver-namespace | awk '{print $2}')
    kubectl logs -n $CSI_DRIVER_NAMESPACE -l app=csi-panfs-node --all-pods -c csi-panfs-plugin
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
| allowVolumeExpansion | bool | `true` | Allow expansion of provisioned volumes |
| allowedTopologies | object | `{}` | Allowed topologies for volume provisioning This can be used to restrict volume provisioning to specific nodes or zones based on node labels. Uncomment and modify the example below as needed. If left empty, no topology constraints will be applied. |
| csiPanFSDriver.namespace | string | `"csi-panfs"` | Namespace where the PanFS CSI driver is deployed |
| mountOptions | list | `[]` |  |
| parameters | object | `{...}` | Storage class parameters. All of them are optional; a parameter with an empty value is left out. |
| realm.api | string | `""` | PanAPI address of the PanFS realm: a host name or IP address (HTTPS is used), or a full `https://` or `http://` URL. The nodes mount volumes from the host of this address, unless PanAPI names another mount address for the tenant. |
| realm.caCert | string | `""` | Optional PEM certificate of the CA that signed the realm certificate. Leave empty to use the system trust store. |
| realm.insecureSkipTLSVerify | bool | `false` | Do not verify the realm certificate. Use it only for a test realm whose certificate cannot be verified: a host that poses as the realm can then read the token. Prefer `realm.caCert`. |
| realm.token | string | `""` | PanAPI service account token. Its tenant must have a storageset assigned. |
| setAsDefaultStorageClass | bool | `false` | Whether to set this storage class as the default for the cluster |
| volumeBindingMode | string | `"WaitForFirstConsumer"` | Default volume binding mode |
| volumeReclaimPolicy | string | `Delete` | Default reclaim policy for volumes |

### PanFS Specific Volume Parameters

The driver passes the following parameters to the realm when it creates a volume:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| parameters."panfs.csi.vdura.com/storageset" | string | `""` | Storageset that holds the volumes: its id (a number) or its name. Optional: without it, the first storageset assigned to the tenant is used |
| parameters."panfs.csi.vdura.com/recoverypriority" | string | `"50"` | Recovery priority for the realm volumes |
| parameters."panfs.csi.vdura.com/efsa" | string | `"retry"` | EFSA volume mode |
| parameters."panfs.csi.vdura.com/layout" | string | `"raid6+"` | Default layout for the realm volumes |
| parameters."panfs.csi.vdura.com/maxwidth" | int | 3 | Maximum number of storages to stripe over |
| parameters."panfs.csi.vdura.com/stripeunit" | string | `"64k"` | Stripe unit for the realm volumes |
| parameters."panfs.csi.vdura.com/rgwidth" | int | 3 | Number of storage nodes to stripe over in a single RAID group |
| parameters."panfs.csi.vdura.com/rgdepth" | int | 2 | Number of stripes written to a RAID parity group before advancing to the next parity group |
| parameters."panfs.csi.vdura.com/volservice" | string |  | Volume service id for the realm volumes |
| parameters."panfs.csi.vdura.com/user" | string |  | Numeric id of the owner user |
| parameters."panfs.csi.vdura.com/group" | string |  | Numeric id of the owner group |
| parameters."panfs.csi.vdura.com/uperm" | string |  | User permissions |
| parameters."panfs.csi.vdura.com/gperm" | string |  | Group permissions |
| parameters."panfs.csi.vdura.com/operm" | string | `"all"` | Other permissions |

The driver rejects the parameters `description`, `encryption` and `recovery` of earlier driver versions.
Use `recoverypriority` in place of `recovery`. Volume encryption is not supported.


# csi-panfs-storageclass

PanFS Realm Storage Class Chart

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![AppVersion: 1.0](https://img.shields.io/badge/AppVersion-1.0-informational?style=flat-square)

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| allowVolumeExpansion | bool | `true` | Allow volume expansion for realms |
| csiPanFSDriver | object | `{"controllerServiceAccount":"csi-panfs-controller","namespace":"csi-panfs","productName":"com.vdura.csi.panfs"}` | PanFS CSI Driver details |
| csiPanFSDriver.controllerServiceAccount | string | `"csi-panfs-controller"` | Service account used by the PanFS CSI driver controller |
| csiPanFSDriver.namespace | string | `"csi-panfs"` | Namespace where the PanFS CSI driver is deployed |
| csiPanFSDriver.productName | string | `"com.vdura.csi.panfs"` | Product name for the PanFS CSI driver |
| mountOptions | list | `[]` |  |
| parameters | object | `{...}` | Optional storage class parameters |
| realm.address | string | `""` | Endpoint address for the backend PanFS realm |
| realm.password | string | `""` | Password for the PanFS backend realm |
| realm.privateKey | string | `""` | Private key for the PanFS backend realm |
| realm.privateKeyPassphrase | string | `""` |  |
| realm.username | string | `""` | Username for the PanFS backend realm |
| setAsDefaultStorageClass | bool | `false` | Whether to set current storage class default for the cluster or not |
| volumeBindingMode | string | `"WaitForFirstConsumer"` | Default volume binding mode |
| volumeReclaimPolicy | string | `"Delete"` | Default reclaim policy for volumes |

### PanFS Specific Volume Parameters (see PanCLI User Guide for the details)

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

## Usage
To use this chart, you need to have a Kubernetes cluster with the CSI driver for PanFS installed. You can then create a StorageClass resource that uses the CSI driver.

### 1. Checking chart configuration

```bash
helm lint charts/storageclass \
    --set realm.address=realm-address \
    --set realm.username=admin \
    --set realm.password=admin-password
```

### 2. Installing the chart

```bash
helm upgrade --install csi-panfs-storage-class-1 charts/storageclass \
    --namespace=csi-panfs-realm-1 --create-namespace \
    --set realm.address=realm-address \
    --set realm.username=admin \
    --set realm.password=admin-password
```

### 4. Verifying the installation
```bash
kubectl get storageclass csi-panfs-storage-class-1 \
    --namespace=csi-panfs-realm-1
```
### 5. Creating a Persistent Volume Claim (PVC)
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-panfs-storage-class-1
```

### 6. Uninstalling the chart

```bash
helm uninstall csi-panfs-storage-class-1 \
    --namespace=csi-panfs-realm-1
```

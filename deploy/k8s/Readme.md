
# PanFS CSI Driver & StorageClass Deployment Guide

This guide describes how to deploy the PanFS CSI Driver and StorageClass using the provided Kubernetes manifests. 

## 1. Choosing the Correct CSI Driver Manifest

- **Default installation (with KMM and SELinux in Enforcing/Permissive mode):**
  - Use: `csi-driver/default.yaml`
- **If you do NOT use Kernel Module Manager (KMM) in your cluster:**
  - Use: `csi-driver/without-kmm.yaml`
- **If SELinux is disabled in your cluster:**
  - Use: `csi-driver/without-selinux.yaml`

**Important:**
Before applying the driver manifest, update the following parameters to match your infrastructure:
- `<IMAGE_PULL_SECRET_NAME>`: The name of your image pull secret for accessing container images.
- `<PANFS_DFC_IMAGE>`: The full image reference for the PanFS DFC container.
- `<KERNEL_VERSION>`: The kernel version required for your environment.

Apply the chosen manifest:
```bash
kubectl apply -f <selected-driver-manifest>.yaml
```


## 2. Deploying the StorageClass and Secret

Choose the manifest that matches your namespace and topology requirements:

### 1. Dedicated Namespace
- **Manifests:** 
  - `storage-class/default.yaml`
  - `storage-class/with-secret-in-dedicated-ns.yaml` (more placeholders to change)
- **Configuration:**
  - Creates a StorageClass
    - Change `<STORAGE_CLASS_NAME>` to your custom name (e.g., `csi-panfs-storage-class-name`)
    - Set the same name for storage class, secret name and its namespace
  - Creates a Secret with PanFS Realm credentials in the provided namespace
    - Replace these placeholders with your actual credentials:
      - `<REALM_ADDRESS>`
      - `<REALM_USERNAME>`
      - `<REALM_PASSWORD>`
      - (add any other required fields)
  - To set this StorageClass as default, set `storageclass.kubernetes.io/is-default-class` to `"true"`
  - Configures Role and RoleBinding to allow the CSI Driver ServiceAccount (in the `csi-panfs` namespace) to read the Secret
    - Update namespace or permissions if needed (`<CSI_NAMESPACE>`)

### 2. CSI Driver Namespace
- **Manifest:** `storage-class/with-secret-in-driver-ns.yaml`
- **Configuration:**
  - Change `<STORAGE_CLASS_NAME>` to your custom name (e.g., `csi-panfs-storage-class-name`)
  - Creates a Secret (same name as StorageClass) with PanFS Realm credentials in the `csi-panfs` namespace
    - Replace these placeholders with your actual credentials:
      - `<REALM_ADDRESS>`
      - `<REALM_USERNAME>`
      - `<REALM_PASSWORD>`
      - (add any other required fields)
  - To set this StorageClass as default, set `storageclass.kubernetes.io/is-default-class` to `"true"`
  - Update namespace or permissions if your CSI Driver namespace differs

Apply the selected manifest:
```bash
kubectl apply -f <selected-storageclass-manifest>.yaml
```

## 3. Example: Creating a PersistentVolumeClaim (PVC)

After deploying the StorageClass, you can create PVCs referencing your StorageClass. Example:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-panfs-sample
spec:
  storageClassName: csi-panfs-storage-class-name
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

Apply with:
```bash
kubectl apply -f <your-pvc-manifest>.yaml
```

## 4. Additional Notes

- Review all comments in the manifests for guidance on configuration and permissions.
- Ensure that any referenced ServiceAccounts, Roles, and RoleBindings are created and configured as described.
- For troubleshooting, check the logs of the CSI driver pods and verify that secrets and StorageClass parameters are correct.
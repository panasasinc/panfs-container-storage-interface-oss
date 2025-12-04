
# PanFS CSI Driver & StorageClass Deployment Guide

This guide describes how to deploy the PanFS CSI Driver and StorageClass using the provided Kubernetes manifests. 

## 1. Choosing the Correct CSI Driver Manifest

- **Default installation (with KMM and SELinux in Enforcing/Permissive mode):**
  - Use: `csi-driver/template-csi-panfs.yaml`
- **If you do NOT use Kernel Module Manager (KMM) in your cluster:**
  - Use: `csi-driver/template-csi-panfs-without-kmm.yaml`
- **If SELinux is disabled in your cluster:**
  - Use: `csi-driver/template-csi-panfs-without-selinux.yaml`

**Key Place Holders:**

  * `<IMAGE_PULL_SECRET_NAME>`: The name of the secret created above.
  * `<PANFS_DFC_KMM_PRIVATE_REGISTRY>`: The URL of your private registry hosting the DFC/KMM images.
  * `<DFC_RELEASE_VERSION>`: The specific version tag of the DFC release you are deploying.

> **Note:** Review other settings relevant to your Kubernetes infrastructure, such as:
> - `replicas`
> - `tolerations`
> - `nodeSelector`
> - etc

Once configured, deploy the driver and KMM module:

```bash
kubectl apply -f <selected-driver-manifest>.yaml
```

## 2. Deploying the StorageClass and Secret

Choose the manifest that matches your namespace and topology requirements:

### 1. Dedicated Namespace
- **Manifests:** 
  - `storage-class/template-secret-in-driver-ns.yaml`
  - `storage-class/template-secret-in-dedicated-ns.yaml` (more placeholders to change)
- **Configuration:**
  - Creates a StorageClass
    - Change `<STORAGE_CLASS_NAME>` to your custom name (e.g., `csi-panfs-storage-class`)
    - Set the same name for storage class, secret name and its namespace
  - Creates a Secret with PanFS Realm credentials in the provided namespace
    - Replace these placeholders with your actual credentials:
      - `<REALM_ADDRESS>`
      - `<REALM_USERNAME>`
      - `<REALM_PASSWORD>`
      - (add any other required fields)
  - To set this StorageClass as default, set `storageclass.kubernetes.io/is-default-class` to `"true"`
  - Configures Role and RoleBinding to allow the CSI Driver ServiceAccount to read the Secret
    - Update namespace or permissions if needed (`<CSI_NAMESPACE>`)

### 2. CSI Driver Namespace
- **Manifest:** `storage-class/template-secret-in-driver-ns.yaml`
- **Configuration:**
  - Change `<STORAGE_CLASS_NAME>` to your custom name (e.g., `csi-panfs-storage-class`)
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

## 3. Optional: Enabling End-to-End Volume Encryption

To enable transparent, end-to-end volume encryption using a KMIP provider:

1.  **Enable Encryption in StorageClass**: Set the parameter below to `"true"` in the StorageClass manifest:
    ```yaml
    kind: StorageClass
    parameters:
      panfs.csi.vdura.com/encryption: "true" # Enables encryption for volumes
    ```
2.  **Configure KMIP Client**: The KMIP client configuration file content must be placed in the Secret under the `kmip_config_data` key as a YAML multi-line string:
    ```yaml
    kind: Secret
    stringData:
      kmip_config_data: |-
        # Insert the full KMIP client configuration file content here.
        # This typically includes server addresses, port, and client TLS/PKI settings.
    ```
3. Make sure KMM module is configured to load `wolfssl` kermel module. Check this:
    ```bash
    kubectl get module panfs -n csi-panfs -o jsonpath='{.spec.moduleLoader.container.modprobe.modulesLoadingOrder}'
    ["panfs","wolfssl"]
    ```

## 4. Example: Creating a PersistentVolumeClaim (PVC)

After deploying the StorageClass, you can create PVCs referencing your StorageClass. Example:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-panfs-sample
spec:
  storageClassName: csi-panfs-storage-class
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

## 5. Additional Notes

- Review all comments in the manifests for guidance on configuration and permissions.
- Ensure that any referenced ServiceAccounts, Roles, and RoleBindings are created and configured as described.
- For troubleshooting, check the logs of the CSI driver pods and verify that secrets and StorageClass parameters are correct.
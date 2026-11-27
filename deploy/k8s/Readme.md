
# PanFS CSI Driver & StorageClass Deployment Guide

This guide describes how to deploy the PanFS CSI Driver and StorageClass using the provided Kubernetes manifests. 

## 1. Choosing the Correct CSI Driver Manifest

- **Default installation (with KMM and SELinux in Enforcing/Permissive mode):**
  - Use: `csi-driver/template-csi-panfs.yaml`
- **If you do NOT use Kernel Module Manager (KMM) in your cluster:**
  - Use: `csi-driver/template-csi-panfs-without-kmm.yaml`
- **If SELinux is disabled in your cluster:**
  - Use: `csi-driver/template-csi-panfs-without-selinux.yaml`

**Key Placeholders:**

  * `<IMAGE_PULL_SECRET_NAME>`: The name of the image pull secret for the private registry hosting the DFC/KMM images.
  * `<PANFS_DFC_KMM_PRIVATE_REGISTRY>`: The URL of your private registry hosting the DFC/KMM images.
  * `<DFC_RELEASE_VERSION>`: The specific version tag of the DFC release you are deploying.

> **Note:** Review other settings relevant to your Kubernetes infrastructure, such as:
> - `replicas`
> - `tolerations`
> - `nodeSelector`
> - etc.

Once configured, deploy the driver and KMM module:

```bash
kubectl apply -f <selected-driver-manifest>.yaml
```

## 2. Deploying the StorageClass and Secret

Choose the manifest that matches your namespace and topology requirements:

### 1. Dedicated Namespace
- **Manifest:** `storage-class/template-secret-in-dedicated-ns.yaml` (more placeholders to change)
- **Configuration:**
  - Creates a StorageClass
    - Change `<STORAGE_CLASS_NAME>` to your custom name (e.g., `csi-panfs-storage-class`)
    - Set the same name for storage class, secret name and its namespace
  - Creates a Secret with PanFS Realm credentials in the provided namespace
    - Replace these placeholders with your actual credentials:
      - `<REALM_API>`
      - `<REALM_TOKEN>`
    - Replace `<STORAGESET>` with the id or the name of the storageset, or remove the line
      to use the first storageset that is assigned to the tenant
  - To set this StorageClass as default, set `storageclass.kubernetes.io/is-default-class` to `"true"`
  - Configures Role and RoleBinding to allow the CSI Driver ServiceAccount to read the Secret
    - Update namespace or permissions if needed (`<CSI_NAMESPACE>`)

### 2. CSI Driver Namespace
- **Manifest:** `storage-class/template-secret-in-driver-ns.yaml`
- **Configuration:**
  - Change `<STORAGE_CLASS_NAME>` to your custom name (e.g., `csi-panfs-storage-class`)
  - Creates a Secret (same name as StorageClass) with PanFS Realm credentials in the `csi-panfs` namespace
    - Replace these placeholders with your actual credentials:
      - `<REALM_API>`
      - `<REALM_TOKEN>`
    - Replace `<STORAGESET>` with the id or the name of the storageset, or remove the line
      to use the first storageset that is assigned to the tenant
  - To set this StorageClass as default, set `storageclass.kubernetes.io/is-default-class` to `"true"`
  - Update namespace or permissions if your CSI Driver namespace differs

Apply the selected manifest:
```bash
kubectl apply -f <selected-storageclass-manifest>.yaml
```

`<REALM_API>` is the PanAPI address of the realm and `<REALM_TOKEN>` is a PanAPI service account token.
For the permissions of the token and the network requirements, see
[PanFS Realm Requirements](../../README.md#panfs-realm-requirements).
If the realm certificate is signed by a private CA, add the key `ca_cert` with the PEM certificate of that CA to the Secret.
If the certificate cannot be verified at all, for example a self-signed certificate that does not name the realm address,
add the key `insecure_skip_tls_verify` with the value `"true"`. The driver then does not verify the realm certificate,
so a host that poses as the realm can read the token. Use it for test realms only.

## 3. Example: Creating a PersistentVolumeClaim (PVC)

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

## 4. Additional Notes

- Review all comments in the manifests for guidance on configuration and permissions.
- Ensure that any referenced ServiceAccounts, Roles, and RoleBindings are created and configured as described.
- For troubleshooting, check the logs of the CSI driver pods and verify that secrets and StorageClass parameters are correct.

## 3. Deploying the Tenant Network (optional)

A volume of a tenant is reachable only from the network of that tenant.
Deploy the tenant network when the worker nodes are not on that network yet.

- **Operator manifest:** `tenant-network/panfs-tenant-network-operator.yaml`, once per cluster.
  It holds the `TenantNetwork` resource definition and the operator, for the namespace `panfs-tenant-network`.
- **Tenant manifest:** `tenant-network/template-tenantnetwork.yaml`, once per tenant.
  - Replace `<NAME>` with a name for the resource
  - Replace `<TENANT_NAMESPACE>` with the namespace of the Secret that holds the PanAPI address and the token of the tenant
  - Replace `<SECRET_NAME>` with the name of that Secret (keys `api` and `token`)
  - Replace `<LABEL_KEY>` and `<LABEL_VALUE>` with a label of the nodes that mount volumes of the tenant.
    Any label works. Without `nodeSelector`, all nodes of the cluster are put on the tenant network

```bash
# Once per cluster: the operator
kubectl create namespace panfs-tenant-network
kubectl apply -f tenant-network/panfs-tenant-network-operator.yaml

# Per tenant: label the nodes with a label of your choice, then ask for the tenant network
kubectl label node <NODE_NAME> <LABEL_KEY>=<LABEL_VALUE>
kubectl apply -f tenant-network/template-tenantnetwork.yaml

# The TenantNetwork must be Ready; its status lists the nodes, and its events say what happened
kubectl get tenantnetworks -A
kubectl -n <TENANT_NAMESPACE> describe tenantnetwork <NAME>
```

A ready node has the label `panfs.vdura.com/tenant-network=<TENANT_NAMESPACE>`.
To provision volumes only for the ready nodes, add this block to the StorageClass manifest of the tenant:

```yaml
allowedTopologies:
  - matchLabelExpressions:
      - key: panfs.vdura.com/tenant-network
        values:
          - "<TENANT_NAMESPACE>"
```

To take a node off the tenant network, remove its label. The pod of the node then removes the tenant interface
and detaches the node from the subnet in the realm:

```bash
kubectl label node <NODE_NAME> panfs.vdura.com/tenant-
```

To remove the network of a tenant, delete its `TenantNetwork` while the operator runs.
The delete waits until every pod has cleaned up its node:

```bash
kubectl delete -f tenant-network/template-tenantnetwork.yaml
```

The requirements and the details are in the [operator documentation](../../charts/tenant-network-operator/Readme.md).

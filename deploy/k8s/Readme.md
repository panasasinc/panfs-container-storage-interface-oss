
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

## 2. Deploying the PanFS Tenant Operator

The PanFS Tenant operator creates the StorageClasses of the driver, and it puts the nodes on the network of a tenant.
Deploy it once per cluster, after the driver.

- **Operator manifest:** `tenant-operator/panfs-tenant-operator.yaml`.
  It holds the resource definitions of `TenantNetwork` and `PanfsStorageClass`, and the operator,
  for `csi-panfs`, the namespace of the driver.

```bash
kubectl apply --server-side -f tenant-operator/panfs-tenant-operator.yaml
kubectl -n csi-panfs rollout status deployment/panfs-tenant-operator
```

## 3. Deploying the Tenant Network (optional)

A volume of a tenant is reachable only from the network of that tenant.
Deploy the tenant network when the worker nodes are not on that network yet.
Skip this section when the nodes reach the realm without a tenant network.

Create a namespace for the tenant, and in it a Secret with the PanAPI address and the token of the tenant:

```bash
kubectl create namespace <TENANT_NAMESPACE>
kubectl label namespace <TENANT_NAMESPACE> pod-security.kubernetes.io/enforce=privileged
kubectl -n <TENANT_NAMESPACE> create secret generic <SECRET_NAME> \
    --from-literal=api=<REALM_API> \
    --from-literal=token=<REALM_TOKEN>
```

`<REALM_API>` is the PanAPI address of the realm and `<REALM_TOKEN>` is a PanAPI service account token.
For the permissions of the token and the network requirements, see
[PanFS Realm Requirements](../../README.md#panfs-realm-requirements).
If the realm certificate is signed by a private CA, add the key `ca_cert` with the PEM certificate of that CA to the Secret.
If the certificate cannot be verified at all, for example a self-signed certificate that does not name the realm address,
add the key `insecure_skip_tls_verify` with the value `"true"`. The driver then does not verify the realm certificate,
so a host that poses as the realm can read the token. Use it for test realms only.

- **Tenant manifest:** `tenant-operator/template-tenantnetwork.yaml`, once per tenant.
  - Replace `<NAME>` with a name for the resource
  - Replace `<TENANT_NAMESPACE>` with the namespace of the Secret
  - Replace `<SECRET_NAME>` with the name of that Secret (keys `api` and `token`)
  - Replace `<LABEL_KEY>` and `<LABEL_VALUE>` with a label of the nodes that mount volumes of the tenant.
    Any label works. Without `nodeSelector`, all nodes of the cluster are put on the tenant network

```bash
# Label the nodes with a label of your choice, then ask for the tenant network
kubectl label node <NODE_NAME> <LABEL_KEY>=<LABEL_VALUE>
kubectl apply -f tenant-operator/template-tenantnetwork.yaml

# The TenantNetwork must be Ready; its status lists the nodes, and its events say what happened
kubectl get tenantnetworks -A
kubectl -n <TENANT_NAMESPACE> describe tenantnetwork <NAME>
```

A ready node has the label `panfs.vdura.com/tenant-network=<TENANT_NAMESPACE>`.

To take a node off the tenant network, remove the label that `nodeSelector` names. The pod of the node then removes
the tenant interface and detaches the node from the subnet in the realm:

```bash
kubectl label node <NODE_NAME> <LABEL_KEY>-
```

To remove the network of a tenant, delete its `TenantNetwork` while the operator runs.
The delete waits until every pod has cleaned up its node:

```bash
kubectl delete -f tenant-operator/template-tenantnetwork.yaml
```

## 4. Deploying the StorageClass

A StorageClass is a `PanfsStorageClass` resource. The operator creates the StorageClass from it: it fills in
the provisioner and the references to the realm Secret, and it lets the CSI controller read that Secret.

- **StorageClass manifest:** `storage-class/template-panfsstorageclass.yaml`, once per StorageClass.
  - Replace `<STORAGE_CLASS_NAME>` with the name of the StorageClass
  - Replace `<NAMESPACE>` with the namespace of the `TenantNetwork`; the operator finds the `TenantNetwork` there,
    uses its Secret, and binds every volume to the ready nodes
  - Only for a realm without a tenant network: `<NAMESPACE>` is a namespace with a Secret like the one of section 3
    and without a `TenantNetwork`. Remove the `#` before `realmSecretName` and replace `<SECRET_NAME>`
  - To choose the storageset, remove the `#` before `panfs.csi.vdura.com/storageset` and replace `<STORAGESET>`
    with its id or its name. Without the line, the first storageset that is assigned to the tenant is used
  - To set this StorageClass as default, set `default` to `true`
  - Remove the optional fields that you do not need

```bash
kubectl apply -f storage-class/template-panfsstorageclass.yaml

# The phase must become Ready
kubectl get panfsstorageclasses -A
kubectl get storageclass <STORAGE_CLASS_NAME>
```

A StorageClass that you write yourself with `allowedTopologies` on the ready label must have
`volumeBindingMode: Immediate`. With `WaitForFirstConsumer` no volume is provisioned.

## 5. Example: Creating a PersistentVolumeClaim (PVC)

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

## 6. Additional Notes

- Review all comments in the manifests for guidance on configuration and permissions.
- For troubleshooting, check the logs of the CSI driver pods and of the operator, and the status and the events
  of the `TenantNetwork` and the `PanfsStorageClass`.
- The requirements and the details are in the
  [documentation of the PanFS Tenant operator](../../charts/panfs-tenant-operator/Readme.md).

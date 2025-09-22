
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
- `<PULL-SECRET-NAME>`: The name of your image pull secret for accessing container images.
- `<PANFS_DFC_IMAGE>`: The full image reference for the PanFS DFC container.
- `<KERNEL_VERSION>`: The kernel version required for your environment.

Apply the chosen manifest:
```bash
kubectl apply -f <selected-driver-manifest>.yaml
```

## 2. Deploying the StorageClass and Secret

Choose the manifest that matches your namespace and topology requirements:

- **Secret and StorageClass in a dedicated namespace (with KMM topology):**
	- Use: `storage-class/default.yaml`
- **Secret and StorageClass in the CSI Driver Namespace:**
	- Use: `storage-class/with-secret-in-driver-ns.yaml`
- **Secret and StorageClass in a dedicated namespace (without KMM topology):**
	- Use: `storage-class/with-secret-in-dedicated-ns-without-kmm.yaml`

**Important:**
- Review all parameters and comments in the manifest.
- Replace all placeholders (e.g., `<REALM_ADDRESS>`, `<REALM_USERNAME>`, `<REALM_PASSWORD>`, `<REALM_PRIVATE_KEY>`, `<CSI_CONTROLLER_SA>`, `<CSI_NAMESPACE>`) with your actual values.
- Ensure that the namespace values match your deployment.

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
	accessModes:
		- ReadWriteOnce
	resources:
		requests:
			storage: 10Gi
	storageClassName: csi-panfs-storage-class-name
```

Apply with:
```bash
kubectl apply -f <your-pvc-manifest>.yaml
```

## 4. Additional Notes

- Review all comments in the manifests for guidance on configuration and permissions.
- Ensure that any referenced ServiceAccounts, Roles, and RoleBindings are created and configured as described.
- For troubleshooting, check the logs of the CSI driver pods and verify that secrets and StorageClass parameters are correct.
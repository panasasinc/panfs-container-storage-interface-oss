
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
<!-- 
  Copyright 2025 VDURA Inc.

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
-->

# Kernel Module Management (KMM) Guide

## Overview

Kernel Module Management (KMM) automates the building, loading, and management of kernel modules in Kubernetes clusters. In this project, KMM is used to ensure the PanFS kernel module is available and loaded on all required nodes.

---

# Prerequisites: KMM Engine Configuration

Before deploying the PanFS CSI Driver, ensure the KMM engine and its dependencies are properly configured.

## 1. Install cert-manager

```sh
helm repo add jetstack https://charts.jetstack.io --force-update
helm upgrade --install \
  cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --version v1.18.0 \
  --set crds.enabled=true
```

## 2. Install Kernel Module Management (KMM)

```sh
kubectl apply -k https://github.com/kubernetes-sigs/kernel-module-management/config/default
```

## 3. Verify KMM Installation

```sh
kubectl get all -n cert-manager
kubectl get crd | grep kmm
```

---

# CSI-Driver Deployment Steps

After KMM is installed and configured, proceed with the CSI Driver deployment.

## 1. Prepare Image Pull Secrets

Ensure your image pull secret is created in the `csi-panfs` namespace:

```sh
kubectl create namespace csi-panfs --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret docker-registry <imagePullSecret> \
  --docker-server=https://us-central1-docker.pkg.dev \
  --docker-username=_json_key \
  --docker-password="$REGISTRY_CREDS" \
  --docker-email=org-registry-admin@email \
  --namespace=csi-panfs \
  --dry-run=client -o json | kubectl apply -f -
```

## 2. PanFS Module KMM Configuration

KMM is configured via a `Module` custom resource. For PanFS, see the `Module` manifest in `deploy/k8s/csi-panfs-driver.yaml`:

```yaml
apiVersion: kmm.sigs.x-k8s.io/v1beta1
kind: Module
metadata:
  name: panfs
  namespace: csi-panfs
spec:
  moduleLoader:
    container:
      modprobe:
        moduleName: panfs
      imagePullPolicy: Always
      kernelMappings:
        - literal: 4.18.0-553.el8_10.x86_64
          containerImage: us-central1-docker.pkg.dev/labvirtualization/vdura-csi/panfs-dfc-kmm:4.18.0-553.el8_10.x86_64
  imageRepoSecret:
    name: <imagePullSecret>
  selector:
    node-role.kubernetes.io/worker: ""
```

- **module Name**: Name of the kernel module to load
- **kernelMappings**: Maps kernel versions to container images containing the module
- **imageRepoSecret**: Secret for pulling images from private registries
- **selector**: Node selector for targeting specific nodes

### About the `containerImage` Format

The `containerImage` field specifies the full path to the container image that contains the kernel module for a specific kernel version. The format is:

```
containerImage: <registry>/<repository>/<image-name>:<tag>
```

For example:

```
containerImage: us-central1-docker.pkg.dev/labvirtualization/vdura-csi/panfs-dfc-kmm:4.18.0-553.el8_10.x86_64
```

- `us-central1-docker.pkg.dev/labvirtualization/vdura-csi` — the container registry and repository
- `panfs-dfc-kmm` — the image name
- `4.18.0-553.el8_10.x86_64` — the tag, which must match the kernel version of the target Linux/worker node

This ensures that the correct kernel module is loaded for each node, based on its kernel version.

## 3. Deploy the PanFS CSI Driver and KMM Module

Deploy using Helm:
```
helm upgrade --install csi-panfs charts/panfs \
  --namespace csi-panfs --wait
```

## 4. Validating PanFS KMM Module installation:

Checking KMM module status:
```
kubectl get module panfs -n csi-panfs
NAME    AGE
panfs   1m

kubectl get module panfs -n csi-panfs -o custom-columns=\
NODES:.status.moduleLoader.nodesMatchingSelectorNumber,\
LOADED:.status.moduleLoader.availableNumber,\
DESIRED:.status.moduleLoader.desiredNumber
NODES   LOADED   DESIRED
6       6        6
```
---

## 5. Troubleshooting

- Ensure the correct kernel version is specified in `kernelMappings`.
- Check pod logs for KMM and PanFS module loader containers.
- Verify image pull secrets and permissions.
- Follow [KMM Troubleshooting guide](https://kmm.sigs.k8s.io/documentation/troubleshooting/).

---

## 6. Cleanup

To remove the PanFS CSI Driver, KMM module, and related resources from your cluster:


```sh
helm uninstall csi-panfs --namespace csi-panfs --wait
```

---

**References:**
- [KMM Documentation](https://kmm.sigs.k8s.io/)
- [PanFS CSI Driver Makefile](../Makefile)
- [KMM Module Example](../deploy/k8s/csi-panfs-driver.yaml)

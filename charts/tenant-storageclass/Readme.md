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

# panfs-tenant-storageclass

PanFS Tenant StorageClass Chart

![Version: 2.0.0](https://img.shields.io/badge/Version-2.0.0-informational?style=flat-square) ![AppVersion: 2.0.0](https://img.shields.io/badge/AppVersion-2.0.0-informational?style=flat-square)

## What the PanFS Tenant StorageClass chart is

The chart creates one `PanfsStorageClass` resource as a Helm release. The
[PanFS StorageClass operator](../storageclass-operator/Readme.md) then creates the StorageClass of the tenant from it.

The name of the release is the name of the `PanfsStorageClass` and of the StorageClass.
The namespace of the release is the namespace of the tenant.

### Resources Created by the Helm Chart

- A `PanfsStorageClass` with the name of the release.

The operator then creates the StorageClass, and a `Role` and a `RoleBinding` in the same namespace;
see the [operator documentation](../storageclass-operator/Readme.md#resources-created-by-the-operator).

## Requirements

- **The PanFS StorageClass operator is installed** (chart [`panfs-storageclass-operator`](../storageclass-operator/Readme.md)).
  The chart checks this before it creates anything, and it stops with a message when the operator is missing.
  The check looks for the resource kind `PanfsStorageClass`, so it proves that the operator chart is installed,
  not that the operator pod runs at this moment.
- **The namespace has a `TenantNetwork`**, for example from the [tenant chart](../tenant/Readme.md).
  The operator finds it: a namespace holds the network of one tenant. For a realm that is reachable without
  a tenant network, set `realmSecretName` instead.
- **The name of the release is free:** a StorageClass has no namespace, so its name must be unique in the cluster.

## Usage

### 1. Install a release in the namespace of the tenant

No value is needed in the namespace of a `TenantNetwork`:

```bash
helm upgrade --install acme-storage ./ --namespace acme
```

It is the same as this manifest:

```yaml
apiVersion: panfs.vdura.com/v1alpha1
kind: PanfsStorageClass
metadata:
  name: acme-storage
  namespace: acme
```

#### Using Helm overrides:

```yaml
# overrides.yaml
parameters:
  panfs.csi.vdura.com/layout: "raid6+"
  panfs.csi.vdura.com/stripeunit: "64k"

reclaimPolicy: Retain
```

```bash
helm upgrade --install acme-storage ./ \
    --namespace acme \
    --values overrides.yaml
```

The parameters, the reclaim policy and the volume binding mode cannot change after the StorageClass was created;
see [Changing a PanfsStorageClass](../storageclass-operator/Readme.md#changing-a-panfsstorageclass).

#### Realm without a tenant network

In a namespace without a `TenantNetwork`, name the Secret with the keys `api` and `token`:

```bash
helm upgrade --install realm-storage ./ \
    --namespace <NAMESPACE> \
    --set realmSecretName=<SECRET_NAME>
```

### 2. Check the result

```bash
kubectl -n acme get panfsstorageclass acme-storage
kubectl -n acme describe panfsstorageclass acme-storage
kubectl get storageclass acme-storage
```

The phase must become `Ready`. The phase `Pending` means that the `TenantNetwork` is not ready yet.
The status and the events are described in the [operator documentation](../storageclass-operator/Readme.md#status).

### Rendering the chart without a cluster

`helm template` has no cluster to ask, so the check for the operator fails there. Tell Helm that the kind exists:

```bash
helm template acme-storage ./ --namespace acme \
    --api-versions panfs.vdura.com/v1alpha1/PanfsStorageClass
```

`helm lint` cannot take that flag; switch the check off there with `--set checkOperator=false`.

### Uninstalling the chart

```bash
helm uninstall acme-storage --namespace acme
```

The uninstall deletes the `PanfsStorageClass`, and the operator deletes the StorageClass. Existing volumes keep working;
see [Deleting a PanfsStorageClass](../storageclass-operator/Readme.md#deleting-a-panfsstorageclass).

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| allowVolumeExpansion | bool | `nil` | Whether a volume can be expanded. Empty means the default of the operator: `true`. |
| checkOperator | bool | `true` | Check that the PanFS StorageClass operator is installed before anything is created. Set it to `false` only to render or lint the chart without a cluster. |
| default | bool | `false` | Make the StorageClass the default one of the cluster |
| mountOptions | list | `[]` | Mount options of the volumes |
| parameters | object | `{}` | PanFS parameters of new volumes, for example `panfs.csi.vdura.com/layout`. Without them the defaults of the driver apply. They cannot change after the StorageClass was created. |
| realmSecretName | string | `""` | Only for a realm that is reachable without a tenant network, in a namespace without a `TenantNetwork`: name of a Secret in the namespace of the release with the keys `api` and `token`, and optionally `ca_cert`. Leave it empty in the namespace of a `TenantNetwork`: the operator uses the Secret of the `TenantNetwork`. |
| reclaimPolicy | string | `""` | `Delete` or `Retain`. Empty means the default of the operator: `Delete`. It cannot change after the StorageClass was created. |
| volumeBindingMode | string | `""` | `Immediate` or `WaitForFirstConsumer`. Empty means the default of the operator: `Immediate` with a tenant network, `WaitForFirstConsumer` without one. It cannot change after the StorageClass was created. |

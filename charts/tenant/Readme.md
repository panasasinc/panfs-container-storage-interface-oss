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

# panfs-tenant

PanFS Tenant Chart

![Version: 2.0.0](https://img.shields.io/badge/Version-2.0.0-informational?style=flat-square) ![AppVersion: 2.0.0](https://img.shields.io/badge/AppVersion-2.0.0-informational?style=flat-square)

## What the PanFS Tenant chart is

The PanFS Tenant chart holds what one PanFS tenant needs in a Kubernetes cluster, in one Helm release:

- a Secret with the connection of the tenant to the realm: the PanAPI address and the token,
- a `TenantNetwork` resource, which asks the [PanFS Tenant Network operator](../tenant-network-operator/Readme.md)
  to put the nodes of the tenant on the network of that tenant.

It replaces three manual steps: create the namespace, create the Secret, and apply the `TenantNetwork`.
Install one release per tenant, each in its own namespace.

> **Note:** Installation into the `default` namespace is not permitted. You must specify a non-default namespace using the `--namespace <namespace>` option during installation.

### Resources Created by the Helm Chart

- A `Secret` with the name of the release and the keys `api` and `token`, and optionally `ca_cert`.
  It is not created when you name your own Secret with `existingSecret`.
- A `TenantNetwork` with the name of the release, which names that Secret.

The operator then creates the DaemonSet of node pods and its permissions in the same namespace;
see the [operator documentation](../tenant-network-operator/Readme.md#resources-created-by-the-operator).

## Requirements

- **The PanFS Tenant Network operator is installed** (chart [`panfs-tenant-network-operator`](../tenant-network-operator/Readme.md)).
  The chart checks this before it creates anything, and it stops with a message when the operator is missing.
  The check looks for the resource kind `TenantNetwork`, so it proves that the operator chart is installed,
  not that the operator pod runs at this moment.
- **One release per namespace.** A namespace holds one `TenantNetwork`, because its name is the value of the node label
  `panfs.vdura.com/tenant-network`.
- The requirements of the tenant network itself (realm, token permissions, namespace, Calico) are in the
  [operator documentation](../tenant-network-operator/Readme.md#requirements).

## Usage

### 1. Install a release for the tenant

```bash
helm upgrade --install acme ./ \
    --namespace acme \
    --create-namespace \
    --set realm.api=<PANAPI_ADDRESS> \
    --set realm.token=<TOKEN>
```

Without more values, all nodes of the cluster are put on the tenant network. To use only some nodes, label them
with a label of your choice and give the chart the same label:

```yaml
# overrides.yaml
realm:
  api: "https://realm.example.com"
  token: "<TOKEN>"

nodeSelector:
  storage-network: acme
```

```bash
kubectl label node <NODE_NAME> storage-network=acme

helm upgrade --install acme ./ \
    --namespace acme \
    --create-namespace \
    --values overrides.yaml
```

#### Keeping the token out of the Helm values

Helm stores the values of a release in the cluster. To keep the token out of them, create the Secret yourself
and name it:

```bash
kubectl create namespace acme
kubectl -n acme create secret generic acme-credentials \
    --from-literal=api=<PANAPI_ADDRESS> \
    --from-literal=token=<TOKEN>

helm upgrade --install acme ./ \
    --namespace acme \
    --set existingSecret=acme-credentials
```

### 2. Check the result

```bash
kubectl -n acme get tenantnetwork acme
kubectl -n acme describe tenantnetwork acme
```

The phase must become `Ready`. A resource that stays without a status means that the operator is not running.
The status and the events are described in the [operator documentation](../tenant-network-operator/Readme.md#4-check-the-result).

A ready node has the label `panfs.vdura.com/tenant-network=acme`, with the namespace of the release as its value.
Use it in `allowedTopologies` of the StorageClass of the tenant and in the node selector of its workloads;
see [Place workloads on the ready nodes](../tenant-network-operator/Readme.md#5-place-workloads-on-the-ready-nodes).

### Rendering the chart without a cluster

`helm template` has no cluster to ask, so the check for the operator fails there. Tell Helm that the kind exists:

```bash
helm template acme ./ --namespace acme \
    --api-versions panfs.vdura.com/v1alpha1/TenantNetwork \
    --set realm.api=<PANAPI_ADDRESS> --set realm.token=<TOKEN>
```

`helm lint` cannot take that flag; switch the check off there with `--set checkOperator=false`.

### Uninstalling the chart

Remove the workloads that use volumes of the tenant first. Then:

```bash
helm uninstall acme --namespace acme
```

The uninstall deletes the `TenantNetwork`, and the operator takes the nodes off the tenant network: every pod removes
the ready label and the tenant interface of its node, and detaches the node from the subnet in the realm.
The command returns before that is finished; `kubectl -n acme get tenantnetwork` shows the phase `Removing` until it is.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| checkOperator | bool | `true` | Stop the install when the PanFS Tenant Network operator is not installed in the cluster. Set it to `false` for tools that render the chart without a cluster and cannot pass `--api-versions panfs.vdura.com/v1alpha1/TenantNetwork`, for example `helm lint`. |
| detachOnRemoval | string | `null` | Whether a pod removes the tenant interface and detaches its node from the subnet when the node leaves the tenant. Leave it unset (`null`) to use the default of the operator. |
| existingSecret | string | `""` | Name of an existing Secret in the release namespace with the keys `api` and `token`, and optionally `ca_cert`. When it is set, the chart creates no Secret and `realm` is not used. Use it to keep the token out of the Helm values. |
| firewalldZone | string | `null` | firewalld zone of the tenant interface on nodes that run firewalld; an empty string leaves firewalld alone. Leave it unset (`null`) to use the default of the operator. |
| interval | string | `""` | Time between two checks of the tenant network on a node, for example `30s`. Leave it empty to use the default of the operator. |
| nodeSelector | object | `{}` | Labels that select the nodes of the tenant. Without them, all nodes of the cluster are put on the tenant network. |
| realm.api | string | `""` | PanAPI address of the PanFS realm: a host name or IP address (HTTPS is used), or a full `https://` or `http://` URL. |
| realm.caCert | string | `""` | Optional PEM certificate of the CA that signed the realm certificate. Leave empty to use the system trust store. |
| realm.insecureSkipTLSVerify | bool | `false` | Do not verify the realm certificate. Use it only for a test realm whose certificate cannot be verified: a host that poses as the realm can then read the token. Prefer `realm.caCert`. |
| realm.token | string | `""` | PanAPI service account token of the tenant |
| vlanParentInterface | string | `""` | Only for a realm that places subnets on VLAN segments: the interface of the nodes that carries the VLAN. Leave it empty to use the default of the operator: the interface with the underlay address of the node. |

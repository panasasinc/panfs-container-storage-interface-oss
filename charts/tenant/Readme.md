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
- a `TenantNetwork` resource, which asks the [PanFS Tenant operator](../panfs-tenant-operator/Readme.md)
  to put the nodes of the tenant on the network of that tenant.

It replaces three manual steps: create the namespace, create the Secret, and apply the `TenantNetwork`.
Install one release per tenant, each in its own namespace.

> **Note:** Installation into the `default` namespace is not permitted. You must specify a non-default namespace using the `--namespace <namespace>` option during installation.

### Resources Created by the Helm Chart

- A `Secret` with the name of the release and the keys `api` and `token`, and optionally `ca_cert`.
  It is not created when you name your own Secret with `existingSecret`.
- A `TenantNetwork` with the name of the release, which names that Secret.
- A `ServiceAccount`, a `Role` and a `RoleBinding` named `<release>-test`, for the pod of `helm test`;
  see [Testing the release](#testing-the-release).

The operator then creates the DaemonSet of node pods and its permissions in the same namespace;
see the [operator documentation](../panfs-tenant-operator/Readme.md#resources-created-by-the-operator).

## Requirements

- **The PanFS Tenant operator is installed** (chart [`panfs-tenant-operator`](../panfs-tenant-operator/Readme.md)).
  The chart checks this before it creates anything, and it stops with a message when the operator is missing.
  The check looks for the resource kind `TenantNetwork`, so it proves that the operator chart is installed,
  not that the operator pod runs at this moment.
- **One release per namespace.** A namespace holds one `TenantNetwork`, because its name is the value of the node label
  `panfs.vdura.com/tenant-network`. The chart checks this before it creates anything: the install stops when the
  namespace has a `TenantNetwork` of another release.
- **With `existingSecret`, the Secret exists** and has the keys `api` and `token`. The chart checks this too.
  Helm reads the `TenantNetwork` resources and that Secret with your permissions. Set `checkNamespace=false`
  to install first and let the operator report the state in the status of the resource.
- The requirements of the tenant network itself (realm, token permissions, namespace, Calico) are in the
  [operator documentation](../panfs-tenant-operator/Readme.md#requirements).

## Usage

### 1. Install a release for the tenant

Label the nodes that mount volumes of the tenant, with a label of your choice, and give the chart the same label:

```bash
kubectl label node <NODE_NAME> storage-network=acme

helm upgrade --install acme ./ \
    --namespace acme \
    --create-namespace \
    --set realm.api=<PANAPI_ADDRESS> \
    --set realm.token=<TOKEN> \
    --set nodeSelector.storage-network=acme
```

Without `nodeSelector`, all nodes of the cluster are put on the tenant network, the control plane nodes included.
A label key with dots needs a backslash before each dot on the command line, for example
`--set nodeSelector."example\.com/storage-network"=acme`; a values file needs none.

#### Using Helm overrides:

```yaml
# overrides.yaml
realm:
  api: "https://realm.example.com"
  token: "<TOKEN>"

nodeSelector:
  storage-network: acme
```

```bash
helm upgrade --install acme ./ \
    --namespace acme \
    --create-namespace \
    --values overrides.yaml
```

#### Realm with a certificate that the cluster does not trust

When `realm.api` is an `https://` address, the operator and the node pods check the certificate of the realm.
The check needs two things: the issuer must be trusted, and the certificate must name the address of the realm
as a subject alternative name.

If the certificate is signed by a private CA, give the chart the certificate of that CA:

```bash
helm upgrade --install acme ./ \
    --namespace acme \
    --create-namespace \
    --set realm.api=https://<PANAPI_ADDRESS> \
    --set realm.token=<TOKEN> \
    --set-file realm.caCert=ca.pem
```

If the certificate cannot be verified at all, for example because it has no subject alternative name,
the `TenantNetwork` shows the phase `Error` with a message like
`x509: certificate relies on legacy Common Name field, use SANs instead`.
The CA certificate does not help then. Switch the check off:

```bash
helm upgrade --install acme ./ \
    --namespace acme \
    --create-namespace \
    --set realm.api=https://<PANAPI_ADDRESS> \
    --set realm.token=<TOKEN> \
    --set realm.insecureSkipTLSVerify=true \
    --set nodeSelector.storage-network=acme
```

> **Note:** with `realm.insecureSkipTLSVerify=true` the connection is still encrypted, but the identity of the realm
> is not checked: a host on the network that poses as the realm can read the token. The operator and the node pods
> log a warning about it on every start. Use it when the network between the cluster and the realm is trusted.
> A realm certificate that names the address of the realm as a subject alternative name removes the need for it.

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

Another way is to use the chart only as a template and apply its output yourself. Helm then stores nothing,
because there is no release:

```bash
kubectl create namespace acme

helm template acme ./ \
    --namespace acme \
    --api-versions panfs.vdura.com/v1alpha1/TenantNetwork \
    --set realm.api=<PANAPI_ADDRESS> \
    --set realm.token=<TOKEN> | kubectl apply -f -
```

`--api-versions` is needed because `helm template` does not ask the cluster; see
[Rendering the chart without a cluster](#rendering-the-chart-without-a-cluster). So the check for the operator
does not run here: without the operator, `kubectl apply` refuses the `TenantNetwork` itself.
`helm list`, `helm upgrade` and `helm uninstall` do not know this tenant. To change it, run the command again;
to remove it, send the same output to `kubectl delete -f -`.

### 2. Check the result

```bash
kubectl -n acme get tenantnetwork acme
kubectl -n acme describe tenantnetwork acme
```

The phase must become `Ready`. A resource that stays without a status means that the operator is not running.
The status and the events are described in the [operator documentation](../panfs-tenant-operator/Readme.md#4-check-the-result).

A ready node has the label `panfs.vdura.com/tenant-network=acme`, with the namespace of the release as its value.
To create a StorageClass of the tenant, install the chart [`panfs-storageclass`](../storageclass/Readme.md)
in the same namespace when the phase is `Ready`. Every volume of that StorageClass is bound to the ready nodes;
see [Create the StorageClass of the tenant](../panfs-tenant-operator/Readme.md#5-create-the-storageclass-of-the-tenant).

### Testing the release

The install does not wait for the operator and the node pods. `helm test` checks the result:
it runs a pod that waits until the `TenantNetwork` has the phase `Ready`.

```bash
helm test acme --namespace acme
```

The command fails when the phase is not `Ready` after `test.timeout` (300 seconds by default).
The status and the events of the resource then say why: `kubectl -n acme describe tenantnetwork acme`.

- The test pod uses the image `test.image`, which must have `kubectl`. For a private registry, set `test.image`
  and `test.imagePullSecrets`.
- The `Role` of the test pod allows reading the `TenantNetwork` of the release, and nothing else.
  Set `test.enabled=false` to leave the three objects out; `helm test` then has no test.
- A test pod that passed is deleted. A pod that failed stays, so that its log can be read, and `helm uninstall`
  does not remove it: `kubectl -n acme delete pod acme-test`.

### Rendering the chart without a cluster

`helm template` has no cluster to ask, so the check for the operator fails there, and the check of the namespace
is skipped. Tell Helm that the kind exists:

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
| checkNamespace | bool | `true` | Check the namespace before anything is created: it has no other `TenantNetwork`, and the Secret that `existingSecret` names exists with the keys `api` and `token`. The install stops with a message otherwise. Helm reads these objects with your permissions. Set it to `false` to install first and let the operator report the state in the status of the resource. |
| checkOperator | bool | `true` | Stop the install when the PanFS Tenant operator is not installed in the cluster. Set it to `false` for tools that render the chart without a cluster and cannot pass `--api-versions panfs.vdura.com/v1alpha1/TenantNetwork`, for example `helm lint`. |
| detachOnRemoval | string | `null` | Whether a pod removes the tenant interface and detaches its node from the subnet when the node leaves the tenant. Leave it unset (`null`) to use the default of the operator. |
| existingSecret | string | `""` | Name of an existing Secret in the release namespace with the keys `api` and `token`, and optionally `ca_cert`. When it is set, the chart creates no Secret and `realm` is not used. Use it to keep the token out of the Helm values. |
| firewalldZone | string | `null` | firewalld zone of the tenant interface on nodes that run firewalld; an empty string leaves firewalld alone. Leave it unset (`null`) to use the default of the operator. |
| interval | string | `""` | Time between two checks of the tenant network on a node, for example `30s`. Leave it empty to use the default of the operator. |
| nodeSelector | object | `{}` | Labels that select the nodes of the tenant. Without them, all nodes of the cluster are put on the tenant network. |
| realm.api | string | `""` | PanAPI address of the PanFS realm: a host name or IP address (HTTPS is used), or a full `https://` or `http://` URL. |
| realm.caCert | string | `""` | Optional PEM certificate of the CA that signed the realm certificate. Leave empty to use the system trust store. |
| realm.insecureSkipTLSVerify | bool | `false` | Do not verify the realm certificate. Use it for a realm whose certificate cannot be verified, for example because it has no subject alternative name. The connection stays encrypted, but the identity of the realm is not checked: a host that poses as the realm can read the token. Prefer `realm.caCert` where it works. |
| realm.token | string | `""` | PanAPI service account token of the tenant |
| test.enabled | bool | `true` | Create the ServiceAccount, the Role and the RoleBinding of the test pod with the release. The Role allows reading the TenantNetwork of the release only. With `false`, `helm test` has no test. |
| test.image | string | `"registry.k8s.io/kubectl:v1.33.1"` | Image of the test pod. It must have `kubectl` |
| test.imagePullSecrets | list | `[]` | List of image pull secrets of the test pod, for an image in a private registry |
| test.timeout | string | `"300s"` | How long the test waits for the phase `Ready`. The node pods must start and reach the realm first |
| vlanParentInterface | string | `""` | Only for a realm that places subnets on VLAN segments: the interface of the nodes that carries the VLAN. Leave it empty to use the default of the operator: the interface with the underlay address of the node. |

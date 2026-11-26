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

# panfs-tenant-network

PanFS Tenant Network Chart

![Version: 2.0.0](https://img.shields.io/badge/Version-2.0.0-informational?style=flat-square) ![AppVersion: 2.0.0](https://img.shields.io/badge/AppVersion-2.0.0-informational?style=flat-square)

## What PanFS Tenant Network is

A volume of a PanFS tenant is reachable only from the network of that tenant.
The PanFS Tenant Network operator puts Kubernetes nodes on that network, and it tells Kubernetes which nodes are ready.

Install this chart once per cluster. After that, a tenant needs two small things in one namespace:
a Secret with the PanAPI address and the token of the tenant, and a `TenantNetwork` that names the Secret
and selects the nodes:

```yaml
apiVersion: panfs.vdura.com/v1alpha1
kind: TenantNetwork
metadata:
  name: acme-network
  namespace: acme
spec:
  secretName: acme-credentials
  nodeSelector:
    storage-network: acme
```

For every `TenantNetwork` the operator does the following:

1. Reads the Secret and asks PanAPI which tenant the token belongs to. You do not write the tenant name anywhere.
2. Creates a DaemonSet in the namespace of the resource. Its pods run on the nodes that `nodeSelector` selects.
3. Reports the result in the status of the resource and as events on it.

Two node labels are involved:

| Label                                        | Who sets it  | Meaning                                      |
|----------------------------------------------|--------------|----------------------------------------------|
| The labels of `nodeSelector`                 | You          | The node is meant for this tenant            |
| `panfs.vdura.com/tenant-network=<namespace>` | The node pod | The network of the tenant works on this node |

The value of the second label is the namespace of the `TenantNetwork`. It is the label for placing workloads,
and the status of the resource shows it as `Ready Label`.
Without `nodeSelector`, the pods run on all nodes of the cluster, the control plane nodes included:

```yaml
spec:
  secretName: acme-credentials
```

A node pod does the following on its node, and repeats it on an interval:

1. Attaches the node to the subnet of the tenant through PanAPI. A repeated attach does not move the node.
2. Builds the tenant interface, or corrects it in place. An interface that is already right is not touched.
   The realm decides the kind: `vxlan<id>` on a VXLAN segment, `vlan<id>` on a VLAN segment; see [Segment types](#segment-types).
3. Moves the interface to the firewalld zone `trusted`, when firewalld runs on the node.
4. Checks that a director of the realm answers on the tenant interface.
5. Sets the second label on the node when the check passes, and removes it when the check fails.

A node pod is Ready only while the check passes.

When a node leaves the tenant, its pod cleans up: it removes the ready label, removes the tenant interface,
and detaches the node from the subnet in the realm. A node leaves the tenant when it no longer matches `nodeSelector`
(you removed its label, or you changed the selector), or when the `TenantNetwork` is deleted. A restart or an upgrade of the pods is not a removal:
the interface and the attachment stay, so mounted volumes keep working.

> **Note:** Installation into the `default` namespace is not permitted. You must specify a non-default namespace using the `--namespace <namespace>` option during installation.

### Resources Created by the Helm Chart

- The `CustomResourceDefinition` of `TenantNetwork` (`tenantnetworks.panfs.vdura.com`).
- A `Deployment` with one pod: the operator. With the release name of this guide it is `panfs-tenant-network-operator`.
- A `ServiceAccount`, a `ClusterRole` and a `ClusterRoleBinding` for the operator.
- A `ClusterRole` for the node pods: read a node and patch its labels.

### Resources Created by the Operator

For every `TenantNetwork`, in the namespace of the resource:

- A `DaemonSet` with the name `panfs-tenant-network-<name>` that runs the node pods.
- A `ServiceAccount`, a `Role` and a `RoleBinding` for these pods. The pods read their own DaemonSet when they stop,
  to tell a removal from a restart.
- A `ClusterRoleBinding` that binds the `ClusterRole` of the node pods to that ServiceAccount.

The operator does not create the Secret.

## Requirements

- **PanFS realm:** the tenant has a subnet on a VXLAN or a VLAN segment; see [Segment types](#segment-types).
- **Secret:** a Secret in the namespace of the `TenantNetwork` with the keys `api` and `token`, and optionally `ca_cert`.
  The Secret of a `csi-panfs-storageclass` release has these keys.
- **Token permissions**, all for its own tenant: read the tenant; list the subnets and read the mount recipe of a subnet;
  list, read, create and delete attachments of a subnet.
- **PanAPI is reachable from the operator pod.** The operator runs on the pod network and calls PanAPI
  to learn the tenant of a token.
- **Tenant namespace:** the namespace of a `TenantNetwork` must allow pods that use the host network and a `hostPath` volume
  (Pod Security level `privileged`). The namespace of the operator has no such need.
- **One node, one tenant:** a node can be on the network of one tenant only, so a node must match the `nodeSelector`
  of one `TenantNetwork` only. The operator does not check this.
- **One `TenantNetwork` per tenant and per namespace:** the operator serves a tenant through the oldest `TenantNetwork`
  of that tenant, and a namespace holds one `TenantNetwork`, because its name is the value of the ready label.
  The operator reports an error on every other one.
- **Calico:** with `IP_AUTODETECTION_METHOD` left at its default, Calico can move a node to its new tenant address,
  and the BGP sessions between the nodes then fail. Pin Calico to the node address before you label the first node:

  ```bash
  kubectl -n kube-system set env daemonset/calico-node IP_AUTODETECTION_METHOD=kubernetes-internal-ip
  kubectl -n kube-system rollout status daemonset/calico-node
  ```

  Other network plugins that pick the node address by themselves need the same care. The operator does not change the network plugin.

### Segment types

The realm places the subnets of its tenants on one kind of network segment, and PanAPI tells every node which one:
the record of a node on the subnet carries the segment type and the segment id. You do not configure the kind.

| Segment type | Tenant interface of a node                                 | What the node needs                                      |
|--------------|------------------------------------------------------------|----------------------------------------------------------|
| `vxlan`      | `vxlan<VNI>`, a tunnel over the underlay network           | UDP to the realm nodes on the tunnel port                |
| `vlan`       | `vlan<VLAN id>`, an 802.1Q interface on a parent interface | The switch ports of the node must carry that VLAN        |
| `none`       | None                                                       | Not supported; the node stays `NotReady` with the reason |

On a VLAN segment the parent interface is the interface of the node that has its underlay address.
To use another one, name it: the value `vlanParentInterface` of the chart for all tenants,
or `spec.vlanParentInterface` of a `TenantNetwork` for one tenant.
The operator cannot configure or check the switches: when the VLAN does not reach a node,
its interface is built, but no director answers, and the node stays `NotReady`.

> **Note:** VLAN segments are covered by unit tests and by a check of the interface commands on a node.
> They are not yet validated against a realm that uses VLAN segments.

### Access of the operator to Secrets

The operator must read the Secret of every `TenantNetwork`, in any namespace, so its `ClusterRole` allows `get` on Secrets
in the whole cluster. It cannot list Secrets, and it reads only the Secrets that a `TenantNetwork` names.
Consider this when several teams share the cluster: who may create a `TenantNetwork` in a namespace
can make the operator use a Secret of that namespace.

## Usage

### 1. Install the operator

Once per cluster:

```bash
helm upgrade --install panfs-tenant-network-operator ./ \
    --namespace panfs-tenant-network \
    --create-namespace \
    --wait
```

#### Using Helm overrides:

```yaml
# overrides.yaml
# Only for an image in a private registry. The Secret must exist in the namespace of the operator
# and in the namespace of every TenantNetwork.
imagePullSecrets:
  - my-registry-secret
```

```bash
helm upgrade --install panfs-tenant-network-operator ./ \
    --namespace panfs-tenant-network \
    --create-namespace \
    --values overrides.yaml \
    --wait
```

### 2. Label the nodes of the tenant

Any label works; the `TenantNetwork` selects the nodes by it. Skip this step to use all nodes of the cluster:

```bash
kubectl label node <NODE_NAME> <LABEL_KEY>=<LABEL_VALUE>
```

### 3. Create the Secret and the TenantNetwork of the tenant

```bash
kubectl create namespace <NAMESPACE>

# The credentials of the tenant
kubectl -n <NAMESPACE> create secret generic <SECRET_NAME> \
    --from-literal=api=<PANAPI_ADDRESS> \
    --from-literal=token=<TOKEN>

cat <<EOF | kubectl -n <NAMESPACE> apply -f -
apiVersion: panfs.vdura.com/v1alpha1
kind: TenantNetwork
metadata:
  name: <NAME>
spec:
  secretName: <SECRET_NAME>
  nodeSelector:
    <LABEL_KEY>: <LABEL_VALUE>
EOF
```

The Secret of a [PanFS StorageClass](../storageclass/Readme.md) release has the keys that the operator needs.
To use it, create the `TenantNetwork` in the namespace of that release and name its Secret; the Secret has the name of the release.

Fields of `spec`:

| Field                 | Required | Meaning                                                                                |
|-----------------------|----------|----------------------------------------------------------------------------------------|
| `secretName`          | Yes      | Name of the Secret with the PanAPI address and the token of the tenant                 |
| `nodeSelector`        | No       | Labels that select the nodes of the tenant; without it, all nodes of the cluster       |
| `vlanParentInterface` | No       | Interface that carries the VLAN on a VLAN segment; see [Segment types](#segment-types) |
| `interval`            | No       | Replaces the value `interval` of the chart for this tenant                             |
| `firewalldZone`       | No       | Replaces the value `firewalldZone` of the chart for this tenant                        |
| `detachOnRemoval`     | No       | Replaces the value `detachOnRemoval` of the chart for this tenant                      |

### 4. Check the result

```bash
kubectl get tenantnetworks -A
kubectl -n <NAMESPACE> describe tenantnetwork <NAME>
```

```text
NAMESPACE   NAME           TENANT   NODES   READY   PHASE   AGE
acme        acme-network   acme     3       3       Ready   1m
```

| Phase      | Meaning                                                                                          |
|------------|--------------------------------------------------------------------------------------------------|
| `Ready`    | The tenant network works on every selected node                                                  |
| `Pending`  | No node matches the selector, or some nodes are not ready yet                                    |
| `Error`    | The operator cannot serve the resource; `kubectl get tenantnetworks -A -o wide` shows the reason |
| `Removing` | The resource is being deleted; the operator waits for the node pods to clean up                  |

`kubectl describe` shows the state of every node and what happened recently:

```text
Status:
  Interface:      vxlan5001
  Nodes:
    worker-1:
      Overlay IP:  192.168.0.57
      State:       Ready
    worker-2:
      Message:     no tenant director answers on vxlan5001: ...
      Overlay IP:  192.168.0.58
      State:       NotReady
  Phase:          Pending
  Ready Label:    panfs.vdura.com/tenant-network=acme
  Tenant:         acme
Events:
  Type     Reason               From                           Message
  Normal   SecretFound          panfs-tenant-network-operator  Using Secret acme-credentials; its token belongs to tenant acme
  Normal   DaemonSetCreated     panfs-tenant-network-operator  Created DaemonSet panfs-tenant-network-acme-network ...
  Normal   NodeAttached         panfs-tenant-network           Node worker-1 is attached to the subnet of tenant acme, overlay address 192.168.0.57/16
  Normal   InterfaceConfigured  panfs-tenant-network           Node worker-1: interface vxlan5001 was created or corrected ...
  Normal   NodeReady            panfs-tenant-network           Node worker-1: a tenant director answers on vxlan5001; the label ... is set
  Warning  NodeNotReady         panfs-tenant-network           Node worker-2: no tenant director answers on vxlan5001: ...
```

An event is written when something changes, not on every check, and Kubernetes keeps events for about one hour.
The status always shows the current state. The pods also report on their node, as the annotations
`panfs.vdura.com/tenant-network-overlay-ip`, `-interface` and `-message`.

| Event reason                                   | Meaning                                                        |
|------------------------------------------------|----------------------------------------------------------------|
| `SecretFound`                                  | The operator read the Secret and the tenant of its token       |
| `DaemonSetCreated`, `DaemonSetUpdated`         | The operator created or changed the DaemonSet of the node pods |
| `Ready`, `Pending`, `Removing`                 | The phase changed                                              |
| `NodeAttached`                                 | A node was attached to the subnet of the tenant                |
| `InterfaceConfigured`, `FirewallZoneSet`       | A pod created or corrected the tenant interface of its node    |
| `NodeReady`, `NodeNotReady`                    | The check of a node passed, or failed twice in a row           |
| `NodeStopped`                                  | A pod stopped for a restart; interface and attachment stay     |
| `NodeDetached`                                 | A node left the tenant and was detached                        |
| `InvalidSpec`, `SecretNotFound`, `PanAPIError` | Warning: the operator cannot serve the resource                |
| `TenantAlreadyServed`, `ReconcileFailed`       | Warning: the operator cannot serve the resource                |

For more detail, read the log of the pod of a node:

```bash
kubectl -n <NAMESPACE> logs -l panfs.vdura.com/tenant-network-name=<NAME> --tail=20
```

### 5. Place workloads on the ready nodes

The ready label is `panfs.vdura.com/tenant-network=<NAMESPACE>`, with the namespace of the `TenantNetwork` as its value.
Add this block to the StorageClass of the tenant. With the PanFS StorageClass chart, it is the value `allowedTopologies`:

```yaml
allowedTopologies:
  - matchLabelExpressions:
      - key: panfs.vdura.com/tenant-network
        values:
          - "<NAMESPACE>"
```

Kubernetes checks `allowedTopologies` when it places the first pod that uses a new volume.
For pods that use an existing volume, add a node selector to the workload:

```yaml
nodeSelector:
  panfs.vdura.com/tenant-network: "<NAMESPACE>"
```

### More than one tenant

Create one Secret and one `TenantNetwork` per tenant, each tenant in its own namespace, and label each node for one tenant.
A node without the label gets no pod, and nothing on it changes.

### Adding and removing nodes

- A new node gets a pod as soon as it matches the `nodeSelector` of the `TenantNetwork`.
- When you remove that label from a node, or change `nodeSelector`, Kubernetes deletes the pods of the nodes that no longer match. Before it stops, the pod removes the ready label,
  removes the tenant interface, and detaches the node from the subnet in the realm.
  Move the pods that use volumes of the tenant away from the node first: without the interface their volumes stop answering.

### Removing the network of a tenant

Remove the workloads that use volumes of the tenant first. Then delete the `TenantNetwork`:

```bash
kubectl -n <NAMESPACE> delete tenantnetwork <NAME>
```

The delete waits until the operator has removed the DaemonSet and every pod has cleaned up its node:
the ready label, the tenant interface, and the attachment in the realm are gone afterwards.

To keep the interfaces and the attachments, set `detachOnRemoval` to `false` before you delete:

```bash
kubectl -n <NAMESPACE> patch tenantnetwork <NAME> --type merge -p '{"spec":{"detachOnRemoval":false}}'
kubectl -n <NAMESPACE> rollout status daemonset/panfs-tenant-network-<NAME>
kubectl -n <NAMESPACE> delete tenantnetwork <NAME>
```

### Uninstalling the operator

Delete every `TenantNetwork` first, while the operator runs. Then:

```bash
helm uninstall panfs-tenant-network-operator --namespace panfs-tenant-network
kubectl delete crd tenantnetworks.panfs.vdura.com
```

Helm does not remove or upgrade a `CustomResourceDefinition`, so the last command is yours.
After an upgrade of the chart, apply the new definition by hand: `kubectl apply -f crds/tenantnetwork.yaml`.

Without the operator, the existing node pods keep working, but a `TenantNetwork` cannot be deleted: it waits for the operator.
If the operator is gone for good, clean up by hand:

```bash
kubectl -n <NAMESPACE> delete daemonset panfs-tenant-network-<NAME> --cascade=foreground
kubectl -n <NAMESPACE> patch tenantnetwork <NAME> --type merge -p '{"metadata":{"finalizers":null}}'
kubectl delete clusterrolebinding <NAMESPACE>-panfs-tenant-network-<NAME>
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| detachOnRemoval | bool | `true` | Default for what a node pod does when its node leaves the tenant: the label `panfs.vdura.com/tenant` is removed from the node, or the TenantNetwork is deleted. With `true`, the pod removes the tenant interface and detaches the node from the subnet in the realm. With `false`, both stay. A restart or an upgrade of the pods never detaches a node. |
| firewalldZone | string | `"trusted"` | Default firewalld zone that the tenant interface is moved to on nodes that run firewalld. The realm opens connections to the PanFS client, and the default zone usually drops them. Set it to an empty string to leave firewalld alone. |
| image | string | `...` | Image of the PanFS Tenant Network program. The operator and the pods it creates on the nodes use the same image. |
| imagePullSecrets | list | `[]` | List of image pull secrets for private registries. Each Secret must exist in the namespace of the operator and in the namespace of every TenantNetwork, because the operator creates the node pods there. |
| interval | string | `"30s"` | Default time between two checks of the tenant network on a node |
| labels | object | `{}` | Labels for the operator Deployment and its pod |
| logLevel | int | `2` | Log verbosity (klog level) of the operator and of the node pods |
| nodePriorityClassName | string | `""` | Priority class of the node pods |
| pullPolicy | string | `"IfNotPresent"` | Image pull policy |
| resources | object | `{}` | Resource requests and limits of the operator container |
| resync | string | `"15s"` | Time between two passes of the operator over the TenantNetwork resources |
| vlanParentInterface | string | `""` | Only for a realm that places subnets on VLAN segments: default name of the interface of the nodes that carries the VLAN. An empty value means the interface with the underlay address of the node. |

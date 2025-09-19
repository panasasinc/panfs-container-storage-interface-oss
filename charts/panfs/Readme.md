
# PanFS CSI Driver Helm Chart

A Helm chart for deploying VDURA PanFS CSI controller, node components, and KMM module

![Version: 1.2.2](https://img.shields.io/badge/Version-1.2.2-informational?style=flat-square) ![AppVersion: 1.2.2](https://img.shields.io/badge/AppVersion-1.2.2-informational?style=flat-square)

## Prerequisites

- Kubernetes 1.20+
- Helm 3.8+
- Access to the container images specified in the `values.yaml` file
- RBAC enabled in the Kubernetes cluster

## Components

| Compatible with CSI Version  | Container Image | [Min K8s Version](https://kubernetes-csi.github.io/docs/kubernetes-compatibility.html#minimum-version) | [Recommended K8s Version](https://kubernetes-csi.github.io/docs/kubernetes-compatibility.html#recommended-version) |
|---|---|---|---|
| [CSI Spec v1.9.0](https://github.com/container-storage-interface/spec/releases/tag/v1.9.0) | [registry.k8s.io/sig-storage/csi-provisioner:v5.3.0](https://github.com/kubernetes-csi/external-provisioner) | 1.20 | 1.31 |
| [CSI Spec v1.10.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0) | [k8s.gcr.io/sig-storage/csi-resizer:v1.13.2](https://github.com/kubernetes-csi/external-resizer) | 1.16 | 1.32 |
| [CSI Spec v1.5.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0) | [registry.k8s.io/sig-storage/csi-attacher:v4.9.0](https://github.com/kubernetes-csi/external-attacher) | 1.17 | 1.22 |
| [CSI Spec v1.5.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0) | [registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.14.0](https://github.com/kubernetes-csi/node-driver-registrar) | 1.13 | 1.23.10 |

> **NOTE**: According to upstream documentation

## Installation

### Clone the Repository

```bash
git clone <repository-url>
cd panfs-container-storage-interface
```

### Helm Commands

Below are example Helm commands to manage the CSI PanFS chart.

#### 1. Template the Chart
Render the chart templates locally to inspect the generated Kubernetes manifests without applying them:

```bash
helm template charts/panfs --namespace csi-panfs
helm template charts/panfs --namespace csi-panfs --output-dir ./output
```

This command generates the manifests in terminal or in the `./output` directory for review.

#### 2. Install / Upgrade the Chart
Install / Upgrade the chart into the `csi-panfs` namespace:

```bash
helm upgrade --install csi-panfs charts/panfs --namespace csi-panfs --create-namespace
```

This installs or updates the deployment while preserving the existing release.

#### 3. Upgrade with Overriding Parameters
Override specific values during an upgrade, for example, changing the replica count or image tag:

```bash
helm upgrade csi-panfs charts/panfs --namespace csi-panfs \
  --set images.panfsPlugin.image=us-central1-docker.pkg.dev/labvirtualization/vdura-csi/csi-plugin@sha256:c3e9fe6257975a1ff984b7f43ea8cd3ef2d2bdfef19341d99af5c0bc6c798c6b
```

This updates the release with a specific image tag for the PanFS plugin.

#### 4. Check Release Status
View the status of the deployed release:

```bash
helm status csi-panfs --namespace csi-panfs
```

This displays details about the release, including the deployed resources and their status.

#### 5. Uninstall the Chart
Remove the CSI PanFS deployment from the cluster:

```bash
helm uninstall csi-panfs --namespace csi-panfs
```

This deletes all resources associated with the release.

## Configuration

The `values.yaml` file contains configurable parameters.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| controllerServer.affinity | object | `{...}` | Affinity rules for controller pods |
| controllerServer.attacher.image | string | `"k8s.gcr.io/sig-storage/csi-attacher:v4.9.0"` | CSI attacher image |
| controllerServer.attacher.logLevel | int | `5` | Log level for attacher |
| controllerServer.attacher.pullPolicy | string | `"IfNotPresent"` | Image pull policy for attacher |
| controllerServer.attacher.resources | object | `{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"100m","memory":"100Mi"}}` | Resource requests and limits for attacher |
| controllerServer.attacher.timeout | string | `"60s"` | Timeout for attacher operations |
| controllerServer.hostNetwork | bool | `true` | Enable host networking for controller pods |
| controllerServer.podDisruptionBudget | object | `{"minAvailable":1}` | PodDisruptionBudget for controller server |
| controllerServer.podDisruptionBudget.minAvailable | int | `1` | Minimum number of available pods for controller |
| controllerServer.priorityClassName | string | `"system-cluster-critical"` | Priority class for controller pods |
| controllerServer.provisioner.image | string | `"k8s.gcr.io/sig-storage/csi-provisioner:v5.3.0"` | CSI provisioner image |
| controllerServer.provisioner.logLevel | int | `5` | Log level for provisioner |
| controllerServer.provisioner.pullPolicy | string | `"IfNotPresent"` | Image pull policy for provisioner |
| controllerServer.provisioner.resources | object | `{"limits":{"cpu":"300m","memory":"300Mi"},"requests":{"cpu":"100m","memory":"100Mi"}}` | Resource requests and limits for provisioner |
| controllerServer.provisioner.retryIntervalStart | string | `"5s"` | Retry interval start for provisioner |
| controllerServer.provisioner.timeout | string | `"60s"` | Timeout for provisioner operations |
| controllerServer.provisioner.workerThreads | int | `5` | Number of worker threads for provisioner |
| controllerServer.replicaCount | int | `3` | Number of controller replicas |
| controllerServer.resizer.image | string | `"gcr.io/k8s-staging-sig-storage/csi-resizer:v1.13.2"` | CSI resizer image |
| controllerServer.resizer.logLevel | int | `5` | Log level for resizer |
| controllerServer.resizer.pullPolicy | string | `"IfNotPresent"` | Image pull policy for resizer |
| controllerServer.resizer.resources | object | `{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"100m","memory":"100Mi"}}` | Resource requests and limits for resizer |
| controllerServer.resizer.timeout | string | `"60s"` | Timeout for resizer operations |
| controllerServer.strategy | object | `{...}` | Deployment strategy type |
| controllerServer.tolerations | list | `[...]` | Tolerations for controller pods |
| csiDriver.fsGroupPolicy | string | `"File"` | Specifies the policy for fsGroup handling |
| csiDriver.requiresRepublish | bool | `false` | Indicates if the driver requires NodePublishVolume to be periodically called for already published volumes |
| csiDriver.seLinuxMount | bool | `true` | Enables SELinux mount support for the CSI driver |
| dfcRelease.kernelMappings | list | `[]` | **PanFS DFC images** for different kernel versions |
| dfcRelease.pullPolicy | string | `"Always"` | Image pull policy for the DFC binary |
| imagePullSecrets | list | `[]` | List of image pull secrets for private registries |
| labels | object | `{}` | Labels for the CSI driver workloads |
| nodeServer.driverRegistrar.image | string | `"k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.5.0"` | CSI node driver registrar image |
| nodeServer.driverRegistrar.logLevel | int | `5` | Log level for driver registrar |
| nodeServer.driverRegistrar.pullPolicy | string | `"IfNotPresent"` | Image pull policy for driver registrar |
| nodeServer.driverRegistrar.resources | object | `{"limits":{"cpu":"100m","memory":"100Mi"},"requests":{"cpu":"100m","memory":"100Mi"}}` | Resource requests and limits for driver registrar |
| nodeServer.driverRegistrar.timeout | string | `"60s"` | Timeout for driver registrar operations |
| nodeServer.priorityClassName | string | `"system-cluster-critical"` | Priority class for node pods |
| nodeServer.selector | object | `{"node-role.kubernetes.io/worker":""}` | Node selector for node pods |
| nodeServer.tolerations | list | `[...]` | Tolerations for node pods |
| nodeServer.updateStrategy.rollingUpdate.maxUnavailable | string | `"100%"` |  |
| nodeServer.updateStrategy.type | string | `"RollingUpdate"` |  |
| panfsKmmModule.enabled | bool | `true` | Enable or disable KMM module for PanFS |
| panfsKmmModule.kmmNodeReadyLabel | object | `{"kmm.node.kubernetes.io/<csi-driver-namespace>.<module-name>.ready": ""}` | Label applied to nodes when the PanFS kernel module is successfully loaded |
| panfsKmmModule.pullPolicy | string | `"Always"` | Image pull policy for the KMM module |
| panfsKmmModule.selector | object | `{"node-role.kubernetes.io/worker":""}` | Node selector for node pods |
| panfsPlugin.image | string | `...` | Image for the PanFS CSI plugin |
| panfsPlugin.logLevel | int | `5` | Log level for the PanFS CSI plugin |
| panfsPlugin.pullPolicy | string | `"Always"` | Image pull policy for the PanFS CSI plugin |
| panfsPlugin.resources | object | `{"limits":{"cpu":"300m","memory":"600Mi"},"requests":{"cpu":"100m","memory":"200Mi"}}` | Resource requests and limits for the PanFS CSI plugin |
| panfsPlugin.seLinuxOptions | object | `{"level":"s0","role":"system_r","type":"container_t","user":"system_u"}` | Security options for the PanFS CSI plugin |
| productName | string | `"com.vdura.csi.panfs"` | Product name |
| seLinux | bool | `true` |  |

> **NOTE:** Please refer to the `values.yaml` file for a complete list of configurable parameters.

To customize, create a `custom-values.yaml` file and apply it during install or upgrade:

```bash
helm install csi-panfs csi-panfs --namespace csi-panfs -f custom-values.yaml
```

## Notes

- Ensure the `csi-panfs` namespace exists or use `--create-namespace` during installation.
- The PanFS CSI plugin requires privileged access for the node DaemonSet. Verify your cluster's security policies allow this.

## Troubleshooting

- Run sanity tests to verify the installation:
  ```bash
  helm test csi-panfs --namespace csi-panfs --logs
  ```
- Check pod logs for errors:
  ```bash
  kubectl logs -l app=csi-panfs-controller -n csi-panfs
  kubectl logs -l app=csi-panfs-node -n csi-panfs
  ```
- Verify RBAC permissions are correctly applied:
  ```bash
  kubectl get clusterrole,clusterrolebinding,role,rolebinding -n csi-panfs
  ```
- Ensure the PanFS CSI plugin image is accessible from your cluster.

For further assistance, refer to the [Kubernetes CSI documentation](https://kubernetes-csi.github.io/docs/) or contact the chart maintainers.

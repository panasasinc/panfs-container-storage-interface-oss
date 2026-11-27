# Changelog

All notable changes to the PanFS CSI Driver are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The entry of the next release is marked `Unreleased`.
The [Create Release](.github/workflows/create-release.yaml) workflow replaces that word with the release date,
and it stops when the release has no entry in this file.

This file covers every release published in this repository, starting with 1.2.1.
Versions 1.0.0 to 1.2.0 were released before the driver became open source;
the [compatibility matrix](README.md#compatibility) lists them, but this repository has no history for them.

## [2.0.0] - Unreleased

### Changed

- **Breaking:** the driver manages volumes through PanAPI, the REST API of the PanFS realm, with a service account token.
  It no longer sends `pancli` commands over SSH. The realm must provide PanAPI.
- **Breaking:** the StorageClass Secret has the keys `api` and `token`, and an optional `ca_cert`.
  The keys of the SSH backend are not read, and a Secret without the new keys is refused.
  Create the Secret and the StorageClass again; see the [upgrade guide](docs/Upgrade.md#migration-from-the-ssh-backend-to-panapi).
- **Breaking:** the chart in `charts/storageclass` (now `panfs-storageclass`, was `csi-panfs-storageclass`) creates
  a `PanfsStorageClass` resource, and the PanFS Tenant operator creates the StorageClass from it. The chart no longer
  creates the StorageClass, the Secret, the `Role` and the `RoleBinding` itself, and its values changed: the `realm.*`,
  `csiPanFSDriver.namespace`, `allowedTopologies`, `setAsDefaultStorageClass` and `volumeReclaimPolicy` values are gone.
  The Secret comes from the `TenantNetwork` of the namespace (chart `charts/tenant`) or, with `realmSecretName`,
  from a Secret that you create. A StorageClass of an earlier version keeps working; see
  [Moving a StorageClass of the old StorageClass chart](charts/panfs-tenant-operator/Readme.md#moving-a-storageclass-of-the-old-storageclass-chart).
- **Breaking:** the `Makefile` variables `REALM_API`, `REALM_TOKEN`, `REALM_CA_CERT` and `STORAGESET`, and the inputs
  `realm_api`, `realm_token` and `storageset` of the deploy workflow, replace the SSH ones.
  `make deploy-storageclass` creates the realm Secret and a `PanfsStorageClass`, so it needs `make deploy-tenant-operator` first.
- The CSI volume ID of a new volume is the PanAPI volume id. The volume name goes to the node in the volume context.
  A PersistentVolume of an earlier version, whose volume ID is the volume name, still works.
- `panfs.csi.vdura.com/storageset` is optional. A number is the id of the storageset and other text is its name.
  Without the parameter, the driver uses the first storageset that is assigned to the tenant of the token.
- `panfs.csi.vdura.com/user` and `panfs.csi.vdura.com/group` accept numeric ids only.
- `kmm.encryptionSupport` defaults to `false` in `charts/panfs`.
- **Breaking:** the driver reports topology (the plugin capability `VOLUME_ACCESSIBILITY_CONSTRAINTS`).
  A StorageClass with `volumeBindingMode: WaitForFirstConsumer` and with `allowedTopologies` on a label that the
  CSI node plugin does not report, for example the ready label of the tenant network, no longer provisions volumes.
  Use a `PanfsStorageClass`, or `volumeBindingMode: Immediate`. A StorageClass without `allowedTopologies`,
  or with the label `node.kubernetes.io/csi-driver.panfs.ready`, works as before.
- The manifest check in CI, and so `make manifests`, uses Helm v4.3.2 (was v3.22.1).

### Added

- PanFS Tenant operator: the new chart `charts/panfs-tenant-operator`, the manifest
  `deploy/k8s/tenant-operator/panfs-tenant-operator.yaml`, and the image `panfs-tenant-operator`.
  Install it once per cluster, in the namespace of the driver. It is one workload that serves two new resources
  of the group `panfs.vdura.com/v1alpha1`. See the [operator documentation](charts/panfs-tenant-operator/Readme.md).
  - `TenantNetwork` names the Secret of a tenant and selects its nodes; the operator reads the tenant of the token
    from PanAPI and runs a pod on every selected node. A pod attaches its node to the subnet of the tenant through
    PanAPI, builds the tenant interface, sets the firewalld zone, and sets the node label
    `panfs.vdura.com/tenant-network=<namespace>` while a director of the realm answers. When a node no longer
    matches the selector, or the `TenantNetwork` is deleted, the pod removes the interface and detaches the node.
    A ready label that the pods could not remove, for example because the whole namespace was deleted, is removed
    by the operator. The pod builds a VXLAN or a VLAN interface, as the realm reports the segment type of the subnet.
    The status of the resource lists the nodes, and the operator and the pods report changes as events on it.
    The template is `deploy/k8s/tenant-operator/template-tenantnetwork.yaml`.
  - `PanfsStorageClass` asks for a StorageClass. From a name alone, in the namespace of a `TenantNetwork`,
    the operator creates the StorageClass: it finds the `TenantNetwork`, fills in the provisioner, the Secret
    references and `allowedTopologies`, and it lets the CSI controller read the Secret.
    `spec.realmSecretName` is for a realm without a tenant network; such a cluster installs the same operator.
    The template is `deploy/k8s/storage-class/template-panfsstorageclass.yaml`.
- PersistentVolume node affinity: the controller returns the `allowedTopologies` of the StorageClass as the
  topology of a new volume. Every pod of a volume of a tenant is placed on a node where the tenant network works,
  without a node selector in the workload.
- PanFS Tenant chart (`charts/tenant`): one Helm release per tenant with the Secret of the realm connection
  and the `TenantNetwork`. It checks that the PanFS Tenant operator is installed before it creates anything.
- The `Makefile` targets `build-tenant-operator-image`, `deploy-tenant-operator`, `uninstall-tenant-operator`
  and `manifest-tenant-operator`, and the variable `TENANT_OPERATOR_IMAGE`.
- The GitHub workflows build, scan, release and clean up the image `panfs-tenant-operator` next to the driver image,
  and the deploy workflow installs the operator before it creates the StorageClass.
- The controller reads the mount address (`rmlist`) of the tenant network from PanAPI and passes it to the node.
  The node mounts `panfs://<first address>/<volume>` with the mount option `rmlist=(...)`.
  Without a mount address, it mounts from the host of the `api` address.
- Static provisioning by name: `volumeHandle` of a PersistentVolume is the name of a volume, or a path inside it.
  The optional volume attribute `panfs.csi.vdura.com/rmlist` sets its mount address.
- The optional Secret key `insecure_skip_tls_verify` (chart value `realm.insecureSkipTLSVerify`, `Makefile` variable
  `REALM_INSECURE_SKIP_TLS_VERIFY`) turns the verification of the realm certificate off, for test realms.
- The token permissions and the network requirements are documented in the [README](README.md#panfs-realm-requirements).

### Removed

- **Breaking:** volume encryption. The parameter `panfs.csi.vdura.com/encryption`, the Secret key `kmip_config_data`
  and the encryption guide are removed, because PanAPI has no encryption setting for a volume.
- The parameters `panfs.csi.vdura.com/description` and `panfs.csi.vdura.com/recovery`. The driver rejects them;
  use `panfs.csi.vdura.com/recoverypriority` in place of `recovery`.
- The SSH client of the driver, and `openssh-client` in the sanity test image.
- **Breaking:** the plain StorageClass manifests `deploy/k8s/storage-class/example-csi-panfs-storage-class.yaml`,
  `template-secret-in-dedicated-ns.yaml` and `template-secret-in-driver-ns.yaml`, and the `Makefile` target
  `manifest-storageclass`. Use `deploy/k8s/storage-class/template-panfsstorageclass.yaml`.

## [1.2.6] - 2026-11-18

### Changed

- The driver is built with Go 1.27 (was Go 1.25).
- The driver image and the stub DFC image are based on Alpine 3.24 (was 3.22).
- All four CSI sidecars are pulled from `registry.k8s.io` instead of `k8s.gcr.io` and `gcr.io/k8s-staging-sig-storage`.
  If your cluster mirrors images or restricts registries, allow `registry.k8s.io` before you upgrade.
- `csi-attacher` v4.9.0 to v4.13.0, `csi-resizer` v1.13.2 to v1.14.0, `csi-node-driver-registrar` v2.5.0 to v2.18.0.
  `csi-provisioner` stays at v5.3.0, because v6 requires Kubernetes 1.34.
- CSI specification v1.11.0 to v1.14.0.
- All direct Go dependencies are updated; the Kubernetes modules are aligned on v0.37.2.
- The manifests in `deploy/k8s` and `examples/k8s` use the 1.2.6 driver image (they were left at 1.2.3 and 1.2.2).
- The default `CSI_IMAGE` in the `Makefile` uses the published image name `panfs-csi-driver`.
- GitHub Actions are updated, and `aquasecurity/trivy-action` is pinned to a release instead of the `master` branch.

- A driver image that is built without `APP_VERSION` reports the version `dev`, not a release number.
  Release builds and `make build-driver-image` always pass the version.
- `make manifests` passes the chart readme file names (`Readme.md`, `Readme.md.gotmpl`) to `helm-docs`,
  so it works on a case-sensitive file system. CI regenerates the manifests and fails when the result differs.

### Fixed

- `NodePublishVolume` rejects a volume with the `SINGLE_NODE_SINGLE_WRITER` access mode that is already published
  at another target path on the node (`FailedPrecondition`). Before, the second publish succeeded.
- The removal of the node label during shutdown is limited to 10 seconds, so a slow API server cannot block the exit.

### Removed

- `deploy/k8s/csi-driver/template-csi-panfs-with-e2ee.yaml`. It was the same as the standard template:
  encryption support is enabled by default, and the file was generated with `dfc.encryptionSupport`,
  which is not a chart value. Use `template-csi-panfs.yaml`.
- The unused `internal/jsonlog` package.
- The Go Report Card badge in `README.md`; the service is retired.

### Added

- `SECURITY.md`, `CHANGELOG.md`, `.dockerignore` and a golangci-lint configuration (`.golangci.yml`).
- Unit tests for `pkg/utils`.
- `make check-version` checks that the charts, the manifests, the `Makefile` and the documentation use one driver version.
  CI runs it on every push, and the Create Release workflow runs it first and stops on a mismatch.
- The Create Release workflow no longer asks for a version: it releases the `appVersion` of `charts/panfs/Chart.yaml`.
  Set the next version in a pull request (charts, `make manifests`, `Makefile`, changelog, compatibility matrix) first.

## [1.2.5] - 2026-07-07

### Fixed

- The KMM `Module` listed the kernel module `wolfssl` in `modulesLoadingOrder`, but the module name is `libwolfssl`.
  Loading worked, but unloading failed with `Module wolfssl not found` and blocked KMM on that node,
  for example after a short `NotReady` period. The chart and all manifests in `deploy/k8s` now use `libwolfssl` (#10).
- Typos, broken links and wrong comments in the documentation and in the manifests (#12).

### Security

- `golang.org/x/crypto` updated to v0.52.0 and `golang.org/x/net` to v0.55.0 (#11).

### Changed

- The Trivy scan in CI uses a pinned `aquasecurity/trivy-action` release (#13, #14).
- A `Create Release` workflow sets the chart versions, creates the tag and publishes the GitHub release (#15).
- Helm chart versions set to 1.2.5 (#16).

## [1.2.4] - 2026-05-25

### Changed

- **Breaking:** the StorageClass parameter `panfs.csi.vdura.com/bladeset` is renamed to
  `panfs.csi.vdura.com/storageset`, to follow the current PanFS vocabulary (#9).
  The driver sends `storageset "<name>"` to the realm and reads `<storagesetName>` from its output.
  Update every StorageClass that uses the old key; a StorageClass is immutable, so delete it and create it again.
  The realm must run a PanFS release that accepts the `storageset` keyword.
- The `Makefile` is reworked: clearer `make help`, documented environment variables,
  and status messages for the build, deploy and uninstall targets (#8).

### Added

- A guide to end-to-end encryption with the driver: `docs/Encryption.md` (removed later, with volume encryption).

## [1.2.3] - 2025-12-23

### Added

- Encrypted volumes (#7). The StorageClass parameter `panfs.csi.vdura.com/encryption` creates an encrypted PanFS volume,
  and the node plugin mounts it with a KMIP configuration that it takes from the StorageClass secret (`kmip_config_data`).
- KMM loads the encryption kernel module with `panfs` when `kmm.encryptionSupport` is enabled in the chart.
- The driver logs the SSH commands that it sends to the realm, and reports realm errors more precisely.

### Changed

- The driver image is built with the full Go image and runs on Alpine.

### Fixed

- The driver sends the encryption option to the realm only when the parameter is `on`.
  A StorageClass without the parameter, or with another value, still works with realms that do not support encryption.

## [1.2.2] - 2025-10-06

### Added

- The node plugin sets the node label `node.kubernetes.io/csi-driver.panfs.ready` when the driver is ready on a node,
  and removes it on shutdown. Use the label as a `nodeSelector` for workloads that mount PanFS volumes (#4).
- The node DaemonSet runs only on nodes where KMM has loaded the PanFS kernel module (#1).
- The `storageclass` chart can create the realm secret in the driver namespace or in a dedicated namespace,
  and supports mount options and allowed topologies (#5).
- Labels and annotations on the driver components, and a Role and RoleBinding that let the controller read
  the StorageClass secret (#4, #5).
- Manifests in `deploy/k8s` for the common deployment variants (with and without KMM, with and without SELinux).
- The pull request workflow deploys the driver to a kind cluster (#2).

### Changed

- Installation documentation and examples updated for the node label and the new StorageClass options.

## [1.2.1] - 2025-09-18

First open source release of the PanFS CSI Driver.

### Added

- The CSI driver: identity, controller and node services, with dynamic provisioning and volume expansion.
- Helm charts for the driver (`charts/panfs`) and for the StorageClass with its secret (`charts/storageclass`),
  and plain Kubernetes manifests in `deploy/k8s`.
- Kernel Module Management (KMM) integration to load the PanFS DirectFlow kernel module on worker nodes.
- Example workloads in `examples/k8s`.
- csi-sanity and Kubernetes end-to-end test configuration, unit tests, and GitHub Actions workflows
  for build, test, vulnerability scan and release.
- Documentation: overview, usage, upgrade, SELinux, KMM, troubleshooting, and a script that collects diagnostics.

[1.2.6]: https://github.com/panasasinc/panfs-container-storage-interface-oss/compare/1.2.5...1.2.6
[1.2.5]: https://github.com/panasasinc/panfs-container-storage-interface-oss/compare/1.2.4...1.2.5
[1.2.4]: https://github.com/panasasinc/panfs-container-storage-interface-oss/compare/1.2.3...1.2.4
[1.2.3]: https://github.com/panasasinc/panfs-container-storage-interface-oss/compare/1.2.2...1.2.3
[1.2.2]: https://github.com/panasasinc/panfs-container-storage-interface-oss/compare/1.2.1...1.2.2
[1.2.1]: https://github.com/panasasinc/panfs-container-storage-interface-oss/releases/tag/1.2.1

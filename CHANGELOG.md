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

- A guide to end-to-end encryption with the driver: [docs/Encryption.md](docs/Encryption.md).

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

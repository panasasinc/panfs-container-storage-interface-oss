# Changelog

All notable changes to the PanFS CSI Driver are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

For releases before 1.2.6, see the
[GitHub Releases](https://github.com/panasasinc/panfs-container-storage-interface-oss/releases) page.

## [1.2.6] - Unreleased

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

### Fixed

- `make manifest-driver` set `dfc.encryptionSupport`, which is not a chart value; it now sets `kmm.encryptionSupport`.
- The removal of the node label during shutdown is limited to 10 seconds, so a slow API server cannot block the exit.

### Removed

- The unused `internal/jsonlog` package.

### Added

- `SECURITY.md`, `CHANGELOG.md`, `.dockerignore` and a golangci-lint configuration (`.golangci.yml`).
- Unit tests for `pkg/utils`.

[1.2.6]: https://github.com/panasasinc/panfs-container-storage-interface-oss/compare/1.2.5...1.2.6

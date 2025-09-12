
# Contributing Guidelines

## Prerequisites

- If you are new to CSI, please review the [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) and [CSI driver development guide](https://kubernetes-csi.github.io/docs/developing.html).

## Contributing a Change

1. Fork the repository, develop and test your changes.
2. Ensure your changes include tests and documentation as needed.
3. Submit a pull request with a clear description and reference any related issues.
4. Respond to reviewer feedback and make requested changes.
5. After merge, your change will be included in a release.

## Development Quickstart Guide

### 1. Local Development

- Run `make build-driver-image` to build the driver and check for compiler/syntax errors.
- Run `make sanity-check` to execute unit tests. Add or update tests for new features or bugfixes.

### 2. Cluster Setup

- Set up a Kubernetes cluster for manual testing.
- Build and install the driver image using provided Makefile targets.
- Uninstall the driver after testing.

### 3. Automated E2E Tests

- Create a cluster and build the driver image.
- Run E2E tests using `make e2e`.
- Clean up your cluster after testing.

### 4. Before Submitting a PR

- Format code, check with linters and code checks.
- Ensure your PR includes tests and documentation.
- Provide evidence of manual testing for features not covered by automation.

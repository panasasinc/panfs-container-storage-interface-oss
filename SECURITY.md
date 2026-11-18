# Security Policy

## Supported Versions

Security fixes are released for the latest version of the PanFS CSI Driver only.
Upgrade to the [latest release](https://github.com/panasasinc/panfs-container-storage-interface-oss/releases/latest)
before you report a problem.

## Reporting a Vulnerability

Do not open a public GitHub issue for a security vulnerability.

Report it privately through GitHub:
open the [Security tab](https://github.com/panasasinc/panfs-container-storage-interface-oss/security)
of this repository and select **Report a vulnerability**.

Include in the report:

- the driver version, the Kubernetes version and the PanFS version
- a description of the vulnerability and its impact
- the steps to reproduce it, or a proof of concept

The maintainers confirm that they received the report, tell you whether they accept it,
and keep you informed until a fix is released. Do not disclose the vulnerability in public before the fix is released.

## How This Project Finds Vulnerabilities

- [Trivy](https://github.com/aquasecurity/trivy) scans the source tree and the driver image on every pull request
  and on a schedule (see the [Vulnerability Scan workflow](.github/workflows/vulnerability.yaml)).
- [Dependabot](.github/dependabot.yml) opens pull requests for Go modules, GitHub Actions and base images.

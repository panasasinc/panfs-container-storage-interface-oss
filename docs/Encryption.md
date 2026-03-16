# PanFS CSI Driver Volume Encryption Guide

This document explains how to configure and enable End-to-End Encryption (E2EE) support for volumes managed by the PanFS CSI driver.

## Prerequisites

To use volume encryption, you must meet the following requirements:
1. PanFS CSI driver must be installed with encryption support enabled.
2. A KMIP server must be available for key management.
3. The underlying PanFS storage must support encryption (volumes will be encrypted using `aes-xts-256` by default when enabled).

## 1. Enable Encryption in CSI Driver

When installing or upgrading the PanFS CSI driver via Helm, encryption support should be enabled. This allows the driver to load necessary encryption modules (e.g. `wolfssl`) on the worker nodes.

Encryption support is enabled by default via the `kmm.encryptionSupport` parameter. To explicitly ensure it is enabled during installation:

```bash
helm upgrade --install csi-panfs charts/panfs \
    --namespace csi-panfs \
    --set kmm.encryptionSupport=true
```

## 2. Provide KMIP Configuration

For the CSI driver to manage encrypted volumes, it needs to communicate with a KMIP server. The KMIP configuration data must be provided as a string in the Kubernetes Secret associated with your StorageClass.

### `kmip_config_data` Format

The `kmip_config_data` must be provided in an INI-style configuration format, which contains the server details and Base64-encoded certificates required for mutual TLS authentication.

```ini
config_version = 1

[global]
primary-server = server:dev

[server:dev]
host = <KMIP_HOST>
port = <KMIP_PORT>
version = 1.4
connect-timeout = 300
reconnect-retries = 5

ca-cert-data = <Base64-encoded CA Certificate>
client-cert-data = <Base64-encoded Client Certificate>
client-key-data = <Base64-encoded Client Key>
```

- **`host` / `port`**: Network address and port of your KMIP server.
- **`ca-cert-data`**: Base64-encoded contents of the CA certificate.
- **`client-cert-data`**: Base64-encoded contents of the client certificate.
- **`client-key-data`**: Base64-encoded contents of the client key.

### Configuration Examples

In your StorageClass Secret, populate the `kmip_config_data` field:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-panfs-storage-class
  namespace: csi-panfs
type: Opaque
stringData:
  realm_ip: panfs-realm.example.com
  user: admin
  password: "password"
  # KMIP server connection details for volume encryption
  kmip_config_data: |
    config_version = 1

    [global]
    primary-server = server:dev

    [server:dev]
    host = kmip.example.com
    port = 5696
    version = 1.4
    connect-timeout = 300
    reconnect-retries = 5

    ca-cert-data = LS0tLS1CRU...
    client-cert-data = LS0tLS1CRU...
    client-key-data = LS0tLS1CRU...
```

If you are using the `csi-panfs-storageclass` Helm chart to deploy your StorageClass, you can set this via Helm values:

```yaml
# custom-values.yaml
realm:
  address: "panfs-realm.example.com"
  username: "admin"
  password: "password"
  kmipConfigData: |
    config_version = 1

    [global]
    primary-server = server:dev

    [server:dev]
    host = kmip.example.com
    port = 5696
    version = 1.4
    connect-timeout = 300
    reconnect-retries = 5

    ca-cert-data = LS0tLS1CRU...
    client-cert-data = LS0tLS1CRU...
    client-key-data = LS0tLS1CRU...
```

> **Note:** If `kmip_config_data` is missing or empty and you attempt to provision an encrypted volume, the volume mounting will fail with the error: `KMIP secret must be provided for encrypted volumes`.

## 3. Enable Encryption in StorageClass

To provision encrypted volumes, you must set the `panfs.csi.vdura.com/encryption` parameter to `"on"` in your StorageClass `parameters`.

Example StorageClass:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-panfs-encrypted
provisioner: com.vdura.csi.panfs
allowVolumeExpansion: true
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
parameters:
  csi.storage.k8s.io/provisioner-secret-name:            csi-panfs-storage-class  # secret name with KMIM configuration
  csi.storage.k8s.io/provisioner-secret-namespace:       csi-panfs-storage-class  # namespace with the secret, which contains "kmip_config_data"
  csi.storage.k8s.io/node-publish-secret-name:           csi-panfs-storage-class
  csi.storage.k8s.io/node-publish-secret-namespace:      csi-panfs-storage-class
  csi.storage.k8s.io/controller-expand-secret-name:      csi-panfs-storage-class
  csi.storage.k8s.io/controller-expand-secret-namespace: csi-panfs-storage-class

  # Enable volume encryption
  panfs.csi.vdura.com/encryption: "on"
```

Valid values for the `panfs.csi.vdura.com/encryption` parameter are:
- `"on"`: Enables volume encryption.
- `"off"`: Disables volume encryption.

If the parameter is omitted, encryption is disabled by default for backward compatibility.

## Verification

Once you have set up the StorageClass with encryption enabled, any PersistentVolumeClaim (PVC) created using this StorageClass will automatically provision an encrypted volume on the PanFS backend. The CSI driver handles the encryption and decryption transparently during volume publish (mount) on the Kubernetes nodes utilizing the provided KMIP configuration.
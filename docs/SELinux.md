<!-- 
  Copyright 2025 VDURA Inc.

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
-->

# SELinux Configuration Reference for Kubernetes CSI Drivers

This document is a technical reference for configuring SELinux in Kubernetes environments using CSI (Container Storage Interface) drivers. It describes configuration options and example settings for operating system, container runtime, Kubernetes cluster, CSI driver, and application layers to enable SELinux integration with persistent volumes.

## Table of Contents

1. [Overview](#1-overview)
2. [SELinux Configuration Requirements](#2-selinux-configuration-requirements)
  - [Operating System Configuration](#21-operating-system-configuration)
  - [Container Runtime Configuration](#22-container-runtime-configuration)
  - [Kubernetes Cluster Configuration](#23-kubernetes-cluster-configuration)
  - [CSI Driver and Storage Configuration](#24-csi-driver-and-storage-configuration)
  - [Application and Workload Configuration](#25-application-and-workload-configuration)

## 1. Overview

SELinux (Security-Enhanced Linux) provides mandatory access control (MAC) that enhances security in Kubernetes clusters by enforcing fine-grained access policies. When implementing CSI drivers with SELinux, proper configuration is required across multiple system layers to ensure seamless volume mounting and access.

This guide is designed for:
- **System administrators** configuring SELinux-enabled nodes
- **Kubernetes administrators** setting up clusters with SELinux support  
- **CSI driver operators** configuring storage classes and driver manifests
- **Application developers** creating SELinux-aware pod specifications

> **Important Notice**: The PanFS CSI Driver's role in SELinux is limited to passing through SELinux contexts (`context`, `defcontext`, etc.) to the underlying mount tool. The CSI Driver does **not** perform any SELinux label management, relabeling, or context enforcement on its own. All SELinux-related functionality, performance characteristics, networking behavior, and security enforcement are handled entirely by:
> - **Kubernetes** (kubelet, container runtime)
> - **Operating System** (SELinux policy engine, kernel)
> - **Mount utilities** (mount command with SELinux mount options - scope limited to kernel-loaded SELinux modules and policies)

**This document focuses on enabling SELinux capabilities in Kubernetes environments, not on CSI Driver-specific tuning.** Any performance degradation, networking issues, or SELinux-related problems should be investigated at the Kubernetes and OS levels, not within the CSI Driver itself.

## 2. SELinux Configuration Requirements

### 2.1 Operating System Configuration

#### Base SELinux Setup
- Configure **Enforcing Mode** and **Policy Type**:
  ```bash
  # Verify current status
  sestatus
  
  # Set enforcing mode permanently (requires reboot for permanent effect)
  echo "SELINUX=enforcing" > /etc/selinux/config
  echo "SELINUXTYPE=targeted" >> /etc/selinux/config
  
  # Apply immediately
  setenforce 1
  ```

- **Required Packages**: Install container-specific SELinux policies
  ```bash
  # RHEL/CentOS/Fedora
  dnf install -y container-selinux
  ```

#### Advanced Policy Configuration
- **Custom Policy Modules**: Develop application-specific policies for enhanced security
- **Booleans Configuration**: Configure SELinux booleans based on your specific requirements
  ```bash
  # Required for container cgroup management (essential for Kubernetes)
  setsebool -P container_manage_cgroup 1
  
  # Only required for specific networking features (usually not needed)
  # setsebool -P virt_sandbox_use_netlink 1
  ```

- **File Context Management**: Verify proper labeling of Kubernetes directories
  ```bash
  # Verify contexts are correct (should show container_var_lib_t and container_file_t)
  ls -Z /var/lib/kubelet/
  ls -Z /var/lib/containerd/
  
  # Only restore contexts if they appear incorrect
  # restorecon -R /var/lib/kubelet /var/lib/containerd
  ```

### 2.2 Container Runtime Configuration

#### containerd Configuration
```toml
# /etc/containerd/config.toml
version = 2

[plugins."io.containerd.grpc.v1.cri"]
  enable_selinux = true
  selinux_category_range = 1024
  
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
  # Enable SELinux support in runc
  SELinux = true
```

#### CRI-O Configuration
```toml
# /etc/crio/crio.conf
[crio.runtime]
selinux = true

[crio.runtime.runtimes.runc]
runtime_path = "/usr/bin/runc"
runtime_type = "oci"
runtime_root = "/run/runc"
```

#### Runtime Security Enhancements
- **MCS (Multi-Category Security)**: Enable category separation between containers
- **Process Separation**: Ensure each container runs with unique SELinux labels
- **Volume Context Inheritance**: Configure proper context propagation for mounted volumes

### 2.3 Kubernetes Cluster Configuration

#### API Server Configuration
```yaml
# kube-apiserver flags
--allow-privileged=true
```

#### Kubelet Configuration
```yaml
# /var/lib/kubelet/config.yaml
kind: KubeletConfiguration
apiVersion: kubelet.config.k8s.io/v1beta1
# ... other configuration ...
featureGates:
  SELinuxMountReadWriteOncePod: true
  SELinuxMount: true
containerRuntimeEndpoint: unix:///var/run/containerd/containerd.sock
```

### 2.4 CSI Driver and Storage Configuration

#### CSI Driver Manifest
```yaml
apiVersion: storage.k8s.io/v1
kind: CSIDriver
metadata:
  name: com.vdura.csi.panfs
spec:
  attachRequired: false
  podInfoOnMount: true
  volumeLifecycleModes:
    - Persistent
  # Enable SELinux mount support
  seLinuxMount: true
  fsGroupPolicy: File
```

#### StorageClass with SELinux
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: selinux-enabled-panfs
provisioner: com.vdura.csi.panfs
parameters:
  # ... other configuration ...

  # SELinux context configuration for mounted volumes
  mountOptions:
  # Set default SELinux context for new files created on the volume
  # This ensures new files inherit container_file_t type for proper container access
  - "defcontext=system_u:object_r:container_file_t:s0"
```

> **Note:** The `container_file_t` SELinux type in the `defcontext` option above is provided as an example. Depending on your storage backend and security requirements, you may need to use a different SELinux type (e.g., `nfs_t`, `svirt_sandbox_file_t`, or a custom type). Always choose the SELinux type that is appropriate for your specific use case and storage configuration.

#### PersistentVolumeClaim with SELinux
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: secure-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: selinux-enabled-panfs
```

### 2.5 Application and Workload Configuration

#### Production Pod Security Context
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: secure-application
  annotations:
    # Kubernetes Pod Security Standards policy annotation indicating restricted security controls
    # This enforces the most restrictive security policy with enhanced isolation
    security.policy: "restricted"
spec:
  securityContext:
    # Pod-level SELinux context
    seLinuxOptions:
      user: "system_u"
      role: "system_r"
      type: "container_t"
      level: "s0:c123"  # Unique MCS label for isolation
  restartPolicy: Never
  containers:
  - name: main
    image: ubuntu:latest
    command:
    - sh
    - -c
    - |
      set -e
      mount | grep /mnt/panfs
      touch /mnt/panfs/main-labels.txt
      echo "SELinux labels applied to the files created in the container:"
      echo "should be 'system_u:object_r:container_file_t:s0:c123'"
      ls -lZ /mnt/panfs | grep 'system_u:object_r:container_file_t:s0:c123'
      echo "SELinux context applied correctly"
    # Container inherits SELinux context from pod-level securityContext
    volumeMounts:
    - name: panfs-volume
      mountPath: /mnt/panfs
  volumes:
  - name: panfs-volume
    persistentVolumeClaim:
      claimName: secure-pvc
```


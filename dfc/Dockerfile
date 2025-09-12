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

# MARK: Stage 1: DFC image
FROM rockylinux/rockylinux:8.10 AS dfc

ARG PANFSPKG_NAME=panfs.rpm
ARG KERNEL_FULL_VERSION

ARG BUILD_DATE
ARG VERSION
ARG GIT_COMMIT

# OCI labels for Open Container Initiative compliance
LABEL org.opencontainers.image.title="PanFS CSI Driver: KMM Module" \
      org.opencontainers.image.description="PanFS CSI Driver: KMM Module for Kubernetes" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.vendor="PanFS CSI Team"

# Copy the panfs RPM package into the image
COPY ${PANFSPKG_NAME} /

# Install the panfs RPM package
RUN yum update -y && \
    yum install -y /${PANFSPKG_NAME} kmod && \
    rm -f /${PANFSPKG_NAME} && \
    yum clean all && \
    rm -rf /var/cache/yum

# Copy the built panfs kernel module from the build stage
RUN mkdir -p /opt/lib/modules/${KERNEL_FULL_VERSION}/ && \
    cp /usr/lib/modules/${KERNEL_FULL_VERSION}/extra/panfs/panfs.ko /opt/lib/modules/${KERNEL_FULL_VERSION}/

# Run depmod to generate module dependency information
RUN depmod -b /opt ${KERNEL_FULL_VERSION}
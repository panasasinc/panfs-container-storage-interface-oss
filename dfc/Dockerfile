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

# MARK: DFC image
FROM rockylinux/rockylinux:8.10 AS dfc

RUN yum install -y cpio findutils tree

ARG PANFSPKG_NAME=panfs.rpm
ARG KERNEL_FULL_VERSION

COPY ${PANFSPKG_NAME} /
RUN rpm2cpio /${PANFSPKG_NAME} | cpio -idmv -D /tmp && \
    rm -f /${PANFSPKG_NAME} && \
    find /tmp/ -name panfs.ko | while read f; do \
        mkdir -p /opt/lib/modules/${KERNEL_FULL_VERSION}; \
        /bin/cp -vf $f /opt/lib/modules/${KERNEL_FULL_VERSION}/; \
        touch /opt/lib/modules/${KERNEL_FULL_VERSION}/modules.{order,builtin,builtin.modinfo}; \
    done

RUN tree /opt

# MARK: DFC/KMM image
FROM alpine:3.22 AS dfc-kmm

LABEL org.opencontainers.image.description="PanFS CSI Driver: DFC/KMM Module for Kubernetes"

# Install kmod
RUN apk update && apk add --no-cache kmod && \
    rm -rf /var/cache/apk/*

COPY --from=dfc /opt/lib/modules/ /opt/lib/modules/
COPY --from=dfc /tmp/sbin/mount.panfs /usr/sbin/mount.panfs

ARG KERNEL_FULL_VERSION
RUN depmod -b /opt $KERNEL_FULL_VERSION
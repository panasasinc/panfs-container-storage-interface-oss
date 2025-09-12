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

# Set the Go version to use (default: 1.24)
ARG GOLANG_VERSION=1.24

FROM golang:${GOLANG_VERSION} AS tests
ARG K8S_VERSION=1.30.13

# Make sure /tests directory exists
RUN mkdir -p /tests

# Install csi-sanity test tool
RUN go install -v github.com/kubernetes-csi/csi-test/v5/cmd/csi-sanity@v5.3.1

# Install required packages for testing
RUN apt-get update && \
  apt-get install -y curl libc6 openssh-client jq libxml2-utils gettext-base && \
  rm -rf /var/lib/apt/lists/*

# Download and install kubectl
RUN curl -LO "https://dl.k8s.io/release/v${K8S_VERSION}/bin/linux/amd64/kubectl" && \
  chmod +x ./kubectl && \
  mv ./kubectl /bin/kubectl

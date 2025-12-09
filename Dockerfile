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

# MARK: Stage 1: Download Go modules for caching
FROM golang:${GOLANG_VERSION} AS modules

# Copy go.mod and go.sum to leverage Docker cache for dependencies
COPY go.mod go.sum /modules/
WORKDIR /modules
RUN go mod download

# MARK: Stage 2: Build the Go binary
FROM golang:${GOLANG_VERSION} AS builder
ARG APP_VERSION="0.2.0"

# Copy downloaded Go modules from previous stage
COPY --from=modules /go/pkg /go/pkg

# Set the working directory
WORKDIR /app

# Copy the application source code
COPY pkg/ pkg/
COPY cmd/ cmd/
COPY go.mod go.sum ./

# Build the binary for Linux amd64, statically linked
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=${APP_VERSION}" -o /bin/panfs-csi ./cmd/csi-plugin/main.go

# MARK: Stage 3: Create the final image
FROM alpine:3.22 AS plugin

ARG BUILD_DATE
ARG VERSION
ARG GIT_COMMIT

# OCI labels for Open Container Initiative compliance
LABEL org.opencontainers.image.title="PanFS CSI Driver" \
      org.opencontainers.image.description="PanFS CSI Driver for Kubernetes" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.vendor="PanFS CSI Team"

RUN apk update && apk upgrade && rm -rf /var/cache/apk/*

# Copy the built panfs-csi binary from builder stage
COPY --from=builder /bin/panfs-csi /panfs-csi

# Set the entrypoint to the panfs-csi binary
ENTRYPOINT ["/panfs-csi"]
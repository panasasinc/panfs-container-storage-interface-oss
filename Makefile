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

# Makefile for PanFS CSI Driver
# This Makefile provides targets for building, deploying, and managing the PanFS CSI Driver and its components.

# Define colors for output
GREEN=\033[32m
RESET=\033[0m
YELLOW=\033[33m
RED=\033[31m
BLUE=\033[36m
GRAY=\033[90m

help:
	@echo Usage:
	@echo "  make <targets>"
	@echo
	@echo 'Targets:'
	@awk '\
		/^## / { \
			sub(/^## /, ""); \
			printf "\n  \033[90m%s\033[0m\n", $$0; \
		} \
		/^[a-zA-Z0-9_-]+:.*##/ { \
			match($$0, /^([a-zA-Z0-9_-]+):.*## (.*)$$/, arr); \
			if (arr[1] && arr[2]) \
				printf "    \033[36m%-40s\033[0m %s\n", arr[1], arr[2]; \
		}' Makefile
	@echo 
	@echo 'Deployment Examples:'
	@echo
	@echo "  $(GRAY)Deploying CSI Driver:$(RESET)"
	@echo "    export CSI_IMAGE=..."
	@echo "    export CSIDFCKMM_IMAGE=..."
	@echo "    $(BLUE)make deploy-driver$(RESET)"
	@echo
	@echo "  $(GRAY)Deploying Storage Class:$(RESET)"
	@echo "    export REALM_ADDRESS=..."
	@echo "    export REALM_USER=..."
	@echo "    export REALM_PASSWORD=..."
	@echo "    $(BLUE)make deploy-storageclass$(RESET)"
	@echo
	@echo 'Environment Variables:'
	@echo
	@echo "  $(GREEN)Build Settings:$(RESET)"
	@echo '    PANFSPKG_NAME                            Name of the PanFS package. (default: $(PANFSPKG_NAME)).'
	@echo '    USE_HELM                                 Use Helm for deployment (true) or manifest (false) (default: $(USE_HELM)).'
	@echo
	@echo "  $(GREEN)Build/Deploy Settings:$(RESET)"
	@echo '    TEST_IMAGE                               Full image name for the test image (for sanity/e2e tests).'
	@echo '    CSI_IMAGE                          Full image name for the PanFS CSI Driver.'
	@echo '    CSIDFCKMM_IMAGE                          Full image name for the Kernel Module Management image.'
	@echo
	@echo "  $(GREEN)Pull/Push Images:$(RESET)"
	@echo '    REGISTRY_CREDS_FILE *                    Path to the file containing GCR credentials (JSON format).'
	@echo '    IMAGE_PULL_SECRET_NAME                   Name of the image pull secret for the registry (default: $(IMAGE_PULL_SECRET_NAME)).'
	@echo
	@echo "  $(GREEN)Realm Settings:$(RESET)"
	@echo '    REALM_ADDRESS                            Address of the PanFS realm (e.g., "panfs.example.com").'
	@echo '    REALM_USER                               Username for the PanFS realm.'
	@echo '    REALM_PASSWORD                           Password for the PanFS realm.'
	@echo '    REALM_PRIVATE_KEY                        Private key for the PanFS realm.'
	@echo '    REALM_PRIVATE_KEY_PASSPHRASE             Passphrase for the PanFS realm private key.'
	@echo
	@echo "  $(GREEN)Storage Class Settings:$(RESET)"
	@echo '    STORAGE_CLASS_NAME                       Name of the storage class to deploy (default: $(STORAGE_CLASS_NAME)).'
	@echo '    SET_STORAGECLASS_DEFAULT                 Boolean indicating if the storage class is set as default (default: true if STORAGE_CLASS_NAME is $(STORAGE_CLASS_NAME)).'
	@echo

# Defaults:
USE_HELM ?= false

STORAGE_CLASS_NAME ?= csi-panfs-storage-class
ifeq ($(STORAGE_CLASS_NAME),csi-panfs-storage-class)
SET_STORAGECLASS_DEFAULT := true
else
SET_STORAGECLASS_DEFAULT := false
endif

TEST_IMAGE ?= ghcr.io/panasasinc/panfs-container-storage-interface-oss/panfs-csi-sanity:stable

## Build Driver and DFC Images:

.PHONNY: compile-driver-bin
compile-driver-bin: ## Compile the PanFS CSI Driver binary
	docker run -it --arch=amd64 -v $(shell pwd):$(shell pwd) -w $(shell pwd) golang:1.24 go build -o build/panfs-csi pkg/csi-plugin/main.go

.PHONNY: build
build: build-driver-image build-dfc-image ## Build both the PanFS CSI Driver and DFC images

APP_VERSION ?= $$(git describe --tags --always --dirty)
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

.PHONY: build-driver-image
build-driver-image: ## Build the PanFS CSI Driver Docker image
	docker build -t $(CSI_IMAGE) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg GIT_COMMIT=$(shell git rev-parse --short HEAD) \
		.

.PHONY: build-dfc-image
build-dfc-image: ## Build the Kernel Module Management Docker image
	docker build -t $(CSIDFCKMM_IMAGE) \
		-f dfc/Dockerfile.stub \
		.

.PHONY: build-test-image
build-test-image: ## Build the test image for the PanFS CSI Driver
	@if [ -z "$(TEST_IMAGE)" ]; then \
		echo "$(RED)Error: TEST_IMAGE is not set$(RESET)"; \
		exit 1; \
	fi

	docker build -t $(TEST_IMAGE) \
		-f tests/csi_sanity/Dockerfile \
		tests/csi_sanity/

.PHONY: run-unit-tests
run-unit-tests: ## Run unit tests for the PanFS CSI Driver
	docker run -it --arch=amd64 -v $(shell pwd):$(shell pwd) -w $(shell pwd) golang:1.24 go test -v -race ./pkg/...

.PHONY: coverage
coverage: ## Get code coverage report
	docker run -it --rm --arch=amd64 -v $(shell pwd):$(shell pwd) -w $(shell pwd) golang:1.24 bash -c ' \
		go test -coverprofile=coverage.out ./...; \
		go tool cover -html=coverage.out -o coverage.html \
	'

.PHONY: sanity-check
sanity-check: ## Run smoke tests using csi-sanity
	@if [ -z "$(TEST_IMAGE)" ]; then \
		echo "$(RED)Error: TEST_IMAGE is not set$(RESET)"; \
		exit 1; \
	fi

	@if [ -z "$(CSI_IMAGE)" ]; then \
		echo "$(RED)Error: CSI_IMAGE is not set$(RESET)"; \
		exit 1; \
	fi

	@CSI_TEST_IMAGE=$(TEST_IMAGE) \
	CSI_IMAGE=$(CSI_IMAGE) \
	docker compose -f tests/csi_sanity/docker-compose.yaml up \
		--abort-on-container-exit \
		--exit-code-from sanity_tests

## Provisioning:

.PHONY: deploy-cert-manager
deploy-cert-manager: ## Deploy Certificate Manager
	@helm repo add jetstack https://charts.jetstack.io --force-update
	@helm upgrade --install \
		cert-manager jetstack/cert-manager \
		--namespace cert-manager \
		--create-namespace \
		--version v1.18.2 \
		--set crds.enabled=true
	@echo
	@echo "Checking Resources in 'cert-manager' namespace"
	kubectl get all -n cert-manager
	@echo "$(GREEN)SUCCESS: installed cert-manager$(RESET)"
	@echo

.PHONY: deploy-kmm-engine
deploy-kmm-engine: deploy-cert-manager ## Deploy Kernel Module Management Engine
	kubectl apply -k https://github.com/kubernetes-sigs/kernel-module-management/config/default
	kubectl get crd | grep kmm

.PHONY: deploy-csi
deploy-csi: deploy-driver deploy-storageclass ## Deploy the complete PanFS CSI solution (driver, DFC module, and storage class)

.PHONY: deploy-driver-ns-prereq
deploy-driver-ns-prereq: ## Create namespace and image pull secret for the PanFS CSI Driver
	kubectl create namespace csi-panfs --dry-run=client -o yaml | kubectl apply -f -
	@echo "$(GREEN)Created namespace 'csi-panfs'$(RESET)"
	@echo
	kubectl label namespace csi-panfs pod-security.kubernetes.io/enforce=privileged --overwrite
	@echo "$(GREEN)Labeled namespace 'csi-panfs' with pod-security.kubernetes.io/enforce=privileged$(RESET)"

.PHONY: deploy-driver-info deploy-storageclass-info
info: deploy-driver-info deploy-storageclass-info

.PHONY: deploy-driver-info
deploy-driver-info: ## Display information about the PanFS CSI Driver to be installed
	@echo "CSI Image:    $(CSI_IMAGE)"
	@echo "DFC Version:  $(DFC_VERSION)"
	@echo "DFC Registry: $(DFC_REGISTRY)"
	@echo

.PHONY: deploy-driver-with-helm
deploy-driver-with-helm:
	@echo "Deploying PanFS CSI Driver using Helm chart since USE_HELM is set..."
	@if [ '$(DFC_VERSION)' = 'stub' ]; then \
		helm upgrade --install csi-panfs charts/panfs \
			--namespace csi-panfs \
			--set csi.image="$(CSI_IMAGE)" \
			--set csi.pullPolicy="IfNotPresent" \
			--set dfc.version="$(DFC_VERSION)" \
			--set dfc.privateRegistry="$(DFC_REGISTRY)" \
			--set dfc.pullPolicy="IfNotPresent" \
			--set kmm.enabled="false" \
			--set seLinux="false" \
			--wait; \
	else \
		helm upgrade --install csi-panfs charts/panfs \
			--namespace csi-panfs \
			--set csi.image="$(CSI_IMAGE)" \
			--set dfc.version="$(DFC_VERSION)" \
			--set dfc.privateRegistry="$(DFC_REGISTRY)" \
			--set imagePullSecrets[0]="$(IMAGE_PULL_SECRET_NAME)" \
			--wait; \
	fi
	@echo "$(GREEN)Successfully deployed PanFS CSI Driver$(RESET)"
	@echo
	@helm get values csi-panfs -n csi-panfs
	@echo

.PHONY: deploy-driver-with-manifest
deploy-driver-with-manifest:
	@echo "Deploying PanFS CSI Driver using manifest file..."
	@cat deploy/k8s/csi-driver/template-csi-panfs.yaml | \
	sed 's@<IMAGE_PULL_SECRET_NAME>@$(IMAGE_PULL_SECRET_NAME)@' | \
	sed 's@<DFC_RELEASE_VERSION>@$(DFC_VERSION)@g' | \
	sed 's@<PANFS_DFC_KMM_PRIVATE_REGISTRY>@$(DFC_REGISTRY)@g' | \
	sed 's@[^ ]*panfs-csi-driver:.*@$(CSI_IMAGE)@g' | \
	kubectl apply --server-side -f -
	@echo "$(GREEN)Successfully deployed PanFS CSI Driver using manifest file deploy/k8s/csi-panfs-driver.yaml$(RESET)"
	@echo

.PHONY: deploy-driver
deploy-driver: deploy-driver-info ## Deploy PanFS CSI Driver (Includes DFC)
	@if [ "$(USE_HELM)" = "true" ]; then \
		make deploy-driver-with-helm; \
	else \
		make deploy-driver-with-manifest; \
	fi

	@echo "Waiting for the PanFS CSI Controller deployment to be ready..."
	@timeout 60 kubectl -n csi-panfs rollout status deployment csi-panfs-controller

	@echo "Waiting for the PanFS CSI Node daemonset to be ready..."
	@timeout 60 kubectl -n csi-panfs rollout status daemonset csi-panfs-node

.PHONY: deploy-storageclass-info
deploy-storageclass-info: ## Display information about the PanFS CSI Storage Class to be deployed
	@echo "Storage Class Name: $(STORAGE_CLASS_NAME)"
	@echo "Set as Default:     $(SET_STORAGECLASS_DEFAULT)"
	@echo

.PHONY: sc
sc: deploy-storageclass ## Alias for deploy-storageclass

.PHONY: deploy-storageclass-with-helm
deploy-storageclass-with-helm:
	@echo "Deploying PanFS CSI Storage Class '$(STORAGE_CLASS_NAME)' with Helm since USE_HELM is set..."
	@helm upgrade --install $(STORAGE_CLASS_NAME) charts/storageclass \
		--namespace $(STORAGE_CLASS_NAME) \
		--create-namespace \
		--set csiPanFSDriver.namespace="csi-panfs" \
		--set setAsDefaultStorageClass=$(SET_STORAGECLASS_DEFAULT) \
		--set realm.address="${REALM_ADDRESS}" \
		--set realm.username="${REALM_USER}" \
		--set realm.password="${REALM_PASSWORD}" \
		--wait
	@echo "$(GREEN)Successfully deployed PanFS CSI Storage Class '$(STORAGE_CLASS_NAME)'$(RESET)"
	@echo

.PHONY: deploy-storageclass-with-manifest
deploy-storageclass-with-manifest:
	@echo "Deploying PanFS CSI Storage Class using manifest file..."
	@export STORAGE_CLASS_NAME=$(STORAGE_CLASS_NAME); \
	export REALM_ADDRESS=$(REALM_ADDRESS); \
	export REALM_USERNAME=$(REALM_USER); \
	export REALM_PASSWORD=$(REALM_PASSWORD); \
	export REALM_PRIVATE_KEY=$(REALM_PRIVATE_KEY); \
	export REALM_PRIVATE_KEY_PASSPHRASE=$(REALM_PRIVATE_KEY_PASSPHRASE); \
	export CSI_CONTROLLER_SA=csi-panfs-controller; \
	export CSI_NAMESPACE=csi-panfs; \
	kubectl create namespace $(STORAGE_CLASS_NAME) --dry-run=client -o yaml | kubectl apply -f -; \
	cat deploy/k8s/storage-class/template-secret-in-dedicated-ns.yaml | \
	sed 's|<|$$|;s/\([^ ]\)>/\1/;s|is-default-class: "false"|is-default-class: "$(SET_STORAGECLASS_DEFAULT)"|' | \
	sed 's|csi-panfs-storage-class|$(STORAGE_CLASS_NAME)|' | envsubst | kubectl apply --server-side -f -
	@echo "$(GREEN)Successfully deployed PanFS CSI Storage Class using manifest file deploy/k8s/storage-class/template-secret-in-dedicated-ns.yaml$(RESET)"
	@echo

.PHONY: deploy-storageclass
deploy-storageclass: deploy-storageclass-info ## Deploy PanFS CSI Storage Class
	@if [ -z "$(REALM_ADDRESS)" ]; then \
		echo "$(RED)ERROR: REALM_ADDRESS is not set$(RESET)"; \
		printf "USAGE:\n  export REALM_ADDRESS=...\n  export REALM_USER=...\n  export REALM_PASSWORD=... (or export REALM_PRIVATE_KEY=...)\n  make deploy-storageclass\n"; \
		exit 1; \
	fi

	@if [ -z "$(REALM_USER)" ]; then \
		echo "$(RED)ERROR: REALM_USER is not set$(RESET)"; \
		printf "USAGE:\n  export REALM_ADDRESS=...\n  export REALM_USER=...\n  export REALM_PASSWORD=... (or export REALM_PRIVATE_KEY=...)\n  make deploy-storageclass\n"; \
		exit 1; \
	fi

	@if [ -z "$(REALM_PASSWORD)" ] && [ -z "$(REALM_PRIVATE_KEY)" ]; then \
		echo "$(RED)ERROR: Either REALM_PASSWORD or REALM_PRIVATE_KEY must be set$(RESET)"; \
		printf "USAGE:\n  export REALM_ADDRESS=...\n  export REALM_USER=...\n  export REALM_PASSWORD=... (or export REALM_PRIVATE_KEY=...)\n  make deploy-storageclass\n"; \
		exit 1; \
	fi
	
	@if [ "$(USE_HELM)" = "true" ]; then \
		make deploy-storageclass-with-helm; \
	else \
		make deploy-storageclass-with-manifest; \
	fi

	@echo "kubectl get storageclass $(STORAGE_CLASS_NAME)"
	@kubectl get storageclass $(STORAGE_CLASS_NAME) | \
		awk '/$(STORAGE_CLASS_NAME)/ {gsub(/$(STORAGE_CLASS_NAME)/, "$(YELLOW)$(STORAGE_CLASS_NAME)$(RESET)"); print; next} {print}'
	@echo

## Troubleshooting:

.PHONY: validate verify
validate: verify
verify: deploy-driver-info ## Verify the installation of the PanFS CSI Driver and its components
	@CSI_IMAGE=$(CSI_IMAGE) DFC_VERSION=$(DFC_VERSION) sh tests/helper/lib/validate.sh

## Uninstall CSI Driver and Storage Class:
.PHONY: uninstall-check
uninstall-check: ## Check if it is safe to uninstall the PanFS CSI Storage Class
	@if kubectl get pv 2>&1 | grep $(STORAGE_CLASS_NAME) 2>/dev/null; then \
		echo "$(RED)Error: There are still Persistent Volumes using the storage class '$(STORAGE_CLASS_NAME)'. Please delete them before uninstalling the storage class.$(RESET)"; \
		kubectl get pv | grep $(STORAGE_CLASS_NAME); \
		exit 1; \
	fi

.PHONY: uninstall-driver
uninstall-driver: ## Uninstall the PanFS CSI Driver
	@kubectl delete -f deploy/k8s/csi-driver/template-csi-panfs.yaml --ignore-not-found
	@kubectl delete secret -n csi-panfs -l owner=helm
	@kubectl label node -l node-role.kubernetes.io/worker= node.kubernetes.io/csi-driver.panfs.ready- --overwrite;
	@echo "$(GREEN)Successfully uninstalled PanFS CSI Driver$(RESET)"
	@echo

.PHONY: uninstall-storageclass
uninstall-storageclass: ## Uninstall the PanFS CSI Storage Class
	@kubectl delete -f deploy/k8s/storage-class/template-secret-in-dedicated-ns.yaml --ignore-not-found
	@kubectl delete namespace $(STORAGE_CLASS_NAME) --ignore-not-found
	@echo "$(GREEN)Successfully uninstalled PanFS CSI Storage Class '$(STORAGE_CLASS_NAME)'$(RESET)"
	@echo

.PHONY: uninstall
uninstall: ## Uninstall both the PanFS CSI Driver and Storage Class 
	@make uninstall-driver
# 	@make uninstall-storageclass

## Prepare to Release:

.PHONY: manifest-driver
manifest-driver: ## Generate manifests for the PanFS CSI Driver
	@echo "Generating manifests for the PanFS CSI Driver..."
	@mkdir -p deploy/k8s/csi-driver/
	helm template csi-panfs charts/panfs --namespace csi-panfs --set dfc.version="1.2.3-4 # Update with the desired DFC release version" > deploy/k8s/csi-driver/example-csi-panfs.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set imagePullSecrets[0]="<IMAGE_PULL_SECRET_NAME>" > deploy/k8s/csi-driver/template-csi-panfs.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set seLinux=false > deploy/k8s/csi-driver/template-csi-panfs-without-selinux.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set kmm.enabled=false > deploy/k8s/csi-driver/template-csi-panfs-without-kmm.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set dfc.encryptionSupport=true > deploy/k8s/csi-driver/template-csi-panfs-with-e2ee.yaml
	sed -i $(shell sed -h 2>&1 | grep GNU >/dev/null || echo "''") '/^# Source:/d' deploy/k8s/csi-driver/*.yaml

.PHONY: manifest-storageclass
manifest-storageclass: ## Generate manifests for the PanFS CSI Storage Class
	@echo "Generating manifests for the PanFS CSI Storage Class..."
	@mkdir -p deploy/k8s/storage-class/
	helm template csi-panfs-storage-class charts/storageclass \
		--namespace csi-panfs-storage-class \
		--set csiPanFSDriver.namespace="csi-panfs" \
		--set setAsDefaultStorageClass=false \
		--set realm.address="panfs-dummy.com" \
		--set realm.username="dummy-user" \
		--set realm.password="dummy-password" \
		--set realm.privateKey="" \
		--set realm.privateKeyPassphrase="" | \
		grep -v '^# Source:' > deploy/k8s/storage-class/example-csi-panfs-storage-class.yaml

	helm template csi-panfs-storage-class charts/storageclass \
		--namespace csi-panfs-storage-class \
		--set csiPanFSDriver.namespace="<CSI_NAMESPACE>" \
		--set setAsDefaultStorageClass=false \
		--set realm.address="<REALM_ADDRESS>" \
		--set realm.username="<REALM_USERNAME>" \
		--set realm.password="<REALM_PASSWORD>" \
		--set realm.privateKey="<REALM_PRIVATE_KEY>" \
		--set realm.privateKeyPassphrase="<REALM_PRIVATE_KEY_PASSPHRASE>" \
		--set realm.kmipConfigData="<KMIP_CONFIG_DATA>" | \
		sed 's|csi-panfs-storage-class-name|<STORAGE_CLASS_NAME>|' | \
		grep -v '^# Source:' > deploy/k8s/storage-class/template-secret-in-dedicated-ns.yaml
	
	helm template csi-panfs-storage-class-name charts/storageclass \
		--namespace csi-panfs \
		--set setAsDefaultStorageClass=false \
		--set realm.address="<REALM_ADDRESS>" \
		--set realm.username="<REALM_USERNAME>" \
		--set realm.password="<REALM_PASSWORD>" \
		--set realm.privateKey="<REALM_PRIVATE_KEY>" \
		--set realm.privateKeyPassphrase="<REALM_PRIVATE_KEY_PASSPHRASE>" \
		--set realm.kmipConfigData="<KMIP_CONFIG_DATA>" | \
		sed 's|csi-panfs-storage-class-name|<STORAGE_CLASS_NAME>|' | \
		grep -v '^# Source:' | \
		sed 's|csi-panfs|<CSI_NAMESPACE>|' > deploy/k8s/storage-class/template-secret-in-driver-ns.yaml

.PHONY: manifests
manifests: manifest-driver manifest-storageclass ## Generate manifests for the PanFS CSI Driver and Storage Class
	helm-docs
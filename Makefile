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
BOLD=\033[1m

.PHONY: help
help:
	@printf "$(BOLD)Please specify the target to build, deploy, or manage the PanFS CSI Driver and its components:$(RESET)\n\n"
	@printf "$(BOLD)Usage:$(RESET)\n\n"
	@printf "  make $(BLUE)<targets>$(RESET)\n\n"
	@printf "$(BOLD)Targets:$(RESET)\n"
	@awk '\
		/^## / {\
			sub(/^## /, "");\
			printf "\n  $(BOLD)%s$(RESET)\n", $$0\
		}\
		/^[a-zA-Z0-9_-]+:.*##/ {\
			match($$0, /^([a-zA-Z0-9_-]+):.*## (.*)$$/, arr);\
			if (arr[1]&&arr[2]){printf "    $(BLUE)%-40s$(RESET) %s\n", arr[1], arr[2]}\
		}' Makefile
	@printf "\n$(BOLD)Environment Variables:$(RESET)\n\n"
	@printf "  $(BOLD)$(GREEN)[Build/Deploy Settings]$(RESET)\n"
	@printf "    TEST_IMAGE                               Full image name for the test image (for sanity tests).\n"
	@printf "    CSI_IMAGE                                Full image name for the PanFS CSI Driver (default: $(CSI_IMAGE)).\n"
	@printf "    DFC_IMAGE                                Full image name for the Kernel Module Management image.\n"
	@printf "    DFC_VERSION                              Version of the DFC to deploy.\n"
	@printf "    USE_HELM                                 Use Helm for deployment (true) or manifest (false) (default: $(USE_HELM)).\n"
	@printf "    IMAGE_PULL_SECRET_NAME                   Name of the image pull secret for the registry (default: $(IMAGE_PULL_SECRET_NAME)).\n\n"
	@printf "  $(BOLD)$(GREEN)[Realm Settings]$(RESET)\n"
	@printf "    REALM_ADDRESS                            Address of the PanFS realm (e.g., "panfs.example.com").\n"
	@printf "    REALM_USER                               Username for the PanFS realm.\n"
	@printf "    REALM_PASSWORD                           Password for the PanFS realm.\n"
	@printf "    REALM_PRIVATE_KEY                        Private key for the PanFS realm.\n"
	@printf "    REALM_PRIVATE_KEY_PASSPHRASE             Passphrase for the PanFS realm private key.\n"
	@printf "    KMIP_CONFIG_DATA                         KMIP configuration data for volume encryption.\n\n"
	@printf "  $(BOLD)$(GREEN)[Storage Class Settings]$(RESET)\n"
	@printf "    STORAGE_CLASS_NAME                       Name of the storage class to deploy (default: $(STORAGE_CLASS_NAME)).\n"
	@printf "    SET_STORAGECLASS_DEFAULT                 Boolean indicating if the storage class is set as default (default: true if STORAGE_CLASS_NAME is $(STORAGE_CLASS_NAME)).\n\n"

# Defaults:
USE_HELM ?= false

STORAGE_CLASS_NAME ?= csi-panfs-storage-class
ifeq ($(STORAGE_CLASS_NAME),csi-panfs-storage-class)
SET_STORAGECLASS_DEFAULT := true
else
SET_STORAGECLASS_DEFAULT := false
endif

CSI_IMAGE  ?= ghcr.io/panasasinc/panfs-container-storage-interface-oss/panfs-csi:1.2.3
TEST_IMAGE ?= ghcr.io/panasasinc/panfs-container-storage-interface-oss/panfs-csi-sanity:stable

## [Build Driver and DFC Images]

.PHONY: compile-driver-bin
compile-driver-bin: ## Compile the PanFS CSI Driver binary
	@printf "$(BOLD)Compiling PanFS CSI Driver binary...$(RESET)\n"
	@mkdir -p build
	docker run --rm -v $(shell pwd):$(shell pwd) -w $(shell pwd) golang:1.24 go build -o build/panfs-csi cmd/csi-plugin/main.go
	@printf "$(GREEN)Successfully compiled PanFS CSI Driver binary$(RESET)\n\n"

.PHONY: build
build: build-driver-image build-dfc-image ## Build both the PanFS CSI Driver and DFC images

APP_VERSION ?= $(shell git describe --tags --always --dirty)
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

.PHONY: build-driver-image
build-driver-image: ## Build the PanFS CSI Driver Docker image
	@if [ -z "$(CSI_IMAGE)" ]; then \
		printf "$(RED)CSI_IMAGE is not set$(RESET)\n"; \
		exit 1; \
	fi
	docker build -t $(CSI_IMAGE) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg GIT_COMMIT=$(shell git rev-parse --short HEAD) \
		.
	@printf "$(GREEN)Successfully built PanFS CSI Driver Docker image: $(CSI_IMAGE)$(RESET)\n\n"

.PHONY: build-dfc-image
build-dfc-image: ## Build the Kernel Module Management Docker image
	@printf "$(BOLD)Building Kernel Module Management Docker image...$(RESET)\n"
	@if [ -z "$(DFC_IMAGE)" ]; then \
		printf "$(RED)DFC_IMAGE is not set$(RESET)\n"; \
		exit 1; \
	fi
	docker build -t $(DFC_IMAGE) -f dfc/Dockerfile.stub dfc/
	@printf "$(GREEN)Successfully built Kernel Module Management Docker image: $(DFC_IMAGE)$(RESET)\n\n"

.PHONY: build-test-image
build-test-image: ## Build the test image for the PanFS CSI Driver
	@printf "$(BOLD)Building test image for the PanFS CSI Driver...$(RESET)\n"
	@if [ -z "$(TEST_IMAGE)" ]; then \
		printf "$(RED)Error: TEST_IMAGE is not set$(RESET)\n"; \
		exit 1; \
	fi
	docker build -t $(TEST_IMAGE) \
		-f tests/csi_sanity/Dockerfile \
		tests/csi_sanity/
	@printf "$(GREEN)Successfully built test image for the PanFS CSI Driver$(RESET)\n\n"

.PHONY: run-unit-tests
run-unit-tests: ## Run unit tests for the PanFS CSI Driver
	@printf "$(BOLD)Running unit tests for the PanFS CSI Driver...$(RESET)\n"
	docker run --rm -v $(shell pwd):$(shell pwd) -w $(shell pwd) golang:1.24 go test -v -race ./pkg/...
	@printf "$(GREEN)Successfully ran unit tests for the PanFS CSI Driver$(RESET)\n\n"

.PHONY: coverage
coverage: ## Get code coverage report
	docker run --rm -v $(shell pwd):$(shell pwd) -w $(shell pwd) golang:1.24 bash -c ' \
		go test -coverprofile=coverage.out ./...; \
		go tool cover -html=coverage.out -o coverage.html \
	'
	@printf "$(GREEN)Successfully generated code coverage report$(RESET)\n\n"

.PHONY: sanity-check
sanity-check: ## Run smoke tests using csi-sanity
	@printf '$(BOLD)Smoke tests using csi-sanity:$(RESET)\n'
	@printf '  %-25s "%s"\n' "CSI_IMAGE:" "$(shell [ -n '$(CSI_IMAGE)' ] && echo $(CSI_IMAGE) || echo "$(RED)unknown$(RESET)")"
	@printf '  %-25s "%s"\n' "TEST_IMAGE:" "$(shell [ -n '$(TEST_IMAGE)' ] && echo $(TEST_IMAGE) || echo "$(RED)unknown$(RESET)")"
	@if [ -z "$(CSI_IMAGE)" ] || [ -z "$(TEST_IMAGE)" ]; then \
		printf '\nPlease set the above environment variables before deploying the driver.\n'; \
		exit 1; \
	fi

	@CSI_TEST_IMAGE=$(TEST_IMAGE) \
	CSI_IMAGE=$(CSI_IMAGE) \
	docker compose -f tests/csi_sanity/docker-compose.yaml up \
		--abort-on-container-exit \
		--exit-code-from sanity_tests
	@printf "$(GREEN)Successfully ran smoke tests using csi-sanity$(RESET)\n\n"

## [Provisioning]

.PHONY: deploy-cert-manager
deploy-cert-manager: ## Deploy Certificate Manager
	@printf "$(BOLD)Deploying Certificate Manager...$(RESET)\n"
	@helm repo add jetstack https://charts.jetstack.io --force-update
	@helm upgrade --install \
		cert-manager jetstack/cert-manager \
		--namespace cert-manager \
		--create-namespace \
		--version v1.18.2 \
		--set crds.enabled=true
	@printf "\n$(BOLD)Checking Resources in 'cert-manager' namespace...$(RESET)\n"
	kubectl get all -n cert-manager
	@printf "$(GREEN)Successfully deployed Certificate Manager$(RESET)\n\n"

.PHONY: deploy-kmm-engine
deploy-kmm-engine: deploy-cert-manager ## Deploy Kernel Module Management Engine
	@printf "$(BOLD)Deploying Kernel Module Management Engine...$(RESET)\n"
	kubectl apply -k https://github.com/kubernetes-sigs/kernel-module-management/config/default
	kubectl get crd | grep kmm
	@printf "$(GREEN)Successfully deployed Kernel Module Management Engine$(RESET)\n\n"

.PHONY: deploy-csi
deploy-csi: deploy-driver deploy-storageclass ## Deploy the complete PanFS CSI solution (driver, DFC module, and storage class)

.PHONY: deploy-driver-ns-prereq
deploy-driver-ns-prereq: ## Create namespace and image pull secret for the PanFS CSI Driver
	@printf "$(BOLD)Creating namespace 'csi-panfs' and image pull secret...$(RESET)\n"
	kubectl create namespace csi-panfs --dry-run=client -o yaml | kubectl apply -f -
	@printf "$(GREEN)Created namespace 'csi-panfs'$(RESET)\n\n"
	kubectl label namespace csi-panfs pod-security.kubernetes.io/enforce=privileged --overwrite
	@printf "$(GREEN)Labeled namespace 'csi-panfs' with pod-security.kubernetes.io/enforce=privileged$(RESET)\n\n"

.PHONY: deploy-driver-info deploy-storageclass-info
info: deploy-driver-info deploy-storageclass-info

.PHONY: deploy-driver-info
deploy-driver-info: ## Display information about the PanFS CSI Driver to be installed
	@printf '$(BOLD)Deployment Method:$(RESET)\n'
	@printf '  %-25s "%s"\n' "USE_HELM:" "$(USE_HELM)"
	@printf '\n$(BOLD)Registry access settings defined through environment variables:$(RESET)\n'
	@printf '  %-25s "%s"\n' "IMAGE_PULL_SECRET_NAME:" "$(shell [ -n '$(IMAGE_PULL_SECRET_NAME)' ] && echo $(IMAGE_PULL_SECRET_NAME) || echo "$(RED)unknown$(RESET)")"
	@printf '\n$(BOLD)Driver settings defined through environment variables:$(RESET)\n'
	@printf '  %-25s "%s"\n' "CSI_IMAGE:" "$(shell [ -n '$(CSI_IMAGE)' ] && echo $(CSI_IMAGE) || echo "$(RED)unknown$(RESET)")"
	@printf '  %-25s "%s"\n' "DFC_VERSION:" "$(shell [ -n '$(DFC_VERSION)' ] && echo $(DFC_VERSION) || echo "$(RED)unknown$(RESET)")"
	@printf '  %-25s "%s"\n' "DFC_REGISTRY:" "$(shell [ -n '$(DFC_REGISTRY)' ] && echo $(DFC_REGISTRY) || echo "$(RED)unknown$(RESET)")"

	@if [ -z "$(CSI_IMAGE)" ] || [ -z "$(DFC_VERSION)" ] || [ -z "$(DFC_REGISTRY)" ]; then \
		printf '\nPlease set the above environment variables before deploying the driver.\n'; \
		exit 1; \
	fi

.PHONY: deploy-driver-with-helm
deploy-driver-with-helm:
	@printf "$(BOLD)Deploying PanFS CSI Driver using Helm chart since USE_HELM is set...$(RESET)\n"
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
	@printf "$(GREEN)Successfully deployed PanFS CSI Driver$(RESET)\n\n"
	@helm get values csi-panfs -n csi-panfs
	@printf "$(GREEN)Successfully displayed values for PanFS CSI Driver$(RESET)\n\n"

.PHONY: deploy-driver-with-manifest
deploy-driver-with-manifest:
	@printf "$(BOLD)Deploying PanFS CSI Driver using manifest file...$(RESET)\n"
	@cat deploy/k8s/csi-driver/template-csi-panfs.yaml | \
	sed 's@<IMAGE_PULL_SECRET_NAME>@$(IMAGE_PULL_SECRET_NAME)@' | \
	sed 's@<DFC_RELEASE_VERSION>@$(DFC_VERSION)@g' | \
	sed 's@<PANFS_DFC_KMM_PRIVATE_REGISTRY>@$(DFC_REGISTRY)@g' | \
	sed 's@[^ ]*panfs-csi-driver:.*@$(CSI_IMAGE)@g' | \
	kubectl apply --server-side -f -
	@printf "$(GREEN)Successfully deployed PanFS CSI Driver using manifest file deploy/k8s/csi-panfs-driver.yaml$(RESET)\n\n"

.PHONY: deploy-driver
deploy-driver: deploy-driver-info ## Deploy PanFS CSI Driver (Includes DFC)
	@([ "$(USE_HELM)" = "true" ] || [ "$(DFC_VERSION)" = "stub" ]) && make deploy-driver-with-helm || make deploy-driver-with-manifest
	@printf "$(BOLD)Waiting for PanFS CSI Driver to be enrolled...$(RESET)\n"
	@timeout 15m bash tests/helper/lib/wait.sh
	@printf "$(GREEN)PanFS CSI Driver is successfully enrolled!$(RESET)\n\n"

.PHONY: deploy-storageclass-info
deploy-storageclass-info: ## Display information about the PanFS CSI Storage Class to be deployed
	@printf "$(BOLD)Deployment Method:$(RESET)\n"
	@printf "  %-25s "%s"\n" "USE_HELM:" "$(USE_HELM)"
	@printf "\n$(BOLD)Storage Class settings defined through environment variables:$(RESET)\n"
	@printf "  %-25s "%s"\n" "STORAGE_CLASS_NAME:" "$(shell [ -n '$(STORAGE_CLASS_NAME)' ] && echo $(STORAGE_CLASS_NAME) || echo "$(RED)unknown$(RESET)")"
	@printf "  %-25s "%s"\n" "SET_STORAGECLASS_DEFAULT:" "$(shell [ -n '$(SET_STORAGECLASS_DEFAULT)' ] && echo $(SET_STORAGECLASS_DEFAULT) || echo "$(RED)unknown$(RESET)")"
	@printf "\n$(BOLD)PanFS Realm credentials defined through environment variables:$(RESET)\n"
	@printf "  %-25s "%s"\n" "REALM_ADDRESS:" "$(shell [ -n '$(REALM_ADDRESS)' ] && echo $(REALM_ADDRESS) || echo "$(RED)unknown$(RESET)")"
	@printf "  %-25s "%s"\n" "REALM_USER:" "$(shell [ -n '$(REALM_USER)' ] && echo $(REALM_USER) || echo "$(RED)unknown$(RESET)")"
	@printf "  %-25s "%s"\n" "REALM_PASSWORD:" "$(shell [ -n '$(REALM_PASSWORD)' ] && echo "*****" || echo "$(RED)unknown$(RESET)")"

	@if [ -z "$(REALM_ADDRESS)" ] || [ -z "$(REALM_USER)" ] || [ -z "$(REALM_PASSWORD)" ]; then \
		printf '\nPlease set the above environment variables before deploying the storage class.\n'; \
		exit 1; \
	fi

	@if [ -z "$(STORAGE_CLASS_NAME)" ] || [ -z "$(SET_STORAGECLASS_DEFAULT)" ]; then \
		printf '\nPlease set the above environment variables before deploying the storage class.\n'; \
		exit 1; \
	fi

.PHONY: deploy-storageclass-with-helm
deploy-storageclass-with-helm:
	@printf "$(BOLD)Deploying PanFS CSI Storage Class '$(STORAGE_CLASS_NAME)' with Helm since USE_HELM is set...$(RESET)\n"
	@helm upgrade --install $(STORAGE_CLASS_NAME) charts/storageclass \
		--namespace $(STORAGE_CLASS_NAME) \
		--create-namespace \
		--set csiPanFSDriver.namespace="csi-panfs" \
		--set setAsDefaultStorageClass=$(SET_STORAGECLASS_DEFAULT) \
		--set realm.address="${REALM_ADDRESS}" \
		--set realm.username="${REALM_USER}" \
		--set realm.password="${REALM_PASSWORD}" \
		--set realm.privateKey="$(REALM_PRIVATE_KEY)" \
		--set realm.privateKeyPassphrase="$(REALM_PRIVATE_KEY_PASSPHRASE)" \
		--set realm.kmipConfigData="$(KMIP_CONFIG_DATA)" \
		--wait
	@printf "$(GREEN)Successfully deployed PanFS CSI Storage Class '$(STORAGE_CLASS_NAME)'$(RESET)\n\n"

.PHONY: deploy-storageclass-with-manifest
deploy-storageclass-with-manifest:
	@printf "$(BOLD)Deploying PanFS CSI Storage Class using manifest file...$(RESET)\n"
	@export STORAGE_CLASS_NAME=$(STORAGE_CLASS_NAME); \
	export REALM_ADDRESS=$(REALM_ADDRESS); \
	export REALM_USERNAME=$(REALM_USER); \
	export REALM_PASSWORD=$(REALM_PASSWORD); \
	export REALM_PRIVATE_KEY="$(REALM_PRIVATE_KEY)"; \
	export REALM_PRIVATE_KEY_PASSPHRASE="$(REALM_PRIVATE_KEY_PASSPHRASE)"; \
	export KMIP_CONFIG_DATA="$(KMIP_CONFIG_DATA)"; \
	export CSI_CONTROLLER_SA=csi-panfs-controller; \
	export CSI_NAMESPACE=csi-panfs; \
	kubectl create namespace $(STORAGE_CLASS_NAME) --dry-run=client -o yaml | kubectl apply -f -; \
	cat deploy/k8s/storage-class/template-secret-in-dedicated-ns.yaml | \
	sed 's|<|$$|;s/\([^ ]\)>/\1/;s|is-default-class: "false"|is-default-class: "$(SET_STORAGECLASS_DEFAULT)"|' | \
	sed 's|csi-panfs-storage-class|$(STORAGE_CLASS_NAME)|' | envsubst | kubectl apply --server-side -f -
	@printf "$(GREEN)Successfully deployed PanFS CSI Storage Class using manifest file deploy/k8s/storage-class/template-secret-in-dedicated-ns.yaml$(RESET)\n\n"

.PHONY: deploy-storageclass
deploy-storageclass: deploy-storageclass-info ## Deploy PanFS CSI Storage Class
	@if [ "$(USE_HELM)" = "true" ]; then \
		make deploy-storageclass-with-helm; \
	else \
		make deploy-storageclass-with-manifest; \
	fi

	@printf "$(BOLD)kubectl get storageclass $(STORAGE_CLASS_NAME)$(RESET)\n"
	@kubectl get storageclass $(STORAGE_CLASS_NAME) | \
		awk '/$(STORAGE_CLASS_NAME)/ {gsub(/$(STORAGE_CLASS_NAME)/, "$(YELLOW)$(STORAGE_CLASS_NAME)$(RESET)"); print; next} {print}'
	@printf "$(GREEN)Successfully verified the deployment of PanFS CSI Storage Class '$(STORAGE_CLASS_NAME)'$(RESET)\n\n"

## [Troubleshooting]

.PHONY: validate verify
validate: verify           ## Alias for verify
verify: deploy-driver-info ## Verify the installation of the PanFS CSI Driver and its components
	@CSI_IMAGE=$(CSI_IMAGE) DFC_VERSION=$(DFC_VERSION) bash tests/helper/lib/validate.sh

## [Uninstall CSI Driver and Storage Class]
.PHONY: uninstall-check
uninstall-check: ## Check if it is safe to uninstall the PanFS CSI Storage Class
	@if kubectl get pv -o jsonpath='{range .items[?(@.metadata.annotations.pv\.kubernetes\.io/provisioned-by=="com.vdura.csi.panfs")]}{.metadata.name}{end}' | grep -q .; then \
		printf "$(RED)Error: There are Persistent Volumes provisioned with com.vdura.csi.panfs CSI driver.$(RESET)\n"; \
		printf "$(RED)       Please delete them before uninstalling the storage class and driver.$(RESET)\n\n"; \
		printf "The following Persistent Volumes are still present:\n"; \
		kubectl get pv -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.pv\.kubernetes\.io/provisioned-by}{"\t"}{.spec.storageClassName}{"\n"}{end}' | grep com.vdura.csi.panfs; \
		exit 1; \
	fi

.PHONY: uninstall-driver
uninstall-driver: ## Uninstall the PanFS CSI Driver
	@kubectl delete -f deploy/k8s/csi-driver/template-csi-panfs.yaml --ignore-not-found
	@kubectl delete secret -n csi-panfs -l owner=helm
	@kubectl label node -l node-role.kubernetes.io/worker= node.kubernetes.io/csi-driver.panfs.ready- --overwrite;
	@printf "$(GREEN)Successfully uninstalled PanFS CSI Driver$(RESET)\n\n"

.PHONY: uninstall-storageclass
uninstall-storageclass: ## Uninstall the PanFS CSI Storage Class
	@kubectl delete -f deploy/k8s/storage-class/template-secret-in-dedicated-ns.yaml --ignore-not-found
	@kubectl delete namespace $(STORAGE_CLASS_NAME) --ignore-not-found
	@printf "$(GREEN)Successfully uninstalled PanFS CSI Storage Class '$(STORAGE_CLASS_NAME)'$(RESET)\n\n"

.PHONY: uninstall
uninstall: uninstall-check ## Uninstall both the PanFS CSI Driver and Storage Class
	@make uninstall-driver
	@make uninstall-storageclass

.PHONY: uninstall-kmm
uninstall-kmm: ## Uninstall the Kernel Module Management Engine
	@kubectl delete -k https://github.com/kubernetes-sigs/kernel-module-management/config/default
	@printf "$(GREEN)Successfully uninstalled Kernel Module Management Engine$(RESET)\n\n"

.PHONY: uninstall-cert-manager
uninstall-cert-manager: ## Uninstall the Certificate Manager
	@helm uninstall cert-manager --namespace cert-manager --wait
	@printf "$(GREEN)Successfully uninstalled Certificate Manager$(RESET)\n\n"

## [Prepare to Release]

.PHONY: manifest-driver
manifest-driver: ## Generate manifests for the PanFS CSI Driver
	@printf "$(BOLD)Generating manifests for the PanFS CSI Driver...$(RESET)\n"
	@mkdir -p deploy/k8s/csi-driver/
	helm template csi-panfs charts/panfs --namespace csi-panfs --set dfc.version="1.2.3-4 # Update with the desired DFC release version" > deploy/k8s/csi-driver/example-csi-panfs.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set imagePullSecrets[0]="<IMAGE_PULL_SECRET_NAME>" > deploy/k8s/csi-driver/template-csi-panfs.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set seLinux=false > deploy/k8s/csi-driver/template-csi-panfs-without-selinux.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set kmm.enabled=false > deploy/k8s/csi-driver/template-csi-panfs-without-kmm.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set dfc.encryptionSupport=true > deploy/k8s/csi-driver/template-csi-panfs-with-e2ee.yaml
	sed -i $(shell sed -h 2>&1 | grep GNU >/dev/null || echo "''") '/^# Source:/d' deploy/k8s/csi-driver/*.yaml
	@printf "$(GREEN)Successfully generated manifests for the PanFS CSI Driver$(RESET)\n\n"

.PHONY: manifest-storageclass
manifest-storageclass: ## Generate manifests for the PanFS CSI Storage Class
	@printf "$(BOLD)Generating manifests for the PanFS CSI Storage Class...$(RESET)\n"
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
	@printf "$(GREEN)Successfully generated manifests for the PanFS CSI Storage Class$(RESET)\n\n"

.PHONY: manifests
manifests: manifest-driver manifest-storageclass ## Generate manifests for the PanFS CSI Driver and Storage Class
	@if command -v helm-docs >/dev/null 2>&1; then \
		helm-docs; \
	else \
		printf "$(RED)Error: helm-docs not found, skipping documentation generation$(RESET)\n"; \
		exit 1; \
	fi
	@printf "$(GREEN)Successfully generated documentation for the PanFS CSI Driver and Storage Class$(RESET)\n\n"
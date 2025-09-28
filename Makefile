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
	@echo "    export CSIDRIVER_IMAGE=..."
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
	@echo '    CSIDRIVER_IMAGE                          Full image name for the PanFS CSI Driver.'
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
KERNEL_VERSION ?= 4.18.0-553.el8_10.x86_64
USE_HELM ?= false
IMAGE_PULL_SECRET_NAME ?= gcr-json-key

STORAGE_CLASS_NAME ?= csi-panfs-storage-class
ifeq ($(STORAGE_CLASS_NAME),csi-panfs-storage-class)
SET_STORAGECLASS_DEFAULT := true
else
SET_STORAGECLASS_DEFAULT := false
endif

## Build Driver and DFC Images:

.PHONNY: compile-driver-bin
compile-driver-bin: ## Compile the PanFS CSI Driver binary
	docker run -it --arch=amd64 -v $(shell pwd):$(shell pwd) -w $(shell pwd) golang:1.24 go build -o build/panfs-csi pkg/csi-plugin/main.go

.PHONNY: build
build: build-driver-image build-dfc-image ## Build both the PanFS CSI Driver and DFC images

.PHONNY: push
push: ## Push the PanFS CSI Driver Docker image and DFC image to the registry
	docker push $(CSIDRIVER_IMAGE)
	@echo "$(GREEN)Successfully pushed image to $(CSIDRIVER_IMAGE)$(RESET)"
	@echo
	docker push $(CSIDFCKMM_IMAGE)
	@echo "$(GREEN)Successfully pushed image to $(CSIDFCKMM_IMAGE)$(RESET)"
	@echo
	@echo "To deploy the driver, run: "
	@echo "  make deploy-driver"
	@echo
	@echo "To reload driver after changes (reload images), run:"
	@echo "  make reload-driver"
	@echo

BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

.PHONY: build-driver-image
build-driver-image: ## Build the PanFS CSI Driver Docker image
	@if [ -z "$(APP_VERSION)" ]; then \
		APP_VERSION=$$(git describe --tags --always --dirty); \
		echo "APP_VERSION is not set. Using git describe: $$APP_VERSION"; \
	else \
		echo "Using provided APP_VERSION: $(APP_VERSION)"; \
	fi

	docker build -t $(CSIDRIVER_IMAGE) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg GIT_COMMIT=$(shell git rev-parse --short HEAD) \
		.

.PHONY: build-dfc-image
build-dfc-image: ## Build the Kernel Module Management Docker image
	docker build -t $(CSIDFCKMM_IMAGE) \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg PANFSPKG_NAME=$(PANFSPKG_NAME) \
		--build-arg KERNEL_FULL_VERSION=$(KERNEL_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg GIT_COMMIT=$(shell git rev-parse --short HEAD) \
		-f dfc/Dockerfile \
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
sanity-check: ## Run static code analysis
	@if [ -z "$(TEST_IMAGE)" ]; then \
		echo "$(RED)Error: TEST_IMAGE is not set$(RESET)"; \
		exit 1; \
	fi

	@if [ -z "$(CSIDRIVER_IMAGE)" ]; then \
		echo "$(RED)Error: CSIDRIVER_IMAGE is not set$(RESET)"; \
		exit 1; \
	fi

	@CSI_TEST_IMAGE=$(TEST_IMAGE) \
	 CSI_IMAGE=$(CSIDRIVER_IMAGE) \
	 docker compose -f tests/csi_sanity/docker-compose.yaml up \
	   --abort-on-container-exit \
	   --exit-code-from sanity_tests

.PHONY: e2e-ns
e2e-ns: ## Create e2e namespace and image pull secret
	@kubectl create ns e2e --dry-run=client -o yaml | kubectl apply -f -
	@echo
	@kubectl get secret -n e2e $(IMAGE_PULL_SECRET_NAME) 2>/dev/null || kubectl create secret docker-registry $(IMAGE_PULL_SECRET_NAME) \
		--docker-server=https://us-central1-docker.pkg.dev \
		--docker-username=_json_key \
		--docker-password='$(shell cat $(REGISTRY_CREDS_FILE))' \
		--docker-email=k8s-artifact-reader@labvirtualization.iam.gserviceaccount.com \
		--namespace=e2e \
		--dry-run=client -o yaml | kubectl apply -f -
	@echo "$(GREEN)Created/Updated image pull secret '$(IMAGE_PULL_SECRET_NAME)' in namespace 'e2e'$(RESET)"
	@echo

.PHONY: e2e
e2e: ## Run e2e tests
	@if kubectl get ns e2e 2>&1 >/dev/null; then \
		echo "$(GREEN)Namespace 'e2e' already exists$(RESET)"; \
	else \
		make e2e-ns; \
	fi
	@echo "Running e2e tests..."
	@if kubectl get jobs -n e2e 2>/dev/null | grep -q Running; then \
		RUNNING_JOB=$$(kubectl get jobs -n e2e | grep Running | awk '{print $$1}' | head -1); \
		if [ -n "$$RUNNING_JOB" ]; then \
			kubectl logs -n e2e job/$$RUNNING_JOB -f; \
		else \
			echo "Error: No running job found"; \
			exit 1; \
		fi; \
	else \
		TIMESTAMP=$$(date +%Y%m%d%H%M%S); \
		JOBNAME=tests-$$TIMESTAMP; \
		kubectl create job --from=cronjob/tests -n e2e $$JOBNAME; \
		if [ $$? -eq 0 ]; then \
			kubectl wait --for=condition=complete --timeout=300s job/$$JOBNAME -n e2e; \
			kubectl logs -n e2e job/$$JOBNAME -f; \
		else \
			echo "Error: Failed to create job $$JOBNAME"; \
			exit 1; \
		fi; \
	fi

.PHONY: e2e-show-last
e2e-show-last:  ## Show report of the last completed e2e job
	@printf "CSI Driver Image: %s\n" $$(kubectl get deploy -n csi-panfs csi-panfs-controller -o jsonpath='{.spec.template.spec.containers[?(@.name=="csi-panfs-plugin")].image}')
	@printf "DFC Module Image: %s\n" $$(kubectl get module -n csi-panfs panfs -o jsonpath='{.spec.moduleLoader.container.kernelMappings[?(@.literal=="$(KERNEL_VERSION)")].containerImage}')
	@echo

	@echo "Showing the last completed e2e job report:"
	@JOBNAME=$(shell kubectl get jobs -n e2e --sort-by=.metadata.creationTimestamp | grep -v Running | tail -1 | awk '{print $$1}'); \
	kubectl get job $$JOBNAME -n e2e -o custom-columns=Name:.metadata.name,Started:.status.startTime,Finished:.status.completionTime,Failed:.status.failed,Succeeded:.status.succeeded; \
	echo; \
	echo "Showing logs of the last completed e2e job '$$JOBNAME' ..."; echo; \
	kubectl logs -n e2e job/$$JOBNAME | tail -80 | sed -n '/Summarizing.*Failures:/,/^$$/p'; \
	echo; \
	kubectl logs -n e2e job/$$JOBNAME | tail -20 | sed -n '/Ran.*Specs.*/,/^$$/p'
	@echo

	@JOBNAME=$(shell kubectl get jobs -n e2e --sort-by=.metadata.creationTimestamp | grep Running | tail -1 | awk '{print $$1}'); \
	if [ -n "$$JOBNAME" ]; then \
		echo "These job(s) are still currently running:"; \
		kubectl get job $$JOBNAME -n e2e -o custom-columns=Name:.metadata.name,Started:.status.startTime; \
	else \
		echo "No running e2e jobs found."; \
	fi

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

.PHONY: deploy-kmm
deploy-csi: deploy-driver deploy-storageclass ## Deploy the complete PanFS CSI solution (driver, DFC module, and storage class)

.PHONY: deploy-driver-ns-prereq-check
REGISTRY_CREDS_FILE ?= unspecified
deploy-driver-ns-prereq-check:
	@if [ "$(REGISTRY_CREDS_FILE)" = "unspecified" ]; then \
		echo "Please set REGISTRY_CREDS_FILE environment variable"; \
		exit 1; \
	fi

.PHONY: deploy-driver-ns-prereq
deploy-driver-ns-prereq: deploy-driver-ns-prereq-check ## Create namespace and image pull secret for the PanFS CSI Driver
	kubectl create namespace csi-panfs --dry-run=client -o yaml | kubectl apply -f -
	@echo "$(GREEN)Created namespace 'csi-panfs'$(RESET)"
	@echo
	kubectl label namespace csi-panfs pod-security.kubernetes.io/enforce=privileged --overwrite
	@echo "$(GREEN)Labeled namespace 'csi-panfs' with pod-security.kubernetes.io/enforce=privileged$(RESET)"
	@echo
	@echo Creating image pull secret '$(IMAGE_PULL_SECRET_NAME)' in namespace 'csi-panfs'...
	@kubectl get secret -n csi-panfs $(IMAGE_PULL_SECRET_NAME) 2>/dev/null || kubectl create secret docker-registry $(IMAGE_PULL_SECRET_NAME) \
		--docker-server=https://us-central1-docker.pkg.dev \
		--docker-username=_json_key \
		--docker-password='$(shell cat $(REGISTRY_CREDS_FILE))' \
		--docker-email=k8s-artifact-reader@labvirtualization.iam.gserviceaccount.com \
		--namespace=csi-panfs \
		--dry-run=client -o yaml | kubectl apply -f -
	@echo "$(GREEN)Created image pull secret '$(IMAGE_PULL_SECRET_NAME)' in namespace 'csi-panfs'$(RESET)"
	@echo

.PHONY: deploy-driver-info deploy-storageclass-info
info: deploy-driver-info deploy-storageclass-info

.PHONY: deploy-driver-info
deploy-driver-info: ## Display information about the PanFS CSI Driver to be installed
	@echo "Driver Image:  $(CSIDRIVER_IMAGE)"
	@echo "DFC/KMM Image: $(CSIDFCKMM_IMAGE)"
	@echo

.PHONY: deploy-driver-with-helm
overrides = $(wildcard charts/panfs/override.yaml)
deploy-driver-with-helm:
	@echo "Deploying PanFS CSI Driver using Helm chart since USE_HELM is set..."
ifneq ($(overrides),)
	helm upgrade --install csi-panfs charts/panfs \
		--namespace csi-panfs \
		--values $(overrides) \
		--wait
else
	@echo $(CSIDFCKMM_IMAGE) | grep -q ':stub'; if [ $$? -eq 0 ]; then \
		helm upgrade --install csi-panfs charts/panfs --namespace csi-panfs \
			--set "imagePullSecrets[0]=$(IMAGE_PULL_SECRET_NAME)" \
			--set "panfsPlugin.image=$(CSIDRIVER_IMAGE)" \
			--set "panfsPlugin.pullPolicy=IfNotPresent" \
			--set "dfcRelease.kernelMappings[0].literal=default" \
			--set "dfcRelease.kernelMappings[0].containerImage=$(CSIDFCKMM_IMAGE)" \
			--set "dfcRelease.pullPolicy=IfNotPresent" \
			--set "panfsKmmModule.enabled=false" \
			--set "seLinux=false"; \
	else \
		helm upgrade --install csi-panfs charts/panfs --namespace csi-panfs \
			--set "imagePullSecrets[0]=$(IMAGE_PULL_SECRET_NAME)" \
			--set "panfsPlugin.image=$(CSIDRIVER_IMAGE)" \
			--set "dfcRelease.kernelMappings[0].literal=$(KERNEL_VERSION)" \
			--set "dfcRelease.kernelMappings[0].containerImage=$(CSIDFCKMM_IMAGE)"; \
	fi
endif
	@echo "$(GREEN)Successfully deployed PanFS CSI Driver$(RESET)"
	@echo
	@helm get values csi-panfs -n csi-panfs
	@echo

.PHONY: deploy-driver-with-manifest
deploy-driver-with-manifest:
	@echo "Deploying PanFS CSI Driver using manifest file..."
	@cat deploy/k8s/csi-driver/template-csi-panfs.yaml | \
	sed 's@^\(  *\)# \(.*\)<IMAGE_PULL_SECRET_NAME.\(.*\)@\1\2$(IMAGE_PULL_SECRET_NAME)\3@' | \
	sed 's@<PANFS_CSI_DRIVER_IMAGE>@$(CSIDRIVER_IMAGE)@g' | \
	sed 's@[^ ]*panfs-csi-driver:.*@$(CSIDRIVER_IMAGE)@g' | \
	sed 's@<PANFS_DFC_IMAGE>@$(CSIDFCKMM_IMAGE)@g' | \
	sed 's@<KERNEL_VERSION>@$(KERNEL_VERSION)@g' | \
	kubectl apply --server-side -f -
	@echo "$(GREEN)Successfully deployed PanFS CSI Driver using manifest file deploy/k8s/csi-panfs-driver.yaml$(RESET)"
	@echo

.PHONY: deploy-driver
deploy-driver: deploy-driver-info ## Deploy PanFS CSI Driver (Includes DFC)
	@if [ -z "$(CSIDRIVER_IMAGE)" ]; then \
		echo "$(RED)ERROR: CSIDRIVER_IMAGE is not set$(RESET)"; \
		printf "USAGE:\n  export CSIDRIVER_IMAGE=...\n  export CSIDFCKMM_IMAGE=...\n  export KERNEL_VERSION=...\n  make deploy-driver\n"; \
		exit 1; \
	fi

	@if [ -z "$(CSIDFCKMM_IMAGE)" ]; then \
		echo "$(RED)ERROR: CSIDFCKMM_IMAGE is not set$(RESET)"; \
		printf "USAGE:\n  export CSIDRIVER_IMAGE=...\n  export CSIDFCKMM_IMAGE=...\n  export KERNEL_VERSION=...\n  make deploy-driver\n"; \
		exit 1; \
	fi

	@if [ -z "$(KERNEL_VERSION)" ]; then \
		echo "$(RED)ERROR: KERNEL_VERSION is not set$(RESET)"; \
		printf "USAGE:\n  export CSIDRIVER_IMAGE=...\n  export CSIDFCKMM_IMAGE=...\n  export KERNEL_VERSION=...\n  make deploy-driver\n"; \
		exit 1; \
	fi

	@if [ "$(USE_HELM)" = "true" ]; then \
		make deploy-driver-with-helm; \
	else \
		make deploy-driver-with-manifest; \
	fi

	@echo "Waiting for the PanFS CSI Controller deployment to be ready..."
	@timeout 60 kubectl -n csi-panfs rollout status deployment csi-panfs-controller

	@echo "Waiting for the PanFS CSI Node daemonset to be ready..."
	@timeout 60 kubectl -n csi-panfs rollout status daemonset csi-panfs-node

	@if kubectl get module panfs -n csi-panfs 2>/dev/null | grep -q panfs; then \
		echo "Waiting for panfs DFC module to fully load..."; \
		while true; do \
			desired=$$(kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.desiredNumber}'); \
			available=$$(kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.availableNumber}'); \
			nodes=$$(kubectl get module panfs -n csi-panfs -o jsonpath='{.status.moduleLoader.nodesMatchingSelectorNumber}'); \
			echo "NODES: $$nodes  LOADED: $$available  DESIRED: $$desired"; \
			if [ "$$desired" = "$$available" ] && [ -n "$$desired" ]; then \
				echo "All modules loaded successfully."; \
				break; \
			fi; \
			sleep 5; \
		done; \
		echo "$(GREEN)PanFS DFC module is fully loaded.$(RESET)"; \
		echo; \
	fi

	@echo "To check the status of the DFC module, run:"
	@echo "  kubectl get module panfs -n csi-panfs"
	@echo "  kubectl get module panfs -n csi-panfs -o custom-columns=NODES:.status.moduleLoader.nodesMatchingSelectorNumber,LOADED:.status.moduleLoader.availableNumber,DESIRED:.status.moduleLoader.desiredNumber"
	@echo

	@echo "To reload driver pods, run the following commands:"
	@echo "  kubectl rollout restart deployment csi-panfs-controller -n csi-panfs"
	@echo "  kubectl rollout restart daemonset csi-panfs-node -n csi-panfs"
	@echo

.PHONY: deploy-storageclass-info
deploy-storageclass-info: ## Display information about the PanFS CSI Storage Class to be deployed
	@echo "Storage Class Name: $(STORAGE_CLASS_NAME)"
	@echo "Set as Default:     $(SET_STORAGECLASS_DEFAULT)"
	@echo

.PHONY: sc
sc: deploy-storageclass ## Alias for deploy-storageclass

.PHONY: deploy-storageclass-with-helm
overrides_sc = $(wildcard charts/storageclass/override.yaml)
deploy-storageclass-with-helm:
	@echo "Deploying PanFS CSI Storage Class '$(STORAGE_CLASS_NAME)' with Helm since USE_HELM is set..."
	helm upgrade --install $(STORAGE_CLASS_NAME) charts/storageclass \
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
	cat deploy/k8s/storage-class/example-csi-panfs-storage-class.yaml | \
	sed 's|<|$$|;s/\([^ ]\)>/\1/;s|is-default-class: "false"|is-default-class: "$(SET_STORAGECLASS_DEFAULT)"|' | \
	sed 's|csi-panfs-storage-class|$(STORAGE_CLASS_NAME)|' | envsubst | kubectl apply --server-side -f -
	@echo "$(GREEN)Successfully deployed PanFS CSI Storage Class using manifest file deploy/k8s/storage-class/example-csi-panfs-storage-class.yaml$(RESET)"
	@echo

.PHONY: deploy-storageclass
deploy-storageclass: deploy-storageclass-info ## Deploy PanFS CSI Storage Class
	@if [ -z "$(STORAGE_CLASS_NAME)" ]; then \
		echo "$(RED)ERROR: STORAGE_CLASS_NAME is not set$(RESET)"; \
		printf "USAGE:\n  export STORAGE_CLASS_NAME=...\n  make deploy-storageclass\n"; \
		exit 1; \
	fi

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
verify: ## Verify the installation of the PanFS CSI Driver and its components
	@echo "CSIDRIVER_IMAGE=$(CSIDRIVER_IMAGE)"
	@echo "CSIDFCKMM_IMAGE=$(CSIDFCKMM_IMAGE)"
	@echo

	@CSIDRIVER_IMAGE=$(CSIDRIVER_IMAGE) \
	CSIDFCKMM_IMAGE=$(CSIDFCKMM_IMAGE) \
	KERNEL_VERSION=$(KERNEL_VERSION) \
	sh tests/helper/lib/validate.sh

## Update Driver and Storage Class:

.PHONY: reload-driver
reload-driver: ## Reload PanFS CSI Driver
	kubectl rollout restart deployment csi-panfs-controller -n csi-panfs
	kubectl rollout restart daemonset csi-panfs-node -n csi-panfs
	@echo

	kubectl -n csi-panfs rollout status deployment csi-panfs-controller
	@echo

	kubectl -n csi-panfs rollout status daemonset csi-panfs-node
	@echo

	kubectl get pods -n csi-panfs -o wide
	@echo

.PHONY: update-driver
update-driver: deploy-driver reload-driver ## Deploy + Reload PanFS CSI Driver

.PHONY: update-storageclass
update-storageclass: deploy-storageclass ## Update PanFS CSI Storage Class

## Uninstall CSI Driver and Storage Class:

.PHONY: uninstall
uninstall: ## Uninstall both the PanFS CSI Driver and Storage Class 
	@if kubectl get pv 2>&1 | grep $(STORAGE_CLASS_NAME) 2>/dev/null; then \
		echo "$(RED)Error: There are still Persistent Volumes using the storage class '$(STORAGE_CLASS_NAME)'. Please delete them before uninstalling the storage class.$(RESET)"; \
		kubectl get pv | grep $(STORAGE_CLASS_NAME); \
		exit 1; \
	fi
	@kubectl delete -f deploy/k8s/csi-driver/template-csi-panfs.yaml --ignore-not-found
	@kubectl delete secret -n csi-panfs -l owner=helm 
	@kubectl delete -f deploy/k8s/storage-class/example-csi-panfs-storage-class.yaml --ignore-not-found
	@kubectl delete secret -n $(STORAGE_CLASS_NAME) -l owner=helm

## Prepare to Release:

.PHONY: manifest-driver
manifest-driver: ## Generate manifests for the PanFS CSI Driver
	@echo "Generating manifests for the PanFS CSI Driver..."
	@mkdir -p deploy/k8s/csi-driver/
	helm template csi-panfs charts/panfs \
		--namespace csi-panfs \
		--set imagePullSecrets[0]="ghcr-docker-registry" \
		--set dfcRelease.kernelMappings[0].literal="4.18.0-553.el8_10.x86_64" \
		--set dfcRelease.kernelMappings[0].containerImage="ghcr.io/panasasinc/panfs-container-storage-interface-oss/panfs-dfc-kmm:4.18.0-553.el8_10.x86_64-11.1.1" \
		--set dfcRelease.kernelMappings[1].literal="4.18.0-553.62.1.el8_10.x86_64" \
		--set dfcRelease.kernelMappings[1].containerImage="ghcr.io/panasasinc/panfs-container-storage-interface-oss/panfs-dfc-kmm:4.18.0-553.62.1.el8_10.x86_64-11.1.1" \
		--set dfcRelease.kernelMappings[2].literal="4.18.0-553.72.1.el8_10.x86_64" \
		--set dfcRelease.kernelMappings[2].containerImage="ghcr.io/panasasinc/panfs-container-storage-interface-oss/panfs-dfc-kmm:4.18.0-553.72.1.el8_10.x86_64-11.1.1" \
		--set dfcRelease.pullPolicy=IfNotPresent | \
		sed 's|^\(.*panfs-dfc-kmm:.*\)$$|\1  # !! Dummy Image|' | \
		grep -v '^# Source:' > deploy/k8s/csi-driver/example-csi-panfs.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs | grep -v '^# Source:' > deploy/k8s/csi-driver/template-csi-panfs.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set seLinux=false | grep -v '^# Source:' > deploy/k8s/csi-driver/template-csi-panfs-without-selinux.yaml
	helm template csi-panfs charts/panfs --namespace csi-panfs --set panfsKmmModule.enabled=false | grep -v '^# Source:' > deploy/k8s/csi-driver/template-csi-panfs-without-kmm.yaml

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
		--set realm.privateKeyPassphrase="<REALM_PRIVATE_KEY_PASSPHRASE>" | \
		sed 's|csi-panfs-storage-class-name|<STORAGE_CLASS_NAME>|' | \
		grep -v '^# Source:' > deploy/k8s/storage-class/template-secret-in-dedicated-ns.yaml
	
	helm template csi-panfs-storage-class-name charts/storageclass \
		--namespace csi-panfs \
		--set setAsDefaultStorageClass=false \
		--set realm.address="<REALM_ADDRESS>" \
		--set realm.username="<REALM_USERNAME>" \
		--set realm.password="<REALM_PASSWORD>" \
		--set realm.privateKey="<REALM_PRIVATE_KEY>" \
		--set realm.privateKeyPassphrase="<REALM_PRIVATE_KEY_PASSPHRASE>" | \
		sed 's|csi-panfs-storage-class-name|<STORAGE_CLASS_NAME>|' | \
		grep -v '^# Source:' | \
		sed 's|csi-panfs|<CSI_NAMESPACE>|' > deploy/k8s/storage-class/template-secret-in-driver-ns.yaml

.PHONY: manifests
manifests: manifest-driver manifest-storageclass ## Generate manifests for the PanFS CSI Driver and Storage Class
	helm-docs
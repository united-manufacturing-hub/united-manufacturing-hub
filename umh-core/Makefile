# Copyright 2025 UMH Systems GmbH
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

# Copyright 2025 UMH Systems GmbH
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

.PHONY: build run clean test

IMAGE_NAME = umh-core
TAG = latest

GINKGO_VERSION = v2.23.3
NILWAY_VERSION = latest
# Default to the architecture of the host
TARGETARCH := $(shell go env GOARCH)

# Default values for build arguments
S6_OVERLAY_VERSION = 3.2.0.2
BENTHOS_UMH_VERSION = 0.6.2
REDPANDA_VERSION = 24.3.8

# Note: Redpanda allocates 2GB of memory per core.
# Additionally 1.5GB (or 7% of the total memory, whichever is greater) are required to be available, after this allocation to make seastar happy.
# If you change this, you need to update the integration_test.go file.
MEMORY = 4096m
CPUS = 2

# Determine version based on git
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")

install:
	go install go.uber.org/nilaway/cmd/nilaway@$(NILWAY_VERSION)
	go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)
	go install github.com/evilmartians/lefthook@latest

download-docker-binaries:
	@mkdir -p .docker-cache/s6-overlay .docker-cache/benthos .docker-cache/redpanda
	@echo "Downloading binaries with caching..."
	
	@# Check and update s6-overlay version
	@if [ ! -f .docker-cache/s6-overlay/version ] || [ "$$(cat .docker-cache/s6-overlay/version)" != "$(S6_OVERLAY_VERSION)" ]; then \
		rm -f .docker-cache/s6-overlay/*.tar.xz; \
		echo "$(S6_OVERLAY_VERSION)" > .docker-cache/s6-overlay/version; \
		wget -O .docker-cache/s6-overlay/s6-overlay-noarch.tar.xz "https://github.com/just-containers/s6-overlay/releases/download/v$(S6_OVERLAY_VERSION)/s6-overlay-noarch.tar.xz"; \
		wget -O .docker-cache/s6-overlay/syslogd-overlay-noarch.tar.xz "https://github.com/just-containers/s6-overlay/releases/download/v$(S6_OVERLAY_VERSION)/syslogd-overlay-noarch.tar.xz"; \
		wget -O .docker-cache/s6-overlay/s6-overlay-x86_64.tar.xz "https://github.com/just-containers/s6-overlay/releases/download/v$(S6_OVERLAY_VERSION)/s6-overlay-x86_64.tar.xz"; \
		wget -O .docker-cache/s6-overlay/s6-overlay-aarch64.tar.xz "https://github.com/just-containers/s6-overlay/releases/download/v$(S6_OVERLAY_VERSION)/s6-overlay-aarch64.tar.xz"; \
	fi
	
	@# Check and update Benthos version
	@if [ ! -f .docker-cache/benthos/version ] || [ "$$(cat .docker-cache/benthos/version)" != "$(BENTHOS_UMH_VERSION)" ]; then \
		rm -f .docker-cache/benthos/benthos-linux-*; \
		echo "$(BENTHOS_UMH_VERSION)" > .docker-cache/benthos/version; \
		wget -O .docker-cache/benthos/benthos-linux-amd64 "https://github.com/united-manufacturing-hub/benthos-umh/releases/download/v$(BENTHOS_UMH_VERSION)/benthos-linux-amd64"; \
		wget -O .docker-cache/benthos/benthos-linux-arm64 "https://github.com/united-manufacturing-hub/benthos-umh/releases/download/v$(BENTHOS_UMH_VERSION)/benthos-linux-arm64"; \
	fi
	
	@# Check and update Redpanda version
	@if [ ! -f .docker-cache/redpanda/version ] || [ "$$(cat .docker-cache/redpanda/version)" != "$(REDPANDA_VERSION)" ]; then \
		rm -rf .docker-cache/redpanda/redpanda-*.tar.gz .docker-cache/redpanda/amd64 .docker-cache/redpanda/arm64; \
		echo "$(REDPANDA_VERSION)" > .docker-cache/redpanda/version; \
		mkdir -p .docker-cache/redpanda/amd64 .docker-cache/redpanda/arm64; \
		echo "Pulling redpandadata/redpanda:$(REDPANDA_VERSION) for amd64..."; \
		docker pull --platform linux/amd64 redpandadata/redpanda:v$(REDPANDA_VERSION); \
		CONTAINER_AMD64=$$(docker create --platform linux/amd64 redpandadata/redpanda:v$(REDPANDA_VERSION)); \
		docker cp $$CONTAINER_AMD64:/opt/redpanda .docker-cache/redpanda/amd64/; \
		docker rm -v $$CONTAINER_AMD64; \
		echo "Pulling redpandadata/redpanda:$(REDPANDA_VERSION) for arm64..."; \
		docker pull --platform linux/arm64 redpandadata/redpanda:v$(REDPANDA_VERSION); \
		CONTAINER_ARM64=$$(docker create --platform linux/arm64 redpandadata/redpanda:v$(REDPANDA_VERSION)); \
		docker cp $$CONTAINER_ARM64:/opt/redpanda .docker-cache/redpanda/arm64/; \
		docker rm -v $$CONTAINER_ARM64; \
	fi
	
	@# Verify all required files exist
	@echo "Verifying downloaded files..."
	@for f in \
		.docker-cache/s6-overlay/s6-overlay-noarch.tar.xz \
		.docker-cache/s6-overlay/s6-overlay-x86_64.tar.xz \
		.docker-cache/s6-overlay/s6-overlay-aarch64.tar.xz \
		.docker-cache/s6-overlay/syslogd-overlay-noarch.tar.xz \
		.docker-cache/benthos/benthos-linux-amd64 \
		.docker-cache/benthos/benthos-linux-arm64; \
	do \
		if [ ! -f $$f ]; then \
			echo "Missing required file: $$f"; \
			rm -rf .docker-cache/redpanda; \
			rm -rf .docker-cache/benthos; \
			rm -rf .docker-cache/s6-overlay; \
			exit 1; \
		fi; \
	done
	
	@# Verify redpanda directories exist
	@if [ ! -d .docker-cache/redpanda/amd64/redpanda ] || [ ! -d .docker-cache/redpanda/arm64/redpanda ]; then \
		echo "Missing required Redpanda directories"; \
		rm -rf .docker-cache/redpanda; \
		rm -rf .docker-cache/benthos; \
		rm -rf .docker-cache/s6-overlay; \
		exit 1; \
	fi

	@# Ensure .docker-cache is in the .gitignore file
	@if ! grep -q ".docker-cache" .gitignore; then \
		echo ".docker-cache" >> .gitignore; \
	fi

	@echo "All required files are present and up to date"

build: download-docker-binaries
	docker buildx build \
		--platform linux/$(TARGETARCH) \
		--build-arg S6_OVERLAY_VERSION=$(S6_OVERLAY_VERSION) \
		--build-arg BENTHOS_UMH_VERSION=$(BENTHOS_UMH_VERSION) \
		--build-arg REDPANDA_VERSION=$(REDPANDA_VERSION) \
		--build-arg APP_VERSION=$(VERSION) \
		--build-arg TARGETARCH=$(TARGETARCH) \
		-t $(IMAGE_NAME):$(TAG) \
		--progress=plain \
		-f Dockerfile .
    
# build-debug: launches delve inside the container.
# This allows you to use vscode's "Connect to umh-core" feature and use breakpoints, etc inside the container.
# To use this, modify one of the build-<type> targets to use this target instead of build.
# You also need to add -p 40000:40000 to the docker run command to expose the Delve port.
build-debug: download-docker-binaries
	docker buildx build \
		--platform linux/$(TARGETARCH) \
		--build-arg S6_OVERLAY_VERSION=$(S6_OVERLAY_VERSION) \
		--build-arg BENTHOS_UMH_VERSION=$(BENTHOS_UMH_VERSION) \
		--build-arg REDPANDA_VERSION=$(REDPANDA_VERSION) \
		--build-arg APP_VERSION=$(VERSION) \
		--build-arg DEBUG=true \
		--progress=plain \
		-t $(IMAGE_NAME):$(TAG)-debug \
		-f Dockerfile .
    
stop-all-containers:
	docker stop $(docker ps -a -q) || true
	docker rm $(docker ps -a -q) || true

clean:
	docker stop $(IMAGE_NAME) || true
	docker rm $(IMAGE_NAME) || true
	docker rmi $(IMAGE_NAME):$(TAG) || true

# Clean up Docker build cache and unused objects for CI environments & stop all containers
cleanup-ci: clean stop-all-containers
	docker stop $(docker ps -a -q) || true
	docker rm $(docker ps -a -q) || true
	docker builder prune -f
	docker system prune -f --volumes

vet:
	go vet -tags=test ./...

nilaway:
	nilaway -include-pkgs="github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core" -tags=test ./...

test: vet nilaway
	go test -v -tags=test ./...

unit-test: vet nilaway
	ginkgo -r -v --tags=test --label-filter='!integration' --fail-fast ./...


integration-test: stop-all-pods vet nilaway
	ginkgo -r -v --tags=test --label-filter='integration' --fail-fast ./...

# integration-test-ci: Runs integration tests in CI environments with proper version tagging
# - Injects VERSION via ldflags into AppVersion for proper error identification in Sentry
# - CI test failures SHOULD be reported to Sentry (unlike local dev failures)
# - Helps identify flaky tests by consistently tracking failures in CI pipelines
integration-test-ci: vet nilaway
	VERSION=$(VERSION) ginkgo -r -v --tags=test --label-filter='integration' --fail-fast -ldflags="-X 'github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/version.AppVersion=$(VERSION)'" ./...

update-go-dependencies:
	go get -u ./...
	go mod tidy
	go mod vendor

test-empty: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-empty.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-clean-install: clean build
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e API_URL=https://management.umh.app/api \
		-e RELEASE_CHANNEL=enterprise \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-hello: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)

test-comm: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-comm.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e AUTH_TOKEN=$(AUTH_TOKEN) \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)

test-broken: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-broken.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-benthos: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-benthos.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e LOGGING_LEVEL=DEBUG \
		-p 8081:8080 \
		-p 4195:4195 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)


test-redpanda: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-redpanda.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e LOGGING_LEVEL=DEBUG \
		-p 8081:8080 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-dfc: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-dataflow.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e LOGGING_LEVEL=DEBUG \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-debug: clean build-debug
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-benthos-broken.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)-debug

pod-shell:
	docker exec -it $(IMAGE_NAME) /bin/bash

stop-all-pods:
	# Stops all pods starting with $(IMAGE_NAME)
	docker ps -q --filter "name=$(IMAGE_NAME)*" | xargs -r docker stop
	# Remove all pods starting with $(IMAGE_NAME) (also show stopped pods)
	docker ps -a -q --filter "name=$(IMAGE_NAME)*" | xargs -r docker rm

push:
	@echo "Pushing $(IMAGE_NAME):$(TAG) to registry"
	@if [ -z "$(REGISTRY)" ]; then \
		echo "Error: REGISTRY environment variable is not set"; \
		exit 1; \
	fi
	docker tag $(IMAGE_NAME):$(TAG) $(REGISTRY)/$(IMAGE_NAME):$(TAG)
	docker push $(REGISTRY)/$(IMAGE_NAME):$(TAG)

# Push with platform-specific tag (used in CI)
push-platform:
	@echo "Pushing $(IMAGE_NAME):$(TAG) for $(TARGETARCH) to registry"
	@if [ -z "$(REGISTRY)" ]; then \
		echo "Error: REGISTRY environment variable is not set"; \
		exit 1; \
	fi
	@if [ -z "$(TAG)" ]; then \
		echo "Error: TAG environment variable is not set"; \
		exit 1; \
	fi
	@PLATFORM_SHORT="$(TARGETARCH)"; \
	PLATFORM_TAG="$(TAG)-$${PLATFORM_SHORT}"; \
	echo "Tagging $(IMAGE_NAME):$(TAG) as $(REGISTRY)/$(IMAGE_NAME):$${PLATFORM_TAG}"; \
	docker tag $(IMAGE_NAME):$(TAG) $(REGISTRY)/$(IMAGE_NAME):$${PLATFORM_TAG}; \
	echo "Pushing $(REGISTRY)/$(IMAGE_NAME):$${PLATFORM_TAG}"; \
	docker push $(REGISTRY)/$(IMAGE_NAME):$${PLATFORM_TAG}

# Build and push in one command (for CI)
build-push: build push-platform
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

.PHONY: build run clean test help test-graphql

# Show available commands and common usage patterns
help: ## Display available make targets and usage information
	@echo "🏭 UMH Core - Build & Test Commands"
	@echo "=================================="
	@echo ""
	@echo "📊 QUICK START - GraphQL API Demo:"
	@echo "   make test-graphql    # Start GraphQL server with simulator data (port 8090)"
	@echo ""
	@echo "🔨 BUILD COMMANDS:"
	@echo "   make build          # Build Docker image"
	@echo "   make build-debug    # Build with debug support"
	@echo "   make build-pprof    # Build with profiling support"
	@echo ""
	@echo "🧪 TEST COMMANDS:"
	@echo "   make test-graphql      # GraphQL API with real-time simulator data (recommended)"
	@echo "   make test-dfc          # Data Flow Components"
	@echo "   make test-redpanda     # Redpanda message broker"
	@echo "   make test-connection   # Connection services"
	@echo "   make test-debug        # Debug configuration"
	@echo ""
	@echo "🛠️  UTILITY COMMANDS:"
	@echo "   make pod-shell      # Shell into running container"
	@echo "   make stop-all-pods  # Stop all UMH Core containers"
	@echo "   make dump-metrics   # Save metrics to file"
	@echo "   make cleanup-all    # Clean all Docker resources"
	@echo ""
	@echo "💡 NOTES:"
	@echo "   • All test commands run in Docker containers"
	@echo "   • Containers auto-cleanup on restart"
	@echo "   • Check 'docker logs umh-core' for troubleshooting"
	@echo "   • GraphQL playground available at http://localhost:8090/"
	@echo "   • Full GraphQL docs: pkg/communicator/graphql/README.md"
	@echo ""

IMAGE_NAME = umh-core
TAG = latest

# Default to the architecture of the host
TARGETARCH := $(shell go env GOARCH)

# Default values for go tools
GINKGO_VERSION = v2.23.4
NILWAY_VERSION = latest
GOLANGCI_LINT_VERSION = v2.1.6
DLV_VERSION = v1.25.0

# Default values for build arguments
GOLANG_VERSION = 1.24.4
ALPINE_VERSION = 3.21
NMAP_VERSION = 7.95-r1
S6_OVERLAY_VERSION = 3.2.0.2
REDPANDA_VERSION = 24.3.8
## Benthos version is a tag or a sha-<commit hash> (e.g. 0.9.4 or sha-a51a685) to be used in the docker image
BENTHOS_UMH_VERSION = 0.10.0-alpha.1
BUF_VERSION = 1.55.1
PROTOC_GEN_GO_VERSION = 1.36.6


# Note: Redpanda allocates 2GB of memory per core.
# Additionally 1.5GB (or 7% of the total memory, whichever is greater) are required to be available, after this allocation to make seastar happy.
# If you change this, you need to update the integration_test.go file.
MEMORY = 4096m
CPUS = 2

# Determine version based on git
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")

install:
	sudo chown vscode:vscode data || true
	sudo chown vscode:vscode tmp || true
	go install go.uber.org/nilaway/cmd/nilaway@$(NILWAY_VERSION)
	go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)
	go install github.com/evilmartians/lefthook@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

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
		echo "Extracting Benthos binary from Docker container for version $(BENTHOS_UMH_VERSION)..."; \
		echo "Pulling ghcr.io/united-manufacturing-hub/benthos-umh:$(BENTHOS_UMH_VERSION) for amd64..."; \
		docker pull --platform linux/amd64 ghcr.io/united-manufacturing-hub/benthos-umh:$(BENTHOS_UMH_VERSION); \
		CONTAINER_AMD64=$$(docker create --platform linux/amd64 ghcr.io/united-manufacturing-hub/benthos-umh:$(BENTHOS_UMH_VERSION)); \
		docker cp $$CONTAINER_AMD64:/benthos .docker-cache/benthos/benthos-linux-amd64; \
		docker rm -v $$CONTAINER_AMD64; \
		echo "Pulling ghcr.io/united-manufacturing-hub/benthos-umh:$(BENTHOS_UMH_VERSION) for arm64..."; \
		docker pull --platform linux/arm64 ghcr.io/united-manufacturing-hub/benthos-umh:$(BENTHOS_UMH_VERSION); \
		CONTAINER_ARM64=$$(docker create --platform linux/arm64 ghcr.io/united-manufacturing-hub/benthos-umh:$(BENTHOS_UMH_VERSION)); \
		docker cp $$CONTAINER_ARM64:/benthos .docker-cache/benthos/benthos-linux-arm64; \
		docker rm -v $$CONTAINER_ARM64; \
		if [ ! -s .docker-cache/benthos/benthos-linux-amd64 ] || [ ! -s .docker-cache/benthos/benthos-linux-arm64 ]; then \
			echo "Error: Extracted Benthos binaries are empty or missing"; \
			rm -f .docker-cache/benthos/benthos-linux-amd64 .docker-cache/benthos/benthos-linux-arm64; \
			rm -f .docker-cache/benthos/version; \
			exit 1; \
		fi; \
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

.PHONY: build build-pprof build-debug
COMMON_ARGS = \
	--platform linux/$(TARGETARCH) \
	--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
	--build-arg ALPINE_VERSION=$(ALPINE_VERSION) \
	--build-arg NMAP_VERSION=$(NMAP_VERSION) \
	--build-arg S6_OVERLAY_VERSION=$(S6_OVERLAY_VERSION) \
	--build-arg BENTHOS_UMH_VERSION=$(BENTHOS_UMH_VERSION) \
	--build-arg REDPANDA_VERSION=$(REDPANDA_VERSION) \
	--build-arg DLV_VERSION=$(DLV_VERSION) \
	--build-arg APP_VERSION=$(VERSION) \
	--build-arg TARGETARCH=$(TARGETARCH)

define DOCKER_BUILD
	docker buildx build $(COMMON_ARGS) $(EXTRA_ARGS) \
		--progress=plain --load -f Dockerfile \
		-t $(IMAGE_NAME):$(TAG) .
endef

build:
build-pprof:       EXTRA_ARGS := --build-arg PPROF=true
build-debug:       EXTRA_ARGS := --build-arg DEBUG=true

# Refresh cache and build docker binaries
build build-pprof build-debug: build-protobuf download-docker-binaries
	$(call DOCKER_BUILD)

FLAVOR ?=                         # empty → normal build
BUILD_TARGET = $(if $(FLAVOR),build-$(FLAVOR),build)

stop-and-remove-umh-core-latest:
	docker stop $(IMAGE_NAME) || true
	docker rm $(IMAGE_NAME) || true
	docker rmi $(IMAGE_NAME):$(TAG) || true

# Cleanup Docker, Go build cache and test cache
cleanup-ci:
	echo "Docker cleanup - removing everything"
	echo " - Running containers:"
	docker ps -a || true
	echo " - Stopping all containers"
	docker stop $$(docker ps -aq) 2>/dev/null || true
	echo " - Removing all containers"
	docker rm $$(docker ps -aq) 2>/dev/null || true
	echo " - Removing all images"
	docker rmi $$(docker images -q) 2>/dev/null || true
	echo " - Pruning Docker build cache"
	docker builder prune -a -f || true
	echo " - Pruning Docker system"
	docker system prune -a -f --volumes || true
	echo " - Cleaning Go build cache"
	go clean -cache -modcache || true
	echo " - Cleaning Go test cache"
	go clean -testcache || true

cleanup-tmp:
	rm -rf ./tmp/*

cleanup-data:
	rm -rf ./data/*

cleanup-all: cleanup-ci cleanup-tmp cleanup-data

vet:
	go vet -tags=test ./...

lint:
	golangci-lint run ./...

nilaway:
	# See lefthook.yaml for more details
	nilaway \
		-exclude-pkgs="github.com/prometheus/prometheus/util/textparse,github.com/onsi/ginkgo/v2,go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp,github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda" \
		-exclude-errors-in-files="integration/" \
		./...

test: vet nilaway lint
	go test -v -tags=test ./...

unit-test: vet nilaway lint
	ginkgo -r -v --tags=test --label-filter='!integration && !redpanda-extended && !tls' --fail-fast ./...

benchmark:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

benchmark-config:
	@echo "Running config package benchmarks..."
	cd pkg/config && go test -bench=. -benchmem

# More specific benchmarks can be added as needed
benchmark-s6:
	@echo "Running S6 benchmarks..."
	cd pkg/service/s6 && go test -bench=. -benchmem

# More specific benchmarks can be added as needed
benchmark-s6-parse-log:
	@echo "Running S6 parseLogLine benchmarks..."
	cd pkg/service/s6 && go test -bench=ParseLogLine -benchmem

benchmark-benthos-metrics:
	@echo "Running Benthos metrics benchmarks..."
	cd pkg/service/benthos_monitor && go test -bench=. -benchmem

benchmark-redpanda-metrics:
	@echo "Running Redpanda metrics benchmarks..."
	cd pkg/service/redpanda_monitor && go test -bench=. -benchmem

benchmark-s6-time:
	@echo "Running S6 time benchmarks..."
	cd pkg/service/s6 && go test -bench=. -benchmem

benchmark-encoding:
	@echo "Running encoding benchmarks..."
	cd pkg/communicator/pkg/encoding && go test -bench=. -benchmem -benchtime=10s -count=1

integration-test: stop-all-pods vet nilaway lint

# Meta target to setup the environment for the tests
pre-test: build-protobuf stop-all-pods vet nilaway lint

# Meta target to setup the environment for the tests in CI environments
pre-test-ci: vet lint

integration-test: pre-test
	ginkgo -r -v --tags=test --label-filter='integration' --fail-fast ./...

redpanda-extended-test: pre-test
	ginkgo -r -v --tags=test --label-filter='redpanda-extended' --fail-fast --timeout=1.5h ./...

badssl-test: pre-test
	ginkgo -r -v --tags=test --label-filter='tls' --fail-fast ./...

# integration-test-ci: Runs integration tests in CI environments with proper version tagging
# - Injects VERSION via ldflags into AppVersion for proper error identification in Sentry
# - CI test failures SHOULD be reported to Sentry (unlike local dev failures)
# - Helps identify flaky tests by consistently tracking failures in CI pipelines
integration-test-ci: pre-test-ci
	VERSION=$(VERSION) ginkgo -r -v --tags=test --label-filter='integration' --fail-fast -ldflags="-X 'github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/version.AppVersion=$(VERSION)'" ./...

update-go-dependencies:
	go get -u ./...
	go mod tidy
	go mod vendor

# Print version variables for use in other scripts/tools
print-versions:
	@echo "GOLANG_VERSION=$(GOLANG_VERSION)"
	@echo "ALPINE_VERSION=$(ALPINE_VERSION)"
	@echo "NMAP_VERSION=$(NMAP_VERSION)"
	@echo "S6_OVERLAY_VERSION=$(S6_OVERLAY_VERSION)"
	@echo "BENTHOS_UMH_VERSION=$(BENTHOS_UMH_VERSION)"
	@echo "REDPANDA_VERSION=$(REDPANDA_VERSION)"
	@echo "DLV_VERSION=$(DLV_VERSION)"

# Print the Go version
## Used by go-analysis.yml
print-golang-version:
	@echo "$(GOLANG_VERSION)"

## Install buf for protobuf generation
install-buf:
	@echo "Installing buf via Go..."
	@go install github.com/bufbuild/buf/cmd/buf@v$(BUF_VERSION)
	@echo "Buf installed via Go"

## Generate go files from protobuf for topic browser
build-protobuf: install-buf
	@echo "Installing protobuf dependencies..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@v$(PROTOC_GEN_GO_VERSION)
	@echo "Generating Go from protobuf..."
	@mkdir -p pkg/communicator/models/topicbrowser/pb
	@rm -f pkg/communicator/models/topicbrowser/pb/topic_browser_data.pb.go || true
	buf generate \
		--template='{"version":"v1","plugins":[{"plugin":"go","out":"pkg/communicator/models/topicbrowser/pb","opt":["paths=source_relative"]}]}' \
		pkg/communicator/models/topicbrowser/proto
	@echo "Go protobuf files generated successfully."

###############################################################################
# choose build flavour                                                        #
#   make test-dfc              → uses normal  image (build)                  #
#   make test-dfc FLAVOR=pprof → uses pprof   image (build-pprof)            #
#   make test-dfc FLAVOR=debug → uses delve   image (build-debug)            #
###############################################################################
# To extract pprof data use:
# http://localhost:6060/debug/pprof/profile?seconds=20
# And open it with flamegraph.com

test-empty: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-empty.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-clean-install: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e API_URL=https://management.umh.app/api \
		-e RELEASE_CHANNEL=enterprise \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-hello: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)

test-comm: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-comm.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e AUTH_TOKEN=$(AUTH_TOKEN) \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)

test-broken: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-broken.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-benthos: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-benthos.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e LOGGING_LEVEL=DEBUG \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-benthos-single: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-benthos-single.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e LOGGING_LEVEL=DEBUG \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-redpanda: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-redpanda.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e LOGGING_LEVEL=DEBUG \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-dfc: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-dataflow.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)


test-protocolconverter: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-protocolconverter.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)

test-connection: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-connection.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)

test-benthos-monitor: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-benthos-monitor.yaml $(PWD)/data/config.yaml
	docker run \
		--name umh-core-staging \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		ghcr.io/united-manufacturing-hub/united-manufacturing-hub/umh-core:staging

# GraphQL API Demo: Complete GraphQL server with real-time simulator data
# 
# USAGE: cd umh-core && make test-graphql
# 
# This target starts UMH Core with GraphQL API enabled and built-in simulator providing real data.
# Perfect for quick testing and development without requiring MQTT/Redis infrastructure.
#
# FEATURES:
# - GraphQL server on port 8090 with Gin framework
# - Real-time UNS topics from built-in simulator (enabled by default in example)
# - GraphiQL Playground for interactive querying
# - CORS enabled for web development
# - No external dependencies required
# - To disable simulator: set agent.simulator=false in config.yaml
#
# ENDPOINTS:
# - http://localhost:8090/graphql    - GraphQL API endpoint
# - http://localhost:8090/           - GraphiQL Playground (interactive UI)
# - http://localhost:8081/metrics    - Prometheus metrics
#
# SAMPLE QUERIES:
# - All topics: { topics { topic metadata { key value } } }
# - With events: { topics { topic lastEvent { ... on TimeSeriesEvent { producedAt stringValue } } } }
# - Filtered:   { topics(filter: {text: "topic1"}) { topic } }
# - Limited:    { topics(limit: 3) { topic } }
# - Single:     { topic(topic: "uns.topic1") { topic } }
#
# TROUBLESHOOTING:
# - If GraphQL server doesn't start, check internal logs: /data/logs/umh-core/current
# - Look for "Starting GraphQL server on port 8090" in logs
# - Ensure ports 8090 and 8081 are not in use
# - Container must have config.yaml with graphql.enabled=true
# - Example config has simulator=true for immediate demo data
# - Set agent.simulator=false to disable simulator in production
#
# TECHNICAL NOTES:
# - Built-in simulator generates realistic UNS topics with proper structure
# - Simulator automatically populates topic browser cache every second
# - GraphQL schema supports both time series and relational events
# - Server uses graceful shutdown with 3-second timeout
# - See pkg/communicator/graphql/README.md for detailed documentation
test-graphql: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-graphql.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e LOGGING_LEVEL=DEBUG \
		-p 8081:8080 \
		-p 8090:8090 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

test-debug: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-benthos-broken.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)

test-allow-insecure-tls: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-empty.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		-e LOGGING_LEVEL=DEBUG \
		-e ALLOW_INSECURE_TLS=true \
		$(IMAGE_NAME):$(TAG)

# This run configuration allows easy start without overwriting the config.yaml
test-no-copy: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e LOGGING_LEVEL=DEBUG \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		-p 8090:8090 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)

# This test runs the UMH Core instance with a proxy (on the host machine) to the backend.
# On the host install mitmproxy (for example via brew install mitmproxy)
# Then run mitmweb --listen-port 6060
test-mitm: stop-and-remove-umh-core-latest $(BUILD_TARGET)
	@mkdir -p $(PWD)/data
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e LOGGING_LEVEL=DEBUG \
		-e HTTP_PROXY=http://host.docker.internal:6060 \
		-e HTTPS_PROXY=https://host.docker.internal:6060 \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)


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

# Retrieve metrics from the UMH Core instance and save them to a file
dump-metrics:
	curl -s http://localhost:8081/metrics > metrics.txt

run: 
	@echo "Running $(IMAGE_NAME):$(TAG)"
	docker rm -f $(IMAGE_NAME)
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-p 6060:6060 \
		-p 40000:40000 \
		--memory $(MEMORY) \
		--cpus $(CPUS) \
		$(IMAGE_NAME):$(TAG)
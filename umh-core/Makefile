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

# Determine version based on git
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")

install:
	go install go.uber.org/nilaway/cmd/nilaway@$(NILWAY_VERSION)
	go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

build:
	docker build --platform=$$(if [ "$$(uname -m)" = "arm64" ]; then echo "linux/arm64"; else echo "linux/amd64"; fi) \
		--build-arg TARGETARCH=$$(if [ "$$(uname -m)" = "arm64" ]; then echo "arm64"; else echo "amd64"; fi) \
		--build-arg APP_VERSION=$(VERSION) \
		-t $(IMAGE_NAME):$(TAG) -f Dockerfile .
	
clean:
	docker stop $(IMAGE_NAME) || true
	docker rm $(IMAGE_NAME) || true
	docker rmi $(IMAGE_NAME):$(TAG) || true

# Clean up Docker build cache and unused objects for CI environments
cleanup-ci: clean
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


integration-test: vet nilaway
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
		$(IMAGE_NAME):$(TAG)

test-clean-install: clean build
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-e API_URL=https://management.umh.app/api \
		-e RELEASE_CHANNEL=enterprise \
		$(IMAGE_NAME):$(TAG)

test-hello: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
		-e LOGGING_LEVEL=DEBUG \
		$(IMAGE_NAME):$(TAG)

test-comm: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-comm.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		$(IMAGE_NAME):$(TAG)

test-broken: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config-broken.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
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
		$(IMAGE_NAME):$(TAG)

test-benthos-tmp: clean build
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/examples/example-config-benthos.yaml:/data/config.yaml \
		-e LOGGING_LEVEL=DEBUG \
		-p 8081:8080 \
		-p 4195:4195 \
		$(IMAGE_NAME):$(TAG)

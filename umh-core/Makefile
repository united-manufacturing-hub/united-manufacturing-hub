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

build:
	docker build --platform=$$(if [ "$$(uname -m)" = "arm64" ]; then echo "linux/arm64"; else echo "linux/amd64"; fi) \
		--build-arg TARGETARCH=$$(if [ "$$(uname -m)" = "arm64" ]; then echo "arm64"; else echo "amd64"; fi) \
		-t $(IMAGE_NAME):$(TAG) -f Dockerfile .
	
clean:
	docker stop $(IMAGE_NAME) || true
	docker rm $(IMAGE_NAME) || true
	docker rmi $(IMAGE_NAME):$(TAG) || true

vet:
	go vet ./...

test: vet
	go test -v ./...

unit-test: vet
	ginkgo -r -v --label-filter='!integration' --fail-fast ./...

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

test-hello: clean build
	@mkdir -p $(PWD)/data
	@cp $(PWD)/examples/example-config.yaml $(PWD)/data/config.yaml
	docker run \
		--name $(IMAGE_NAME) \
		-v $(PWD)/data:/data \
		-p 8081:8080 \
		-p 4195:4195 \
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

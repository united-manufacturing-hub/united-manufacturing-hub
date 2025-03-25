.PHONY: build run clean test

IMAGE_NAME = umh-core
TAG = latest

build:
	docker build -t $(IMAGE_NAME):$(TAG) -f Dockerfile .
	
clean:
	docker stop $(IMAGE_NAME) || true
	docker rm $(IMAGE_NAME) || true
	docker rmi $(IMAGE_NAME):$(TAG) || true

test:
	go vet ./...
	go test -v ./...

unit-test:
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

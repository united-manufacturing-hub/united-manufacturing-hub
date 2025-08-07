tidy:
	go mod tidy

fmt: tidy
	go fmt ./...

lint: fmt
	golangci-lint run ./...

build: lint
	docker build -t s6-rc-poc .

run: build
	docker run -v $(PWD)/data:/data s6-rc-poc

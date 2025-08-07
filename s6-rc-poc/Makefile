tidy:
	go mod tidy

fmt: tidy
	go fmt ./...

lint: fmt
	go vet ./...
	golangci-lint run ./...

build: lint
	docker build -t s6-rc-poc .

run: build
	sudo rm -rf $(PWD)/data
	mkdir -p $(PWD)/data/logs/s6-rc-poc
	chmod -R 777 $(PWD)/data
	docker run -v $(PWD)/data:/data s6-rc-poc

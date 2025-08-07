tidy:
	go mod tidy

lint: tidy
	golangci-lint run ./...

build: lint
	docker build -t s6-rc-poc .

run: build
	docker run s6-rc-poc

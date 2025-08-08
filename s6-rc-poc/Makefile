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
	docker rm -f s6-rc-poc 2>/dev/null || true
	docker run --name s6-rc-poc -v $(PWD)/data:/data -v $(PWD)/config.yaml:/config.yaml s6-rc-poc

test: lint
	go test -race ./...

pod-shell:
	docker exec -it s6-rc-poc /bin/sh 
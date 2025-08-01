.PHONY: build clean test run-example

BINARY_NAME=loganalyzer
GO=go

build:
	$(GO) build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)

test:
	$(GO) test -v ./...

run-example:
	./$(BINARY_NAME) -f ../../data/logs/umh-core/current -i

install:
	$(GO) install

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...
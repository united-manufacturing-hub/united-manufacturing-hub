REVIEWDOG_ARG ?= -diff="git diff master"

LINTERS=\
	github.com/securego/gosec/v2/cmd/gosec \
  github.com/reviewdog/reviewdog/cmd/reviewdog

.PHONY: all
all: test install-linters reviewdog-local

.PHONY: install-linters
install-linters:
	@for tool in $(LINTERS) ; do \
		echo "Installing $$tool" ; \
		go get -u $$tool; \
	done
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin latest

.PHONY: test
test:
	go mod tidy
	go test -v -race ./...

.PHONY: reviewdog-ci
reviewdog-ci:
	reviewdog -conf=.reviewdog.yml -reporter=github-pr-review

.PHONY: reviewdog-local
reviewdog-local:
	reviewdog -conf=.reviewdog.yml -diff="git diff main"


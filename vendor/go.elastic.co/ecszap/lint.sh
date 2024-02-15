#!/bin/sh

printf "Updating...\n"

go mod tidy

go fmt

go run github.com/elastic/go-licenser@latest .

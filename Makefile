GOPATH=$(shell go env GOPATH)
GOTAGS="badgerdb"

.PHONY: default
default: lint test

.PHONY: lint
lint:
	go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.37.0
	$(GOPATH)/bin/golangci-lint run -e gosec ./... --timeout=2m
	go fmt ./...
	go mod tidy

.PHONY: test
test:
	go test -tags=$(GOTAGS) -race ./...

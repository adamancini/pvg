.PHONY: help build install test vet clean

VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build pvg binary
	go build -ldflags "$(LDFLAGS)" -o pvg ./cmd/pvg

install: ## Install pvg to $GOPATH/bin
	go install -ldflags "$(LDFLAGS)" ./cmd/pvg

test: ## Run tests
	go test -v ./...

vet: ## Run go vet
	go vet ./...

clean: ## Remove build artifacts
	rm -f pvg
	rm -rf dist/

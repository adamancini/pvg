VERSION := $(shell cat VERSION)
BINARY  := pvg
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test vet install clean help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

build: ## Build pvg binary
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) ./cmd/pvg/
	@echo "Built $(BINARY) $(VERSION)"

test: ## Run all tests
	go test ./... -v

vet: ## Run go vet
	go vet ./...

install: build ## Install pvg to ~/go/bin
	@mkdir -p "$(HOME)/go/bin"
	cp $(BINARY) "$(HOME)/go/bin/$(BINARY)"
	@echo "Installed $(BINARY) $(VERSION) to $(HOME)/go/bin/$(BINARY)"

clean: ## Remove build artifacts
	rm -f $(BINARY)
	rm -rf dist/

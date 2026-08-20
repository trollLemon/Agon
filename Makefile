SHELL := /bin/bash

BIN := agon
PKG := ./cmd

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@echo "Usage: make <target>"
	@echo
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile the binary
	@go build -o $(BIN) $(PKG)

.PHONY: run
run: ## Run the TUI in the foreground (Ctrl-C to quit)
	@go run $(PKG)

.PHONY: test
test: ## Run all tests with the race detector, serialized (recommended for -race)
	@go test -race -p 1 -v ./...

.PHONY: vet
vet: ## Run go vet
	@go vet ./...

.PHONY: fmt
fmt: ## Check gofmt formatting
	@test -z "$$(gofmt -l .)" || { echo "not gofmt-formatted:"; gofmt -l .; exit 1; }

.PHONY: clean
clean: ## Remove the built binary
	@rm -f $(BIN)
	@echo "cleaned"

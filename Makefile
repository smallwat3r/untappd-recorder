# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Directories
BIN_DIR=bin

# Binary names
APP_NAME_BACKFILL=backfill
APP_NAME_RECORD=record
TARGET_BACKFILL=$(BIN_DIR)/$(APP_NAME_BACKFILL)
TARGET_RECORD=$(BIN_DIR)/$(APP_NAME_RECORD)

.DEFAULT_GOAL := help

.PHONY: help
help:  ## Show this help menu
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: all
all: fmt lint test build ## Run all common tasks

.PHONY: build
build: build-backfill build-record ## Build all Go applications

.PHONY: build-backfill
build-backfill: ## Build the backfill Go application
	$(GOBUILD) -o $(TARGET_BACKFILL) ./cmd/backfill

.PHONY: build-record
build-record: ## Build the record Go application
	$(GOBUILD) -o $(TARGET_RECORD) ./cmd/record

.PHONY: test
test: ## Run the Go tests
	$(GOTEST) -v ./...

.PHONY: clean
clean: ## Remove compiled binaries and build cache
	@if [ -d $(BIN_DIR) ] ; then rm -r $(BIN_DIR); fi
	$(GOCLEAN) -cache

.PHONY: tidy
tidy: ## Tidy the Go module dependencies
	$(GOMOD) tidy

.PHONY: fmt
fmt: ## Format Go source files
	$(GOFMT) ./...

.PHONY: lint
lint: ## Lint Go source files
	$(GOVET) ./...
	@command -v staticcheck >/dev/null 2>&1 || (echo "Installing staticcheck..."; go install honnef.co/go/tools/cmd/staticcheck@latest)
	@staticcheck ./...

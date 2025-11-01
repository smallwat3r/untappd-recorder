BINARY_NAME=untappd-server
BINARY_UNIX=$(BINARY_NAME)

.PHONY: help
help:  ## Show this help menu
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@grep --no-filename -E '^[a-zA-Z_%-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "% -25s %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the Go application
	go build -o $(BINARY_UNIX) ./cmd/...

.PHONY: run
run: build ## Run the Go application
	@./$(BINARY_UNIX)

.PHONY: test
test: ## Run the Go tests
	go test -v ./...

clean: ## Remove compiled binaries and build cache
	@if [ -f $(BINARY_UNIX) ] ; then rm $(BINARY_UNIX); fi

.PHONY: tidy
tidy: ## Tidy the Go module dependencies
	go mod tidy

.PHONY: fmt
fmt: ## Format Go source files
	go fmt ./...

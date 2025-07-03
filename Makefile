.PHONY: build lint fmt test all clean

# Variables
BINARY_NAME=pd
OUTPUT_DIR=bin
PKG_LIST=$(shell go list ./... | grep -v /vendor/ | grep -v /integration_tests)

# Default target
all: build test lint fmt

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	go build -v -o $(OUTPUT_DIR)/$(BINARY_NAME) main.go

# Run linter
lint:
	@echo "Linting code..."
	@command -v golangci-lint > /dev/null 2>&1 || { echo >&2 "golangci-lint not found. Please install: https://golangci-lint.run/usage/install/"; exit 1; }
	golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	gofmt -w .

# Run tests (excluding integration tests)
test:
	@echo "Running tests (excluding integration tests)..."
	go test -v $(PKG_LIST)

test-quiet:
	@echo "Running tests quietly (excluding integration tests)..."
	go test  $(PKG_LIST)

# Run all tests including integration tests
test-all: test test-integration
	@echo "All tests completed" 

# Run integration tests
test-integration: build
	@echo "Running integration tests..."
	@if [ -z "$(GCP_PROJECT_ID)" ]; then \
		echo "Error: GCP_PROJECT_ID environment variable is not set"; \
		exit 1; \
	fi
	@cp $(OUTPUT_DIR)/$(BINARY_NAME) integration_tests/$(BINARY_NAME)
	cd integration_tests && go test -v -timeout 45m ./...
	@rm -f integration_tests/$(BINARY_NAME)

# Run integration tests in parallel
test-integration-parallel: build
	@echo "Running integration tests in parallel..."
	@if [ -z "$(GCP_PROJECT_ID)" ]; then \
		echo "Error: GCP_PROJECT_ID environment variable is not set"; \
		exit 1; \
	fi
	@cp $(OUTPUT_DIR)/$(BINARY_NAME) integration_tests/$(BINARY_NAME)
	cd integration_tests && go test -v -parallel 4 -timeout 45m ./...
	@rm -f integration_tests/$(BINARY_NAME)

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(OUTPUT_DIR)
	@rm -f integration_tests/$(BINARY_NAME)
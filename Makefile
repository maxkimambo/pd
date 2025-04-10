.PHONY: build lint fmt test all clean

# Variables
BINARY_NAME=pd
OUTPUT_DIR=bin
PKG_LIST=$(shell go list ./... | grep -v /vendor/)

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

# Run tests
test:
	@echo "Running tests..."
	go test -v $(PKG_LIST)

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(OUTPUT_DIR)
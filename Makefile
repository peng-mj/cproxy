# Makefile for scproxy

APP_NAME := scproxy
OUTPUT_DIR := output
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# Binary name
BINARY_NAME := $(APP_NAME)

# Main targets
.PHONY: all build clean test run help install deps

all: build

## build: Build the application for current platform
build:
	@echo "Building $(BINARY_NAME)..."
	@$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "Build complete: $(BINARY_NAME)"

## build-all: Build for all platforms (Linux/Windows + amd64/arm64)
build-all:
	@echo "Building for all platforms..."
	@./build.sh

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@$(GOCLEAN)
	@rm -rf $(OUTPUT_DIR)
	@rm -f $(BINARY_NAME)
	@echo "Clean complete"

## test: Run tests
test:
	@echo "Running tests..."
	@$(GOTEST) -v ./...

## test-cover: Run tests with coverage
test-cover:
	@echo "Running tests with coverage..."
	@$(GOTEST) -v -coverprofile=coverage.out ./...
	@$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## run: Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	@$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .
	@./$(BINARY_NAME) --help

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

## install: Install the application to /usr/local/bin
install:
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@sudo cp $(BINARY_NAME) /usr/local/bin/
	@sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "Installation complete"

## lint: Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

## fmt: Format code
fmt:
	@echo "Formatting code..."
	@$(GOCMD) fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	@$(GO) vet ./...

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

# Development targets
.PHONY: dev dev-run

## dev: Build for development
dev:
	@echo "Building for development..."
	@$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "Development build complete"

## dev-run: Build and run with default config
dev-run: dev
	@echo "Starting development server..."
	@./$(BINARY_NAME) --target https://httpbin.org --proxy http://localhost:8080 --port 8080 --log-level debug

# Release targets
.PHONY: release release-linux release-windows release-darwin

## release: Create release packages
release: clean build-all
	@echo "Release packages created in $(OUTPUT_DIR)/"

## release-linux: Build only Linux versions
release-linux:
	@echo "Building Linux versions..."
	@mkdir -p $(OUTPUT_DIR)
	@for arch in amd64 arm64; do \
		GOOS=linux GOARCH=$$arch $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(APP_NAME)-linux-$$arch/$(APP_NAME) .; \
		tar -czf $(OUTPUT_DIR)/$(APP_NAME)-linux-$$arch.tar.gz -C $(OUTPUT_DIR)/$(APP_NAME)-linux-$$arch $(APP_NAME); \
	done

## release-windows: Build only Windows versions
release-windows:
	@echo "Building Windows versions..."
	@mkdir -p $(OUTPUT_DIR)
	@for arch in amd64 arm64; do \
		GOOS=windows GOARCH=$$arch $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(APP_NAME)-windows-$$arch/$(APP_NAME).exe .; \
		cd $(OUTPUT_DIR)/$(APP_NAME)-windows-$$arch && zip -q ../../$(OUTPUT_DIR)/$(APP_NAME)-windows-$$arch.zip $(APP_NAME).exe && cd ../..; \
	done

## release-darwin: Build only macOS versions
release-darwin:
	@echo "Building macOS versions..."
	@mkdir -p $(OUTPUT_DIR)
	@for arch in amd64 arm64; do \
		GOOS=darwin GOARCH=$$arch $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(APP_NAME)-darwin-$$arch/$(APP_NAME) .; \
		tar -czf $(OUTPUT_DIR)/$(APP_NAME)-darwin-$$arch.tar.gz -C $(OUTPUT_DIR)/$(APP_NAME)-darwin-$$arch $(APP_NAME); \
	done

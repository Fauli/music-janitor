.PHONY: all build test clean install run fmt vet lint doctor help

# Variables
BINARY_NAME=mlc
BUILD_DIR=build
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-X main.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"

all: test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/mlc

# Run tests
test:
	@echo "Running tests..."
	$(GO) test $(GOFLAGS) ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -coverprofile=coverage.txt -covermode=atomic ./...
	$(GO) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	$(GO) test -race ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.txt coverage.html
	@rm -f *.db *.db-shm *.db-wal
	@rm -rf artifacts/

# Install the binary to GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install $(LDFLAGS) ./cmd/mlc

# Run the binary
run: build
	@echo "Running $(BINARY_NAME)..."
	@$(BUILD_DIR)/$(BINARY_NAME)

# Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

# Run linter (requires golangci-lint)
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install from https://golangci-lint.run/" && exit 1)
	golangci-lint run ./...

# Check environment and dependencies
doctor:
	@echo "Checking environment..."
	@echo "Go version:"
	@$(GO) version
	@echo ""
	@echo "Checking for ffprobe:"
	@which ffprobe > /dev/null && echo "✓ ffprobe found: $$(ffprobe -version 2>&1 | head -n1)" || echo "✗ ffprobe not found (required)"
	@echo ""
	@echo "Checking for fpcalc (optional):"
	@which fpcalc > /dev/null && echo "✓ fpcalc found: $$(fpcalc -version 2>&1 | head -n1)" || echo "✗ fpcalc not found (optional for fingerprinting)"
	@echo ""
	@echo "Go module status:"
	@$(GO) mod verify

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# Create a release build for current platform
release:
	@echo "Building release for current platform..."
	@mkdir -p $(BUILD_DIR)/release
	$(GO) build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/release/$(BINARY_NAME) ./cmd/mlc
	@echo "Release binary created: $(BUILD_DIR)/release/$(BINARY_NAME)"

# Cross-compile for multiple platforms
release-all:
	@echo "Building releases for multiple platforms..."
	@mkdir -p $(BUILD_DIR)/release
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/release/$(BINARY_NAME)-darwin-amd64 ./cmd/mlc
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/release/$(BINARY_NAME)-darwin-arm64 ./cmd/mlc
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/release/$(BINARY_NAME)-linux-amd64 ./cmd/mlc
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/release/$(BINARY_NAME)-linux-arm64 ./cmd/mlc
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -trimpath -o $(BUILD_DIR)/release/$(BINARY_NAME)-windows-amd64.exe ./cmd/mlc
	@echo "Release binaries created in $(BUILD_DIR)/release/"

# Help
help:
	@echo "Music Library Cleaner (mlc) - Makefile targets:"
	@echo ""
	@echo "  make build         - Build the binary"
	@echo "  make test          - Run tests"
	@echo "  make test-coverage - Run tests with coverage report"
	@echo "  make test-race     - Run tests with race detector"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make install       - Install binary to GOPATH/bin"
	@echo "  make run           - Build and run the binary"
	@echo "  make fmt           - Format code with go fmt"
	@echo "  make vet           - Run go vet"
	@echo "  make lint          - Run golangci-lint"
	@echo "  make doctor        - Check environment and dependencies"
	@echo "  make deps          - Download and tidy dependencies"
	@echo "  make release       - Build release binary for current platform"
	@echo "  make release-all   - Build release binaries for all platforms"
	@echo "  make help          - Show this help message"

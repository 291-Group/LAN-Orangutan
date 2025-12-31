# LAN Orangutan Makefile

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -ldflags "-s -w \
	-X 'github.com/291-Group/LAN-Orangutan/internal/cli.Version=$(VERSION)' \
	-X 'github.com/291-Group/LAN-Orangutan/internal/cli.Commit=$(COMMIT)' \
	-X 'github.com/291-Group/LAN-Orangutan/internal/cli.BuildDate=$(BUILD_DATE)'"

BINARY := orangutan
BUILD_DIR := bin

.PHONY: all build clean test lint install

all: build

# Build for current platform
build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/orangutan

# Build for all platforms
build-all: build-linux build-darwin build-windows

# Linux builds
build-linux: build-linux-amd64 build-linux-arm64 build-linux-arm

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/orangutan

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd/orangutan

build-linux-arm:
	GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm ./cmd/orangutan

# macOS builds
build-darwin: build-darwin-amd64 build-darwin-arm64

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./cmd/orangutan

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./cmd/orangutan

# Windows builds
build-windows: build-windows-amd64

build-windows-amd64:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe ./cmd/orangutan

# Run tests
test:
	go test -v ./...

# Run linter
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# Tidy dependencies
tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	go clean

# Install locally
install: build
	sudo cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)

# Development run
run: build
	./$(BUILD_DIR)/$(BINARY) serve

# Check for vulnerabilities
vuln:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

# Generate checksums for release
checksums:
	cd $(BUILD_DIR) && sha256sum $(BINARY)-* > SHA256SUMS.txt

# Create release archives
release: build-all checksums
	cd $(BUILD_DIR) && for f in $(BINARY)-*; do \
		if [ -f "$$f" ] && [ "$$f" != "SHA256SUMS.txt" ]; then \
			if echo "$$f" | grep -q ".exe"; then \
				zip "$${f%.exe}.zip" "$$f"; \
			else \
				tar -czvf "$$f.tar.gz" "$$f"; \
			fi \
		fi \
	done

help:
	@echo "LAN Orangutan Build Targets:"
	@echo ""
	@echo "  build          - Build for current platform"
	@echo "  build-all      - Build for all platforms"
	@echo "  build-linux    - Build for Linux (amd64, arm64, arm)"
	@echo "  build-darwin   - Build for macOS (amd64, arm64)"
	@echo "  build-windows  - Build for Windows (amd64)"
	@echo ""
	@echo "  test           - Run tests"
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"
	@echo "  tidy           - Tidy dependencies"
	@echo ""
	@echo "  clean          - Clean build artifacts"
	@echo "  install        - Install to /usr/local/bin"
	@echo "  run            - Build and run server"
	@echo ""
	@echo "  release        - Build all platforms and create archives"
	@echo "  checksums      - Generate SHA256 checksums"

# musicd Makefile
# Cross-platform build targets for the music daemon

BINARY_NAME=musicd
VERSION?=dev
BUILD_DIR=../bin
GO=go
GOFLAGS=-ldflags "-X main.Version=$(VERSION)"
DOCKER_IMAGE=musicd-builder

# Build targets
.PHONY: all build build-docker docker-image build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 clean test test-coverage

all: build

# Prerequisites:
# Linux: sudo apt-get install libasound2-dev
# macOS: Xcode command line tools
# Windows: MinGW or MSVC
# Docker: docker build -t musicd-builder .

build:
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/musicd

# Docker-based build (no local dependencies required)
docker-image:
	docker build -t $(DOCKER_IMAGE) .

build-docker: docker-image
	@mkdir -p $(BUILD_DIR)
	@CONTAINER_ID=$$(docker create $(DOCKER_IMAGE)) && \
		docker cp "$$CONTAINER_ID:/musicd" $(BUILD_DIR)/$(BINARY_NAME) && \
		docker rm "$$CONTAINER_ID" > /dev/null && \
		chmod +x $(BUILD_DIR)/$(BINARY_NAME) && \
		echo "✓ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Linux builds (require CGO for ALSA audio)
build-linux-amd64:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/musicd

build-linux-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/musicd

# macOS builds (requires cgo for media session)
build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/musicd

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/musicd

# Windows build (no CGO needed for stub media session)
build-windows-amd64:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/musicd

# Build all platforms (native - requires all toolchains)
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64

# Docker-based builds for Linux (cross-platform friendly)
build-docker-linux-amd64: docker-image
	@mkdir -p $(BUILD_DIR)
	@CONTAINER_ID=$$(docker create --platform linux/amd64 $(DOCKER_IMAGE)) && \
		docker cp "$$CONTAINER_ID:/musicd" $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 && \
		docker rm "$$CONTAINER_ID" > /dev/null && \
		chmod +x $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 && \
		echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

build-docker-linux-arm64:
	@mkdir -p $(BUILD_DIR)
	docker build --platform linux/arm64 -t $(DOCKER_IMAGE)-arm64 .
	@CONTAINER_ID=$$(docker create --platform linux/arm64 $(DOCKER_IMAGE)-arm64) && \
		docker cp "$$CONTAINER_ID:/musicd" $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 && \
		docker rm "$$CONTAINER_ID" > /dev/null && \
		chmod +x $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 && \
		echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64"

# Build all Linux variants with Docker
build-docker-all-linux: build-docker-linux-amd64 build-docker-linux-arm64

# Testing
test:
	$(GO) test ./... -v -race

test-short:
	$(GO) test ./... -short

test-coverage:
	$(GO) test ./... -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out -o coverage.html

test-integration:
	$(GO) test ./internal/ipc/... -tags=integration -v

# Linting
lint:
	golangci-lint run ./...

# Clean
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Development helpers
run:
	$(GO) run ./cmd/musicd -verbose

run-test-mode:
	$(GO) run ./cmd/musicd -test-mode -verbose

# Check syntax without building (useful when system deps are missing)
check:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

# Install development dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

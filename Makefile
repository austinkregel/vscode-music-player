# local-media Makefile
# Orchestrates building both the Go daemon and the VS Code extension

.PHONY: all build build-daemon build-extension clean test test-go test-extension package

# Default target
all: build

# Build everything
build: build-daemon build-extension

# Build the Go daemon for all platforms (requires native toolchains)
build-daemon:
	cd src-go && $(MAKE) build-all

# Build just for the current platform
build-daemon-local:
	cd src-go && $(MAKE) build

# Build Linux daemons using Docker (cross-platform friendly)
build-daemon-docker:
	cd src-go && $(MAKE) build-docker-linux-amd64

# Build all Linux variants with Docker
build-daemon-docker-all:
	cd src-go && $(MAKE) build-docker-all-linux

# Build the VS Code extension
build-extension:
	npm run compile

# Package the extension for distribution
# Uses Docker for Linux builds, requires native builds for macOS/Windows
package: clean-bin build-daemon-docker build-extension
	npm run package
	@echo "✓ Extension packaged. Run 'vsce package' to create .vsix"

# Package with all platforms (run on each platform or use CI)
package-all: clean-bin build-daemon build-extension
	npm run package
	@echo "✓ Extension packaged with all platform binaries"

# Clean all build artifacts
clean:
	rm -rf bin/ dist/ out/ coverage/
	cd src-go && $(MAKE) clean

# Clean just the bin folder (for rebuilding binaries)
clean-bin:
	rm -rf bin/
	mkdir -p bin

# Run all tests
test: test-go test-extension

# Run Go tests
test-go:
	cd src-go && $(MAKE) test

# Run Go tests with coverage
test-go-coverage:
	cd src-go && $(MAKE) test-coverage

# Run extension tests
test-extension:
	npm test

# Run extension unit tests only
test-extension-unit:
	npm run test:unit

# Setup development environment (detects OS and installs deps)
setup:
	./scripts/setup.sh

# Install dependencies
deps:
	npm install
	cd src-go && go mod download

# Development: watch mode
watch:
	npm run watch

# Lint
lint:
	npm run lint
	cd src-go && $(MAKE) lint

# Format code
fmt:
	cd src-go && $(MAKE) fmt

# Create bin directory structure
setup-bin:
	mkdir -p bin

# Copy daemon binaries to extension bin folder
copy-binaries: build-daemon setup-bin
	cp src-go/../bin/* bin/ 2>/dev/null || true

# Development helpers
.PHONY: dev dev-daemon

# Run daemon in development mode
dev-daemon:
	cd src-go && $(MAKE) run-test-mode

# Full development setup
dev: deps build-daemon-local
	npm run watch

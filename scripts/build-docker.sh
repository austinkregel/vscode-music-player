#!/bin/bash
#
# Build the musicd daemon using Docker
# This avoids host system dependency issues
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BIN_DIR="$PROJECT_DIR/bin"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}Building musicd daemon using Docker...${NC}"

# Create bin directory
mkdir -p "$BIN_DIR"

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        TARGETARCH="amd64"
        ;;
    aarch64|arm64)
        TARGETARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo -e "  Target: ${YELLOW}linux/${TARGETARCH}${NC}"

# Build using Docker
cd "$PROJECT_DIR"

# Use BuildKit for better caching and the export feature
DOCKER_BUILDKIT=1 docker build \
    --file scripts/Dockerfile.build \
    --target builder \
    --build-arg TARGETARCH="$TARGETARCH" \
    --output type=local,dest="$BIN_DIR" \
    --progress=plain \
    . 2>&1 | tail -20

# The binary should now be in bin/
if [ -f "$BIN_DIR/musicd" ]; then
    chmod +x "$BIN_DIR/musicd"
    echo -e "${GREEN}✓ Build successful!${NC}"
    echo -e "  Binary: ${YELLOW}$BIN_DIR/musicd${NC}"
    
    # Show binary info
    file "$BIN_DIR/musicd"
else
    # Try alternative extraction method
    echo "Extracting binary from container..."
    
    docker build \
        --file scripts/Dockerfile.build \
        --target builder \
        --build-arg TARGETARCH="$TARGETARCH" \
        -t musicd-builder \
        . 
    
    # Create container and copy binary out
    CONTAINER_ID=$(docker create musicd-builder)
    docker cp "$CONTAINER_ID:/musicd" "$BIN_DIR/musicd"
    docker rm "$CONTAINER_ID"
    
    if [ -f "$BIN_DIR/musicd" ]; then
        chmod +x "$BIN_DIR/musicd"
        echo -e "${GREEN}✓ Build successful!${NC}"
        echo -e "  Binary: ${YELLOW}$BIN_DIR/musicd${NC}"
    else
        echo "Build failed - binary not found"
        exit 1
    fi
fi

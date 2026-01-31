#!/bin/bash
#
# Setup script for local-media development
# Detects OS and installs required dependencies
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     Local Media - Development Setup        ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════╝${NC}"
echo ""

# Detect OS
detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Linux - detect distro
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            OS="linux"
            DISTRO=$ID
            DISTRO_VERSION=$VERSION_ID
            DISTRO_NAME=$PRETTY_NAME
        elif [ -f /etc/lsb-release ]; then
            . /etc/lsb-release
            OS="linux"
            DISTRO=$DISTRIB_ID
            DISTRO_VERSION=$DISTRIB_RELEASE
            DISTRO_NAME=$DISTRIB_DESCRIPTION
        else
            OS="linux"
            DISTRO="unknown"
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        OS="macos"
        DISTRO="macos"
        DISTRO_VERSION=$(sw_vers -productVersion)
        DISTRO_NAME="macOS $DISTRO_VERSION"
    elif [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]] || [[ "$OSTYPE" == "win32" ]]; then
        OS="windows"
        DISTRO="windows"
        DISTRO_NAME="Windows"
    else
        OS="unknown"
        DISTRO="unknown"
    fi
}

# Detect desktop environment
detect_desktop() {
    if [ -n "$XDG_CURRENT_DESKTOP" ]; then
        DESKTOP=$XDG_CURRENT_DESKTOP
    elif [ -n "$DESKTOP_SESSION" ]; then
        DESKTOP=$DESKTOP_SESSION
    else
        DESKTOP="unknown"
    fi
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Print system info
print_system_info() {
    echo -e "${GREEN}System Information:${NC}"
    echo -e "  OS:        ${YELLOW}$OS${NC}"
    echo -e "  Distro:    ${YELLOW}$DISTRO_NAME${NC}"
    echo -e "  Desktop:   ${YELLOW}$DESKTOP${NC}"
    echo ""
}

# Install dependencies for Debian/Ubuntu
install_debian() {
    echo -e "${GREEN}Installing dependencies for Debian/Ubuntu...${NC}"
    
    local packages=(
        "build-essential"
        "libasound2-dev"    # ALSA development headers for audio
        "pkg-config"
        "ffmpeg"            # For audio decoding
    )
    
    echo -e "${YELLOW}The following packages will be installed:${NC}"
    printf '  - %s\n' "${packages[@]}"
    echo ""
    
    read -p "Do you want to proceed? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo apt-get update
        sudo apt-get install -y "${packages[@]}"
        echo -e "${GREEN}✓ Dependencies installed successfully${NC}"
    else
        echo -e "${YELLOW}Skipped dependency installation${NC}"
    fi
}

# Install dependencies for Fedora/RHEL
install_fedora() {
    echo -e "${GREEN}Installing dependencies for Fedora/RHEL...${NC}"
    
    local packages=(
        "gcc"
        "alsa-lib-devel"    # ALSA development headers
        "pkg-config"
        "ffmpeg"
    )
    
    echo -e "${YELLOW}The following packages will be installed:${NC}"
    printf '  - %s\n' "${packages[@]}"
    echo ""
    
    read -p "Do you want to proceed? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo dnf install -y "${packages[@]}"
        echo -e "${GREEN}✓ Dependencies installed successfully${NC}"
    else
        echo -e "${YELLOW}Skipped dependency installation${NC}"
    fi
}

# Install dependencies for Arch Linux
install_arch() {
    echo -e "${GREEN}Installing dependencies for Arch Linux...${NC}"
    
    local packages=(
        "base-devel"
        "alsa-lib"
        "pkg-config"
        "ffmpeg"
    )
    
    echo -e "${YELLOW}The following packages will be installed:${NC}"
    printf '  - %s\n' "${packages[@]}"
    echo ""
    
    read -p "Do you want to proceed? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo pacman -S --noconfirm "${packages[@]}"
        echo -e "${GREEN}✓ Dependencies installed successfully${NC}"
    else
        echo -e "${YELLOW}Skipped dependency installation${NC}"
    fi
}

# Install dependencies for macOS
install_macos() {
    echo -e "${GREEN}Installing dependencies for macOS...${NC}"
    
    if ! command_exists brew; then
        echo -e "${RED}Homebrew not found. Please install Homebrew first:${NC}"
        echo "  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
        exit 1
    fi
    
    local packages=(
        "ffmpeg"
        "pkg-config"
    )
    
    echo -e "${YELLOW}The following packages will be installed:${NC}"
    printf '  - %s\n' "${packages[@]}"
    echo ""
    
    read -p "Do you want to proceed? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        brew install "${packages[@]}"
        echo -e "${GREEN}✓ Dependencies installed successfully${NC}"
    else
        echo -e "${YELLOW}Skipped dependency installation${NC}"
    fi
}

# Check Go installation
check_go() {
    echo -e "${GREEN}Checking Go installation...${NC}"
    
    if command_exists go; then
        GO_VERSION=$(go version | awk '{print $3}')
        echo -e "  Go version: ${YELLOW}$GO_VERSION${NC}"
        return 0
    else
        echo -e "${RED}Go is not installed.${NC}"
        echo ""
        echo "Please install Go from: https://go.dev/dl/"
        echo "Or use your package manager:"
        
        case "$DISTRO" in
            ubuntu|debian|linuxmint|pop)
                echo "  sudo apt-get install golang-go"
                ;;
            fedora|rhel|centos)
                echo "  sudo dnf install golang"
                ;;
            arch|manjaro)
                echo "  sudo pacman -S go"
                ;;
            macos)
                echo "  brew install go"
                ;;
        esac
        return 1
    fi
}

# Check Node.js installation
check_node() {
    echo -e "${GREEN}Checking Node.js installation...${NC}"
    
    if command_exists node; then
        NODE_VERSION=$(node --version)
        echo -e "  Node version: ${YELLOW}$NODE_VERSION${NC}"
        return 0
    else
        echo -e "${RED}Node.js is not installed.${NC}"
        echo ""
        echo "Please install Node.js from: https://nodejs.org/"
        return 1
    fi
}

# Check FFmpeg installation
check_ffmpeg() {
    echo -e "${GREEN}Checking FFmpeg installation...${NC}"
    
    if command_exists ffmpeg; then
        FFMPEG_VERSION=$(ffmpeg -version | head -n1 | awk '{print $3}')
        echo -e "  FFmpeg version: ${YELLOW}$FFMPEG_VERSION${NC}"
        return 0
    else
        echo -e "${YELLOW}FFmpeg is not installed (required for audio playback)${NC}"
        return 1
    fi
}

# Install dependencies based on OS
install_dependencies() {
    case "$DISTRO" in
        ubuntu|debian|linuxmint|pop|elementary|zorin)
            install_debian
            ;;
        fedora|rhel|centos|rocky|alma)
            install_fedora
            ;;
        arch|manjaro|endeavouros)
            install_arch
            ;;
        macos)
            install_macos
            ;;
        *)
            echo -e "${YELLOW}Unknown distribution: $DISTRO${NC}"
            echo "Please manually install:"
            echo "  - ALSA development headers (libasound2-dev or alsa-lib-devel)"
            echo "  - FFmpeg"
            echo "  - pkg-config"
            ;;
    esac
}

# Build the Go daemon
build_daemon() {
    echo ""
    echo -e "${GREEN}Building Go daemon...${NC}"
    
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
    
    cd "$PROJECT_DIR/src-go"
    
    if make build; then
        echo -e "${GREEN}✓ Daemon built successfully${NC}"
        echo -e "  Binary: ${YELLOW}$PROJECT_DIR/bin/musicd${NC}"
        return 0
    else
        echo -e "${RED}✗ Daemon build failed${NC}"
        return 1
    fi
}

# Main script
main() {
    detect_os
    detect_desktop
    print_system_info
    
    echo -e "${GREEN}Checking prerequisites...${NC}"
    echo ""
    
    GO_OK=true
    NODE_OK=true
    FFMPEG_OK=true
    
    check_go || GO_OK=false
    check_node || NODE_OK=false
    check_ffmpeg || FFMPEG_OK=false
    
    echo ""
    
    if [ "$GO_OK" = false ] || [ "$NODE_OK" = false ]; then
        echo -e "${RED}Missing required tools. Please install them first.${NC}"
        exit 1
    fi
    
    # Install OS-specific dependencies
    echo ""
    install_dependencies
    
    echo ""
    echo -e "${GREEN}Installing Node.js dependencies...${NC}"
    
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
    
    cd "$PROJECT_DIR"
    npm install
    
    echo ""
    read -p "Do you want to build the Go daemon now? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        build_daemon
    fi
    
    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║           Setup Complete!                  ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Build the daemon:    cd src-go && make build"
    echo "  2. Build the extension: npm run compile"
    echo "  3. Run tests:           npm test"
    echo ""
}

# Run main
main "$@"

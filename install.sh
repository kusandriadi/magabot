#!/bin/bash
# Magabot Installer
# Usage: curl -sL https://raw.githubusercontent.com/kusandriadi/magabot/master/install.sh | bash

set -e

REPO="kusandriadi/magabot"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="magabot"
VERSION="${MAGABOT_VERSION:-latest}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case $ARCH in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l)  ARCH="arm" ;;
        *)       error "Unsupported architecture: $ARCH" ;;
    esac
    
    case $OS in
        linux|darwin) ;;
        *)  error "Unsupported OS: $OS" ;;
    esac
    
    PLATFORM="${OS}_${ARCH}"
    info "Detected platform: $PLATFORM"
}

# Get latest version
get_latest_version() {
    if [ "$VERSION" = "latest" ]; then
        VERSION=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
        if [ -z "$VERSION" ]; then
            VERSION="v0.1.0"
        fi
    fi
    info "Version: $VERSION"
}

# Download and install binary
install_binary() {
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/magabot_${PLATFORM}.tar.gz"
    
    info "Downloading from: $DOWNLOAD_URL"
    
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT
    
    # Try to download pre-built binary
    if curl -sL --fail "$DOWNLOAD_URL" -o "$TMP_DIR/magabot.tar.gz" 2>/dev/null; then
        tar -xzf "$TMP_DIR/magabot.tar.gz" -C "$TMP_DIR"
        
        # Install
        if [ -w "$INSTALL_DIR" ]; then
            mv "$TMP_DIR/magabot" "$INSTALL_DIR/"
        else
            info "Need sudo to install to $INSTALL_DIR"
            sudo mv "$TMP_DIR/magabot" "$INSTALL_DIR/"
        fi
        
        chmod +x "$INSTALL_DIR/magabot"
        info "Installed to: $INSTALL_DIR/magabot"
    else
        warn "Pre-built binary not found. Building from source..."
        install_from_source
    fi
}

# Install from source
install_from_source() {
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        error "Go is not installed. Install Go first: https://go.dev/dl/"
    fi
    
    info "Building from source..."
    
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT
    
    git clone --depth 1 "https://github.com/${REPO}.git" "$TMP_DIR/magabot"
    cd "$TMP_DIR/magabot"
    
    go build -ldflags="-s -w" -o magabot ./cmd/magabot
    
    if [ -w "$INSTALL_DIR" ]; then
        mv magabot "$INSTALL_DIR/"
    else
        info "Need sudo to install to $INSTALL_DIR"
        sudo mv magabot "$INSTALL_DIR/"
    fi
    
    info "Built and installed to: $INSTALL_DIR/magabot"
}

# Verify installation
verify() {
    if command -v magabot &> /dev/null; then
        info "Installation successful!"
        echo ""
        magabot version
        echo ""
        info "Run 'magabot setup' to configure"
    else
        error "Installation failed"
    fi
}

# Install voice dependencies (optional)
install_voice_deps() {
    echo ""
    info "Voice Support (optional)"
    echo "  Enables voice-to-voice: send voice → transcribe → LLM → reply with voice"
    echo "  Requires: faster-whisper, edge-tts, ffmpeg"
    echo ""
    read -r -p "Install voice dependencies? [Y/n] " reply
    echo
    if [[ "$reply" =~ ^[Nn]$ ]]; then
        info "Skipping voice dependencies. Run 'magabot setup voice' later to install."
        return
    fi

    # ffmpeg
    if command -v ffmpeg &> /dev/null; then
        info "ffmpeg already installed"
    else
        info "Installing ffmpeg..."
        if [ "$OS" = "darwin" ]; then
            if command -v brew &> /dev/null; then
                brew install ffmpeg
            else
                warn "Homebrew not found. Install manually: brew install ffmpeg"
            fi
        else
            sudo apt-get install -y ffmpeg || warn "Could not install ffmpeg. Install manually: sudo apt install ffmpeg"
        fi
    fi

    # pip
    PIP=$(command -v pip3 2>/dev/null || command -v pip 2>/dev/null)
    if [ -z "$PIP" ]; then
        warn "pip not found. Install Python 3 and pip, then run: magabot setup voice"
        return
    fi

    info "Installing faster-whisper..."
    $PIP install faster-whisper || warn "Could not install faster-whisper"

    info "Installing edge-tts..."
    $PIP install edge-tts || warn "Could not install edge-tts"

    # Install scripts via magabot
    info "Installing voice scripts..."
    magabot setup voice --scripts-only 2>/dev/null || true

    info "Voice dependencies installed! First use will download the Whisper model (~1.5 GB)."
}

# Main
main() {
    echo "🤖 Magabot Installer"
    echo "===================="
    echo ""

    detect_platform
    get_latest_version
    install_binary
    verify
    install_voice_deps

    echo ""
    info "Quick start:"
    echo "  magabot setup    # First-time setup"
    echo "  magabot start    # Start bot"
    echo "  magabot status   # Check status"
}

main "$@"

#!/bin/bash

# DDx (Document-Driven Development eXperience) Installation Script
# Usage: curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
DDX_HOME="${HOME}/.ddx"
DDX_REPO="https://github.com/DocumentDrivenDX/ddx"
DDX_API="https://api.github.com/repos/DocumentDrivenDX/ddx"

# Logging functions
log() {
    echo -e "${BLUE}[DDx]${NC} $1"
}

success() {
    echo -e "${GREEN}[DDx]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[DDx]${NC} $1"
}

error() {
    echo -e "${RED}[DDx]${NC} $1"
    exit 1
}

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check for git
    if ! command -v git &> /dev/null; then
        error "Git is required but not installed. Please install git first."
    fi
    
    # Check for basic utilities (curl/wget for downloading binaries)
    if ! command -v curl &> /dev/null && ! command -v wget &> /dev/null; then
        error "curl or wget is required but neither is installed."
    fi

    # Check for git-subtree (required for sync features)
    if ! git subtree 2>&1 | grep -q "git subtree"; then
        warn "git-subtree not found. Some DDx features will be limited."
        echo ""
        case "$(uname -s)" in
            Linux)
                if command -v dnf &>/dev/null; then
                    warn "Install with: sudo dnf install git-subtree"
                elif command -v apt &>/dev/null; then
                    warn "Usually included with git. Try: sudo apt update && sudo apt install git"
                else
                    warn "Install git-subtree using your package manager"
                fi
                ;;
            Darwin)
                warn "Install with: brew install git"
                ;;
            *)
                warn "Install git-subtree for your operating system"
                ;;
        esac
        echo ""
    fi

    success "Prerequisites check passed"
}

# Setup DDx directory structure
setup_ddx_directory() {
    log "Setting up DDx directory structure at ${DDX_HOME}..."

    # Create DDx home directory if it doesn't exist
    mkdir -p "${DDX_HOME}"

    success "Directory structure created"
}

# Resolve the version to install
resolve_version() {
    if [ -n "${DDX_VERSION:-}" ]; then
        # Strip leading 'v' if provided, then normalize to tag format
        VERSION="${DDX_VERSION#v}"
        TAG="v${VERSION}"
        log "Using requested version: ${TAG}"
    else
        log "Fetching latest release version..."
        if command -v curl &> /dev/null; then
            TAG=$(curl -fsSL "${DDX_API}/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
        else
            TAG=$(wget -qO- "${DDX_API}/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
        fi
        if [ -z "$TAG" ]; then
            error "Could not determine latest release version. Set DDX_VERSION to specify a version."
        fi
        VERSION="${TAG#v}"
        log "Latest release: ${TAG}"
    fi
    echo "$TAG"
}

# Install CLI tool
install_cli() {
    # Check if DDx is already installed
    LOCAL_BIN="${HOME}/.local/bin"
    EXISTING_VERSION=""
    if [ -x "${LOCAL_BIN}/ddx" ]; then
        EXISTING_VERSION=$("${LOCAL_BIN}/ddx" version 2>/dev/null | head -1 | awk '{print $2}' || echo "")
    fi

    if [ -n "$EXISTING_VERSION" ]; then
        log "Upgrading DDx from ${EXISTING_VERSION}..."
    else
        log "Installing DDx CLI tool..."
    fi

    # Resolve version
    TAG=$(resolve_version)

    # Detect platform
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$ARCH" in
        x86_64) ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l) ARCH="arm" ;;
    esac

    case "$OS" in
        linux|darwin) ;;
        *)
            error "Unsupported OS: ${OS}. Supported: linux, darwin."
            ;;
    esac

    # Determine archive extension based on OS
    ARCHIVE_EXT="tar.gz"
    BINARY_NAME="ddx"

    # Download appropriate archive
    ARCHIVE_NAME="ddx-${OS}-${ARCH}.${ARCHIVE_EXT}"
    DOWNLOAD_URL="${DDX_REPO}/releases/download/${TAG}/${ARCHIVE_NAME}"
    CHECKSUM_URL="${DOWNLOAD_URL}.sha256"

    log "Downloading ${ARCHIVE_NAME} from ${DOWNLOAD_URL}..."

    # Create temp directory for download
    TEMP_DIR=$(mktemp -d)
    trap "rm -rf ${TEMP_DIR}" EXIT

    # Download archive with error checking
    if command -v curl &> /dev/null; then
        if ! curl -fsSL "${DOWNLOAD_URL}" -o "${TEMP_DIR}/${ARCHIVE_NAME}"; then
            error "Failed to download ${ARCHIVE_NAME}. Please check your internet connection and try again."
        fi
    else
        if ! wget -q "${DOWNLOAD_URL}" -O "${TEMP_DIR}/${ARCHIVE_NAME}"; then
            error "Failed to download ${ARCHIVE_NAME}. Please check your internet connection and try again."
        fi
    fi

    # Verify download succeeded and file is not empty
    if [ ! -f "${TEMP_DIR}/${ARCHIVE_NAME}" ] || [ ! -s "${TEMP_DIR}/${ARCHIVE_NAME}" ]; then
        error "Downloaded file is missing or empty. The release may not exist for ${OS}-${ARCH}."
    fi

    # Verify checksum if available
    if command -v curl &> /dev/null; then
        CHECKSUM_DATA=$(curl -fsSL "${CHECKSUM_URL}" 2>/dev/null || true)
    else
        CHECKSUM_DATA=$(wget -qO- "${CHECKSUM_URL}" 2>/dev/null || true)
    fi
    if [ -n "$CHECKSUM_DATA" ]; then
        log "Verifying checksum..."
        echo "${CHECKSUM_DATA}" > "${TEMP_DIR}/${ARCHIVE_NAME}.sha256"
        # The .sha256 file contains "hash  filename" — rewrite to point at local file
        EXPECTED_HASH=$(awk '{print $1}' "${TEMP_DIR}/${ARCHIVE_NAME}.sha256")
        if command -v sha256sum &> /dev/null; then
            ACTUAL_HASH=$(sha256sum "${TEMP_DIR}/${ARCHIVE_NAME}" | awk '{print $1}')
        elif command -v shasum &> /dev/null; then
            ACTUAL_HASH=$(shasum -a 256 "${TEMP_DIR}/${ARCHIVE_NAME}" | awk '{print $1}')
        else
            warn "No sha256sum or shasum found; skipping checksum verification."
            ACTUAL_HASH="$EXPECTED_HASH"
        fi
        if [ "$ACTUAL_HASH" != "$EXPECTED_HASH" ]; then
            error "Checksum mismatch for ${ARCHIVE_NAME}. Expected ${EXPECTED_HASH}, got ${ACTUAL_HASH}."
        fi
        success "Checksum verified"
    else
        warn "No checksum file available; skipping verification."
    fi

    log "Download completed successfully"

    # Extract binary from archive
    log "Extracting binary..."
    cd "${TEMP_DIR}"
    tar -xzf "${ARCHIVE_NAME}"

    # Install binary directly to local bin
    mkdir -p "${LOCAL_BIN}"

    # Move binary directly to local bin instead of DDx home
    mv "${BINARY_NAME}" "${LOCAL_BIN}/ddx"
    chmod +x "${LOCAL_BIN}/ddx"

    success "CLI tool installed (${TAG})"
}

# Set up shell completions
setup_completions() {
    log "Setting up shell completions..."
    
    # Detect shell
    SHELL_NAME=$(basename "$SHELL")
    
    case "$SHELL_NAME" in
        bash)
            COMPLETION_FILE="$HOME/.bash_completion"
            if [ -f "$COMPLETION_FILE" ]; then
                echo "# DDx completions" >> "$COMPLETION_FILE"
                echo "eval \"\$(ddx completion bash)\"" >> "$COMPLETION_FILE"
            fi
            ;;
        zsh)
            COMPLETION_DIR="$HOME/.zsh/completions"
            mkdir -p "$COMPLETION_DIR"
            ddx completion zsh > "$COMPLETION_DIR/_ddx" 2>/dev/null || true
            ;;
        fish)
            COMPLETION_DIR="$HOME/.config/fish/completions"
            mkdir -p "$COMPLETION_DIR"
            ddx completion fish > "$COMPLETION_DIR/ddx.fish" 2>/dev/null || true
            ;;
    esac
    
    success "Shell completions configured"
}

# Add to PATH if needed
update_path() {
    log "Checking PATH configuration..."
    
    # Local bin path
    LOCAL_BIN="${HOME}/.local/bin"
    
    # Check if already in PATH
    if [[ ":$PATH:" == *":$LOCAL_BIN:"* ]]; then
        success "PATH is already configured"
        return
    fi
    
    # Add to shell rc file
    SHELL_NAME=$(basename "$SHELL")
    case "$SHELL_NAME" in
        bash)
            RC_FILE="$HOME/.bashrc"
            ;;
        zsh)
            RC_FILE="$HOME/.zshrc"
            ;;
        fish)
            RC_FILE="$HOME/.config/fish/config.fish"
            ;;
        *)
            RC_FILE="$HOME/.profile"
            ;;
    esac
    
    if [ -f "$RC_FILE" ]; then
        echo "" >> "$RC_FILE"
        echo "# DDx CLI PATH" >> "$RC_FILE"
        echo "export PATH=\"\$PATH:$LOCAL_BIN\"" >> "$RC_FILE"
        success "Added DDx to PATH in $RC_FILE"
    else
        warn "Could not find shell config file. Please add $LOCAL_BIN to your PATH manually."
    fi
}

# Verify installation
verify_installation() {
    log "Verifying installation..."

    # Check if binary exists and is executable
    LOCAL_BIN="${HOME}/.local/bin/ddx"
    if [ ! -f "${LOCAL_BIN}" ] || [ ! -x "${LOCAL_BIN}" ]; then
        error "Installation failed: DDx binary not found or not executable at ${LOCAL_BIN}"
    fi

    # Test binary execution
    if ! "${LOCAL_BIN}" version &> /dev/null; then
        warn "DDx binary installed but 'ddx version' command failed. This may be normal if PATH is not yet configured."
    fi

    success "Installation verification completed"
}

# Show getting started information
show_getting_started() {
    echo ""
    echo "🎉 DDx (Document-Driven Development eXperience) installed successfully!"
    echo ""
    echo "📚 Getting Started:"
    echo "   ddx --help           Show available commands"
    echo "   ddx init             Initialize DDx in a project"
    echo "   ddx list             Show available templates and patterns"
    echo "   ddx doctor           Check installation and diagnose issues"
    echo ""
    echo "📖 Documentation:"
    echo "   ${DDX_REPO}          Online repository and documentation"
    echo ""
    echo "🔧 Binary Location:"
    echo "   ${HOME}/.local/bin/ddx    DDx executable"
    echo ""
    echo "⚡ Quick Start:"
    echo "   cd your-project"
    echo "   ddx init"
    echo ""
    
    if command -v ddx &> /dev/null; then
        success "DDx is ready to use! Run 'ddx --version' to verify."
    else
        warn "Please restart your shell or run 'source ~/.${SHELL_NAME}rc' to use ddx command."
    fi
}

# Main installation flow
main() {
    echo "🚀 Installing DDx - Document-Driven Development eXperience"
    echo ""
    
    check_prerequisites
    setup_ddx_directory
    install_cli
    setup_completions
    update_path
    verify_installation
    show_getting_started
}

# Run installation
main "$@"
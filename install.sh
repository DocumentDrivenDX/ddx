#!/bin/bash

# DDx (Document-Driven Development eXperience) Installation Script
# Usage: curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
#        ./install.sh --from-build cli/build/ddx

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
DDX_REPO="https://github.com/DocumentDrivenDX/ddx"
DDX_API="https://api.github.com/repos/DocumentDrivenDX/ddx"
INSTALL_PREFIX="${HOME}/.local"
INSTALL_SOURCE=""
INSTALL_TAG=""
SKIP_SHELL_SETUP="0"

# Logging functions (all to stderr to avoid polluting command substitution)
log() {
    echo -e "${BLUE}[DDx]${NC} $1" >&2
}

success() {
    echo -e "${GREEN}[DDx]${NC} $1" >&2
}

warn() {
    echo -e "${YELLOW}[DDx]${NC} $1" >&2
}

file_sha256() {
    if command -v sha256sum &> /dev/null; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum &> /dev/null; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        return 1
    fi
}

error() {
    echo -e "${RED}[DDx]${NC} $1" >&2
    exit 1
}

usage() {
    cat >&2 <<'EOF'
DDx installer

Usage:
  install.sh [options]

Options:
  --from-build [PATH]  Install an already-built ddx binary. Defaults to cli/build/ddx.
  --prefix PATH       Install under PATH/bin/ddx. Defaults to $HOME/.local.
  --version VERSION   Install release VERSION (same as DDX_VERSION).
  --no-shell          Skip shell completions and PATH rc-file updates.
  -h, --help          Show this help.

Canonical local install path:
  $HOME/.local/bin/ddx

Examples:
  curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
  make build
  ./install.sh --from-build
  ./install.sh --from-build cli/build/ddx --prefix "$HOME/.local"
EOF
}

parse_args() {
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --from-build)
                if [ "$#" -gt 1 ] && [ "${2#-}" = "$2" ]; then
                    INSTALL_SOURCE="$2"
                    shift 2
                else
                    INSTALL_SOURCE="cli/build/ddx"
                    shift
                fi
                ;;
            --prefix)
                [ "$#" -gt 1 ] || error "--prefix requires a path"
                INSTALL_PREFIX="$2"
                shift 2
                ;;
            --version)
                [ "$#" -gt 1 ] || error "--version requires a version"
                INSTALL_TAG="$2"
                shift 2
                ;;
            --no-shell)
                SKIP_SHELL_SETUP="1"
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                ;;
        esac
    done

    if [ -n "$INSTALL_TAG" ]; then
        DDX_VERSION="$INSTALL_TAG"
        export DDX_VERSION
    fi
}

cleanup_stale_ddx_copies() {
    CANONICAL_DDX="${INSTALL_PREFIX}/bin/ddx"
    [ -x "$CANONICAL_DDX" ] || return 0

    CANONICAL_HASH=$(file_sha256 "$CANONICAL_DDX" 2>/dev/null || true)
    if [ -z "$CANONICAL_HASH" ]; then
        warn "No sha256sum or shasum found; skipping stale ddx PATH cleanup."
        return 0
    fi

    log "Checking for stale ddx copies on PATH..."

    LEGACY_GOPATH_BIN=""
    if command -v go &> /dev/null; then
        GOPATH_VALUE=$(go env GOPATH 2>/dev/null || true)
        if [ -n "$GOPATH_VALUE" ]; then
            LEGACY_GOPATH_BIN="${GOPATH_VALUE}/bin"
        fi
    elif [ -n "${GOPATH:-}" ]; then
        LEGACY_GOPATH_BIN="${GOPATH}/bin"
    else
        LEGACY_GOPATH_BIN="${HOME}/go/bin"
    fi

    SCAN_PATH="${PATH}"
    if [ -n "$LEGACY_GOPATH_BIN" ]; then
        SCAN_PATH="${SCAN_PATH}:${LEGACY_GOPATH_BIN}"
    fi

    REMOVED=0
    SKIPPED=0
    SEEN=":"
    IFS=':' read -r -a PATH_DIRS <<< "$SCAN_PATH"
    for DIR in "${PATH_DIRS[@]}"; do
        [ -n "$DIR" ] || continue
        CANDIDATE="${DIR}/ddx"
        case "$SEEN" in
            *":${CANDIDATE}:"*) continue ;;
        esac
        SEEN="${SEEN}${CANDIDATE}:"

        [ -f "$CANDIDATE" ] && [ -x "$CANDIDATE" ] || continue
        if [ "$CANDIDATE" = "$CANONICAL_DDX" ]; then
            continue
        fi

        CANDIDATE_HASH=$(file_sha256 "$CANDIDATE" 2>/dev/null || true)
        if [ -z "$CANDIDATE_HASH" ] || [ "$CANDIDATE_HASH" = "$CANONICAL_HASH" ]; then
            continue
        fi

        if rm -f "$CANDIDATE"; then
            warn "Removed stale ddx copy: ${CANDIDATE}"
            REMOVED=$((REMOVED + 1))
        else
            warn "Could not remove stale ddx copy: ${CANDIDATE}"
            SKIPPED=$((SKIPPED + 1))
        fi
    done

    if [ "$REMOVED" -eq 0 ] && [ "$SKIPPED" -eq 0 ]; then
        success "No stale ddx copies found"
    elif [ "$SKIPPED" -gt 0 ]; then
        warn "Removed ${REMOVED} stale ddx copy/copies; ${SKIPPED} could not be removed"
    else
        success "Removed ${REMOVED} stale ddx copy/copies"
    fi
}

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."

    if [ -z "$INSTALL_SOURCE" ]; then
        # Check for git
        if ! command -v git &> /dev/null; then
            error "Git is required but not installed. Please install git first."
        fi

        # Check for basic utilities (curl/wget for downloading binaries)
        if ! command -v curl &> /dev/null && ! command -v wget &> /dev/null; then
            error "curl or wget is required but neither is installed."
        fi
    fi

    success "Prerequisites check passed"
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
    LOCAL_BIN="${INSTALL_PREFIX}/bin"
    EXISTING_VERSION=""
    if [ -x "${LOCAL_BIN}/ddx" ]; then
        EXISTING_VERSION=$("${LOCAL_BIN}/ddx" version 2>/dev/null | head -1 | awk '{print $2}' || echo "")
    fi

    if [ -n "$EXISTING_VERSION" ]; then
        log "Upgrading DDx from ${EXISTING_VERSION}..."
    else
        log "Installing DDx CLI tool..."
    fi

    mkdir -p "${LOCAL_BIN}"

    if [ -n "$INSTALL_SOURCE" ]; then
        if [ ! -f "$INSTALL_SOURCE" ]; then
            error "Build artifact not found: ${INSTALL_SOURCE}. Run 'make build' first or pass --from-build PATH."
        fi
        if [ ! -x "$INSTALL_SOURCE" ]; then
            error "Build artifact is not executable: ${INSTALL_SOURCE}"
        fi
        log "Installing DDx from build artifact: ${INSTALL_SOURCE}"
        install -m 0755 "${INSTALL_SOURCE}" "${LOCAL_BIN}/ddx"
        SOURCE_HASH=$(file_sha256 "${INSTALL_SOURCE}" 2>/dev/null || true)
        INSTALLED_HASH=$(file_sha256 "${LOCAL_BIN}/ddx" 2>/dev/null || true)
        if [ -z "$SOURCE_HASH" ] || [ -z "$INSTALLED_HASH" ]; then
            SOURCE_HASH=""
            INSTALLED_HASH=""
            warn "No sha256sum or shasum found; skipping local binary verification."
        fi
        if [ -n "$SOURCE_HASH" ] && [ "$SOURCE_HASH" != "$INSTALLED_HASH" ]; then
            error "Installed binary hash mismatch. Expected ${SOURCE_HASH}, got ${INSTALLED_HASH}."
        fi
        cleanup_stale_ddx_copies
        success "CLI tool installed from build artifact"
        return
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
    # Install binary directly to local bin instead of DDx home.
    install -m 0755 "${BINARY_NAME}" "${LOCAL_BIN}/ddx"
    cleanup_stale_ddx_copies

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
            if [ -f "$COMPLETION_FILE" ] && ! grep -Fq 'ddx completion bash' "$COMPLETION_FILE"; then
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

    LOCAL_BIN="${INSTALL_PREFIX}/bin"

    # Add to shell rc file
    SHELL_NAME=$(basename "$SHELL")
    case "$SHELL_NAME" in
        bash)
            RC_FILE="$HOME/.bashrc"
            PATH_SNIPPET="case \":\$PATH:\" in *\":$LOCAL_BIN:\"*) ;; *) export PATH=\"$LOCAL_BIN:\$PATH\" ;; esac"
            ;;
        zsh)
            RC_FILE="$HOME/.zshrc"
            PATH_SNIPPET="case \":\$PATH:\" in *\":$LOCAL_BIN:\"*) ;; *) export PATH=\"$LOCAL_BIN:\$PATH\" ;; esac"
            ;;
        fish)
            RC_FILE="$HOME/.config/fish/config.fish"
            PATH_SNIPPET="contains \"$LOCAL_BIN\" \$PATH; or set -gx PATH \"$LOCAL_BIN\" \$PATH"
            ;;
        *)
            RC_FILE="$HOME/.profile"
            PATH_SNIPPET="case \":\$PATH:\" in *\":$LOCAL_BIN:\"*) ;; *) export PATH=\"$LOCAL_BIN:\$PATH\" ;; esac"
            ;;
    esac

    mkdir -p "$(dirname "$RC_FILE")"
    touch "$RC_FILE"

    TMP_RC=$(mktemp)
    awk -v bin="$LOCAL_BIN" '
        /^# DDx CLI PATH$/ { skip_legacy = 1; next }
        /^# DDx CLI PATH:START$/ { skip_block = 1; next }
        /^# DDx CLI PATH:END$/ { skip_block = 0; next }
        skip_block { next }
        skip_legacy && index($0, bin) > 0 && index($0, "PATH") > 0 { skip_legacy = 0; next }
        $0 == "export PATH=\"" bin ":$PATH\"" { next }
        $0 == "export PATH=\"$PATH:" bin "\"" { next }
        $0 == "PATH=\"" bin ":$PATH\"" { next }
        $0 == "PATH=\"$PATH:" bin "\"" { next }
        { skip_legacy = 0; print }
    ' "$RC_FILE" > "$TMP_RC"
    mv "$TMP_RC" "$RC_FILE"

    {
        echo ""
        echo "# DDx CLI PATH:START"
        echo "$PATH_SNIPPET"
        echo "# DDx CLI PATH:END"
    } >> "$RC_FILE"

    if [[ ":$PATH:" == *":$LOCAL_BIN:"* ]]; then
        success "PATH already contains $LOCAL_BIN; ensured idempotent shell configuration in $RC_FILE"
    else
        success "Added DDx to PATH in $RC_FILE"
    fi
}

# Verify installation
verify_installation() {
    log "Verifying installation..."

    # Check if binary exists and is executable
    LOCAL_DDX="${INSTALL_PREFIX}/bin/ddx"
    if [ ! -f "${LOCAL_DDX}" ] || [ ! -x "${LOCAL_DDX}" ]; then
        error "Installation failed: DDx binary not found or not executable at ${LOCAL_DDX}"
    fi

    # Test binary execution
    if ! "${LOCAL_DDX}" version &> /dev/null; then
        warn "DDx binary installed but 'ddx version' command failed. This may be normal if PATH is not yet configured."
    fi

    success "Installation verification completed"
}

# Show getting started information
show_getting_started() {
    echo ""
    echo "🎉 DDx (Document-Driven Development eXperience) installed successfully!"
    echo ""
    echo "📚 Next Steps:"
    echo "   ddx install --global Install skills to ~/.ddx/ and set up symlinks"
    echo "   ddx init             Initialize DDx in a project"
    echo "   ddx install helix    Install a workflow plugin into your project"
    echo "   ddx doctor           Check installation and diagnose issues"
    echo ""
    echo "📖 Documentation:"
    echo "   ${DDX_REPO}          Online repository and documentation"
    echo ""
    echo "🔧 Binary Location:"
    echo "   ${INSTALL_PREFIX}/bin/ddx    DDx executable"
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
    parse_args "$@"

    echo "🚀 Installing DDx - Document-Driven Development eXperience"
    echo ""

    check_prerequisites
    install_cli
    if [ "$SKIP_SHELL_SETUP" != "1" ]; then
        setup_completions
        update_path
    fi
    verify_installation
    show_getting_started
}

# Run installation
main "$@"

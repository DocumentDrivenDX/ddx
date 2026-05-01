#!/usr/bin/env bash
# Development environment setup script for DDx
# Checks for required tools and assists with installation

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Track if any tools are missing
MISSING_TOOLS=0
OPTIONAL_MISSING=0

echo "🔍 Checking development environment..."
echo ""

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to print status
print_status() {
    local tool=$1
    local required=$2
    local installed=$3
    local install_cmd=$4

    if [ "$installed" = "true" ]; then
        echo -e "${GREEN}✓${NC} $tool is installed"
    else
        if [ "$required" = "true" ]; then
            echo -e "${RED}✗${NC} $tool is ${RED}REQUIRED${NC} but not installed"
            echo -e "  ${BLUE}Install:${NC} $install_cmd"
            ((MISSING_TOOLS++))
        else
            echo -e "${YELLOW}○${NC} $tool is optional but not installed"
            echo -e "  ${BLUE}Install:${NC} $install_cmd"
            ((OPTIONAL_MISSING++))
        fi
    fi
}

# Check Go
if command_exists go; then
    GO_VERSION=$(go version | awk '{print $3}')
    echo -e "${GREEN}✓${NC} Go is installed ($GO_VERSION)"
else
    echo -e "${RED}✗${NC} Go is ${RED}REQUIRED${NC} but not installed"
    echo -e "  ${BLUE}Install:${NC} https://go.dev/doc/install"
    ((MISSING_TOOLS++))
fi

# Check golangci-lint
if command_exists golangci-lint; then
    GOLANGCI_VERSION=$(golangci-lint --version | head -n1 | awk '{print $4}')
    echo -e "${GREEN}✓${NC} golangci-lint is installed ($GOLANGCI_VERSION)"
else
    print_status "golangci-lint" "true" "false" "curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$(go env GOPATH)/bin"
fi

# Check lefthook
if command_exists lefthook; then
    LEFTHOOK_VERSION=$(lefthook version 2>/dev/null || echo "unknown")
    echo -e "${GREEN}✓${NC} lefthook is installed ($LEFTHOOK_VERSION)"
else
    print_status "lefthook" "true" "false" "go install github.com/evilmartians/lefthook@latest"
fi

# Check git
if command_exists git; then
    GIT_VERSION=$(git --version | awk '{print $3}')
    echo -e "${GREEN}✓${NC} git is installed ($GIT_VERSION)"
else
    echo -e "${RED}✗${NC} git is ${RED}REQUIRED${NC} but not installed"
    ((MISSING_TOOLS++))
fi

# Optional tools
echo ""
echo "📦 Optional tools:"

print_status "gitleaks" "false" "$(command_exists gitleaks && echo true || echo false)" "brew install gitleaks (macOS) or https://github.com/gitleaks/gitleaks#installing"
print_status "yq" "false" "$(command_exists yq && echo true || echo false)" "brew install yq (macOS) or https://github.com/mikefarah/yq#install"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Summary
if [ $MISSING_TOOLS -eq 0 ]; then
    echo -e "${GREEN}✓ All required tools are installed!${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Install git hooks: lefthook install"
    echo "  2. Build the CLI: cd cli && make build"
    echo "  3. Run tests: make test"
else
    echo -e "${RED}✗ Missing $MISSING_TOOLS required tool(s)${NC}"
    echo ""
    echo "Install the required tools above and run this script again."
    exit 1
fi

if [ $OPTIONAL_MISSING -gt 0 ]; then
    echo ""
    echo -e "${YELLOW}Note: $OPTIONAL_MISSING optional tool(s) not installed${NC}"
    echo "These tools enable additional checks but are not required for development."
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
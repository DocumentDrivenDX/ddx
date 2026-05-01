#!/usr/bin/env bash
# DDx Installation Demo — shows install command and post-install verification
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_lib.sh"

# Show the install command (display only — binary is pre-mounted in Docker)
echo ""
echo "$ curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash"
echo "🚀 Installing DDx - Document-Driven Development eXperience"
echo "  ✓ Binary installed to ~/.local/bin/ddx"
sleep 1

# Show version
type_command ddx version

# Show doctor
type_command ddx doctor

# Show help
type_command ddx --help


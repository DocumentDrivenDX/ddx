#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_lib.sh
source "$SCRIPT_DIR/_lib.sh"

# Show install command (echo only — no network required)
type_command echo 'curl -fsSL https://ddx.dev/install.sh | bash'

# Show version
type_command ddx version

# Show top-level help
type_command ddx --help

#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_lib.sh
source "$SCRIPT_DIR/_lib.sh"

type_command ddx search workflow

# Network-dependent — failure is expected in offline environments
echo ""
echo "$ ddx install helix"
sleep 0.5
ddx install helix || echo "(install skipped — no network or repo unavailable)"
sleep 1

type_command ddx installed

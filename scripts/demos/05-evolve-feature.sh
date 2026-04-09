#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_lib.sh
source "$SCRIPT_DIR/_lib.sh"

setup_demo_dir

ddx init --silent --skip-claude-injection > /dev/null 2>&1

EPIC_ID=$(ddx bead create "Build task tracker" --type epic --labels "helix,phase:build" 2>/dev/null)
ddx bead create "Add task CRUD" --type task --labels "helix,phase:build" > /dev/null 2>&1

# Create the new bead and capture its ID for dep commands
NEW_ID=$(ddx bead create "Add task priorities" --type task --labels "helix,phase:build" 2>/dev/null)
echo ""
echo "$ ddx bead create \"Add task priorities\" --type task --labels \"helix,phase:build\""
echo "$NEW_ID"
sleep 1

type_command ddx bead dep add "$NEW_ID" "$EPIC_ID"

type_command ddx bead dep tree "$EPIC_ID"

type_command ddx bead list

cleanup_demo_dir

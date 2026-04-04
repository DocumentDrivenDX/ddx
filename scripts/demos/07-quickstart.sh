#!/usr/bin/env bash
# DDx Quickstart — single recording covering install → init → helix → use
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_lib.sh"

setup_demo_dir

# Install & init
type_command ddx init
type_command ddx doctor

# Install HELIX
type_command ddx search workflow
type_command ddx install helix
type_command ddx installed

# Create some beads
type_command ddx bead create "Design authentication system" --type epic --labels "helix,phase:frame"
EPIC_ID=$(ddx bead list --json 2>/dev/null | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

type_command ddx bead create "Implement login endpoint" --type task --labels "helix,phase:build" --set "spec-id=FEAT-001"
TASK_ID=$(ddx bead list --json 2>/dev/null | grep -o '"id":"[^"]*"' | tail -1 | cut -d'"' -f4)

# Wire dependency
if [ -n "$EPIC_ID" ] && [ -n "$TASK_ID" ]; then
  type_command ddx bead dep add "$TASK_ID" "$EPIC_ID"
fi

# Inspect
type_command ddx bead list
type_command ddx bead ready

# Check agent harnesses
type_command ddx agent list

cleanup_demo_dir

#!/usr/bin/env bash
# DDx Quickstart — real install experience: download → init → helix → bead → agent
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_lib.sh"

# Use the pre-mounted binary (simulates post-install state)
export GIT_TEMPLATE_DIR=""

# Show the install command (display only — binary is pre-mounted in Docker)
echo ""
echo "$ curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash"
echo "🚀 Installing DDx - Document-Driven Development eXperience"
echo "  ✓ Binary installed to ~/.local/bin/ddx"
sleep 1

type_command ddx version

# Set up a demo project
DEMO_DIR=$(mktemp -d)
cd "$DEMO_DIR"
GIT_TEMPLATE_DIR="" git init -q
git config user.email "demo@ddx.dev"
git config user.name "DDx Demo"
echo "# My Project" > README.md
git add . && git commit -q -m "init"

# Init DDx in the project (copies bootstrap skills to .ddx/, .agents/, .claude/)
type_command ddx init
type_command ls -la .ddx/skills/
type_command ls -la .agents/skills/

# Install HELIX workflow (project-scoped)
type_command ddx install helix
type_command ddx installed

# Create some beads
type_command ddx bead create "Design authentication system" --type epic --priority 1 --labels "helix,phase:frame" --acceptance "Auth design doc approved"
EPIC_ID=$(ddx bead list --json 2>/dev/null | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

type_command ddx bead create "Implement login endpoint" --type task --priority 2 --labels "helix,phase:build" --set "spec-id=FEAT-001" --acceptance "POST /login returns JWT"
TASK_ID=$(ddx bead list --json 2>/dev/null | grep -o '"id":"[^"]*"' | tail -1 | cut -d'"' -f4)

if [ -n "$EPIC_ID" ] && [ -n "$TASK_ID" ]; then
  type_command ddx bead dep add "$TASK_ID" "$EPIC_ID"
fi

type_command ddx bead list
type_command ddx bead ready
type_command ddx bead status

# Show agent harnesses
type_command ddx agent list

cd /
rm -rf "$DEMO_DIR"

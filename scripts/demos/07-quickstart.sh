#!/usr/bin/env bash
# DDx Quickstart — bootstrap → beads → work loop
# Covers: ddx init, ddx install helix, bead create, ddx work draining the queue
# with agent dispatch visible. Uses the script harness so the demo runs without
# any external API keys; agent-dispatch lines mirror real session-log output.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_lib.sh"

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

# Bootstrap
type_command ddx init
type_command ddx install helix
type_command ddx installed

# Create the work
type_command ddx bead create "Design auth system" --type epic --priority 1 \
  --labels "helix,phase:frame" --acceptance "Auth design doc approved"
type_command ddx bead create "Implement login endpoint" --type task --priority 2 \
  --labels "helix,phase:build" --set "spec-id=FEAT-001" \
  --acceptance "POST /login returns JWT"
type_command ddx bead create "Add login regression test" --type task --priority 2 \
  --labels "helix,phase:test" --set "spec-id=FEAT-001" \
  --acceptance "e2e test asserts JWT round-trip"

type_command ddx bead ready
type_command ddx bead status

# Drain the queue. The script harness produces deterministic commits so the
# recording exercises the full layer-3 path (claim → execute → land → review)
# without external services.
type_command ddx work --local --harness script --no-review

type_command ddx bead status
type_command git log --oneline -3

cd /
rm -rf "$DEMO_DIR"

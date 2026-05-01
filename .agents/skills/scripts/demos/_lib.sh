#!/usr/bin/env bash
# Shared helpers for DDx demo recordings

# Terminal size for consistent recordings
export COLUMNS=80
export LINES=24

# Create and enter temp directory
setup_demo_dir() {
  DEMO_DIR=$(mktemp -d)
  cd "$DEMO_DIR"
  git init -q
  git config user.email "demo@ddx.dev"
  git config user.name "DDx Demo"
  echo "# Demo Project" > README.md
  git add . && git commit -q -m "init"
}

cleanup_demo_dir() {
  cd /
  rm -rf "$DEMO_DIR"
}

# Simulate typing with delay for readability
type_command() {
  echo ""
  echo "$ $*"
  sleep 0.5
  "$@"
  sleep 1
}

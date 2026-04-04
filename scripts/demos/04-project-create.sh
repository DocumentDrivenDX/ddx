#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_lib.sh
source "$SCRIPT_DIR/_lib.sh"

setup_demo_dir

type_command ddx init

type_command ddx bead create "Build task tracker" --type epic --labels "helix,phase:build"

type_command ddx bead create "Add task CRUD" --type task --labels "helix,phase:build"

type_command ddx bead list

type_command ddx bead ready

cleanup_demo_dir

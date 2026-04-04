#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_lib.sh
source "$SCRIPT_DIR/_lib.sh"

setup_demo_dir

type_command ddx init

type_command ddx list

type_command ddx doctor

type_command ddx persona list

cleanup_demo_dir

#!/usr/bin/env bash
# build-fixture-repo.sh — create a clean ddx-initialized git repo for tests / demos.
#
# Usage:
#   scripts/build-fixture-repo.sh <dest> [--profile minimal|standard|multi-project|federated]
#
# Env:
#   DDX_BIN   path to the ddx binary used for seeding (default: ddx from PATH)
#
# Cleanup is the caller's responsibility (rm -rf <dest>).

set -euo pipefail

PROFILE="minimal"
DEST=""

usage() {
  cat <<'USAGE'
Usage: build-fixture-repo.sh <dest> [--profile minimal|standard|multi-project|federated]

Profiles:
  minimal        empty .ddx/ (config + empty beads.jsonl), no seeded data
  standard       .ddx/ + 5 mixed-priority sample beads
  multi-project  two registered projects (proj-a, proj-b); proj-a seeded
  federated      hub/ + spoke/ projects (each ddx-initialized; federation handshake not registered)

Env:
  DDX_BIN  path to the ddx binary (default: ddx from PATH)
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile) PROFILE="$2"; shift 2;;
    --profile=*) PROFILE="${1#--profile=}"; shift;;
    -h|--help) usage; exit 0;;
    --) shift; break;;
    -*) echo "build-fixture-repo: unknown flag: $1" >&2; usage >&2; exit 2;;
    *)
      if [[ -z "$DEST" ]]; then
        DEST="$1"; shift
      else
        echo "build-fixture-repo: extra positional arg: $1" >&2; exit 2
      fi
      ;;
  esac
done

if [[ -z "$DEST" ]]; then
  echo "build-fixture-repo: <dest> is required" >&2
  usage >&2
  exit 2
fi

DDX_BIN="${DDX_BIN:-ddx}"
if ! command -v "$DDX_BIN" >/dev/null 2>&1; then
  echo "build-fixture-repo: ddx binary not found (DDX_BIN=$DDX_BIN); set DDX_BIN or add ddx to PATH" >&2
  exit 3
fi

# Resolve to absolute path so cd-ing around doesn't break us.
mkdir -p "$DEST"
DEST="$(cd "$DEST" && pwd)"

# create_project <abs-dir>
create_project() {
  local dir="$1"
  mkdir -p "$dir/.ddx"
  (
    cd "$dir"
    git init -q -b main
    git config user.email "fixture@ddx.test"
    git config user.name  "DDx Fixture"

    cat > .ddx/config.yaml <<'YAML'
version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/DocumentDrivenDX/ddx-library
    branch: main
YAML
    : > .ddx/beads.jsonl

    cat > .gitignore <<'EOF'
.ddx/agent-logs/
.ddx/workers/
.ddx/exec-runs.d/
.ddx/.execute-bead-wt-*/
.ddx/executions/*/embedded/
.ddx/executions/**/scratch/
.ddx/*.lock
.ddx/*.tmp
.ddx/server.env
.ddx/server/
EOF

    git add .ddx/config.yaml .gitignore
    git commit -q -m "chore: init fixture project"
  )
}

# seed_standard_beads <abs-dir>: adds 5 mixed-priority sample beads + commits.
seed_standard_beads() {
  local dir="$1"
  (
    cd "$dir"
    "$DDX_BIN" bead create "Sample: lint cleanup"        --priority 1 --labels "phase:1,kind:chore"  --description "Pretend lint cleanup."   --acceptance "lint passes" >/dev/null
    "$DDX_BIN" bead create "Sample: add docs"            --priority 2 --labels "phase:1,kind:docs"   --description "Pretend docs write-up." --acceptance "doc exists"  >/dev/null
    "$DDX_BIN" bead create "Sample: refactor API"        --priority 0 --labels "phase:2,kind:refactor" --description "Pretend refactor."    --acceptance "tests green" >/dev/null
    "$DDX_BIN" bead create "Sample: write tests"         --priority 2 --labels "phase:1,kind:test"   --description "Pretend new tests."     --acceptance "coverage up" >/dev/null
    "$DDX_BIN" bead create "Sample: investigate flake"   --priority 3 --labels "phase:2,kind:bug"    --description "Pretend flake hunt."    --acceptance "rootcause"  >/dev/null

    git add .ddx/beads.jsonl
    git commit -q -m "chore: seed sample beads"
  )
}

case "$PROFILE" in
  minimal)
    create_project "$DEST"
    ;;
  standard)
    create_project "$DEST"
    seed_standard_beads "$DEST"
    ;;
  multi-project)
    create_project "$DEST/proj-a"
    create_project "$DEST/proj-b"
    seed_standard_beads "$DEST/proj-a"
    ;;
  federated)
    create_project "$DEST/hub"
    create_project "$DEST/spoke"
    seed_standard_beads "$DEST/hub"
    ;;
  *)
    echo "build-fixture-repo: unknown profile: $PROFILE" >&2
    usage >&2
    exit 2
    ;;
esac

echo "fixture ready: $DEST (profile=$PROFILE)"

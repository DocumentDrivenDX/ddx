#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
allowlist="$root/docs/dev/legacy-routing-docs-allowlist.tsv"

pattern='\bddx agent (run|execute-bead|execute-loop)\b|--harnesses\b|--quorum\b|--profile\b|profile_ladders|model_overrides|ResolveRoute|RouteDecision'

scan_paths=(
  "docs/agent-execute.md"
  "docs/dev"
  "docs/migrations"
  "docs/helix/00-discover"
  "docs/helix/01-frame"
  "docs/helix/02-design"
  "docs/helix/03-test/test-plans"
  "docs/helix/tests"
  "website/content/docs/cli/commands"
)

if [[ ! -f "$allowlist" ]]; then
  echo "missing allowlist: $allowlist" >&2
  exit 2
fi

validate_allowlist() {
  local row=0
  local path match reason extra
  while IFS=$'\t' read -r path match reason extra || [[ -n "${path:-}" ]]; do
    row=$((row + 1))
    [[ -z "${path:-}" || "${path:0:1}" == "#" ]] && continue
    if [[ -n "${extra:-}" || -z "${path:-}" || -z "${match:-}" || -z "${reason:-}" ]]; then
      echo "invalid allowlist row $row: expected path<TAB>matching text<TAB>reason" >&2
      exit 2
    fi
  done < "$allowlist"
}

is_allowed() {
  local hit_path="$1"
  local hit_text="$2"
  local path match reason

  while IFS=$'\t' read -r path match reason || [[ -n "${path:-}" ]]; do
    [[ -z "${path:-}" || "${path:0:1}" == "#" ]] && continue
    if [[ "$hit_path" == "$path" && "$hit_text" == *"$match"* ]]; then
      return 0
    fi
  done < "$allowlist"

  return 1
}

validate_allowlist

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

set +e
(
  cd "$root"
  rg -n --glob '!docs/dev/legacy-routing-docs-allowlist.tsv' -e "$pattern" "${scan_paths[@]}"
) > "$tmp"
rg_status=$?
set -e

if [[ "$rg_status" -eq 1 ]]; then
  exit 0
fi
if [[ "$rg_status" -ne 0 ]]; then
  cat "$tmp" >&2
  exit "$rg_status"
fi

unallowed=0
while IFS=: read -r path line_no text || [[ -n "${path:-}" ]]; do
  [[ -z "${path:-}" ]] && continue
  if ! is_allowed "$path" "$text"; then
    if [[ "$unallowed" -eq 0 ]]; then
      echo "unallowlisted legacy routing docs references:" >&2
    fi
    printf '%s:%s:%s\n' "$path" "$line_no" "$text" >&2
    unallowed=$((unallowed + 1))
  fi
done < "$tmp"

if [[ "$unallowed" -ne 0 ]]; then
  echo >&2
  echo "Add a narrow row to docs/dev/legacy-routing-docs-allowlist.tsv only for historical, migration, or explicit deprecation references." >&2
  exit 1
fi

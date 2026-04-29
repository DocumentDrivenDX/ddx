#!/usr/bin/env bash
# Migrate HELIX tracker JSONL from old format to bd-compatible bead format.
#
# Old format (scripts/tracker.sh):
#   type, assignee, deps (flat array), created, updated,
#   spec-id, execution-eligible, superseded-by, replaces, design,
#   claimed-at, claimed-pid
#
# New format (ddx bead / bd-compatible):
#   issue_type, owner, dependencies (structured array), created_at, updated_at,
#   + extra fields preserved as-is (spec-id, execution-eligible, etc.)
#
# Usage:
#   bash scripts/migrate-tracker-to-beads.sh [path/to/.helix/issues.jsonl]
#
# Writes migrated data in-place (backs up original to .bak first).
#
set -euo pipefail

input="${1:-.helix/issues.jsonl}"

if [[ ! -f "$input" ]]; then
  echo "migrate: file not found: $input" >&2
  exit 1
fi

lines="$(wc -l < "$input")"
if (( lines == 0 )); then
  echo "migrate: empty file, nothing to do"
  exit 0
fi

# Detect format: if first record has "issue_type", already migrated
first_field="$(head -1 "$input" | jq -r 'has("issue_type") // false')"
if [[ "$first_field" == "true" ]]; then
  echo "migrate: already in new format (has issue_type field)"
  exit 0
fi

# Backup
cp "$input" "${input}.bak"
echo "migrate: backed up to ${input}.bak"

# Transform each line
jq -c '
  # Map flat deps array to structured dependencies
  (if (.deps | type) == "array" and (.deps | length) > 0
   then [.deps[] | {issue_id: "", depends_on_id: ., type: "blocks"}]
   else []
   end) as $deps_array |

  # Build new record
  {
    id:            .id,
    title:         .title,
    issue_type:    (.type // "task"),
    status:        .status,
    priority:      (.priority // 2),
    owner:         (.assignee // ""),
    created_at:    (.created // .created_at // ""),
    updated_at:    (.updated // .updated_at // ""),
    labels:        (.labels // []),
    parent:        (.parent // ""),
    description:   (.description // ""),
    acceptance:    (.acceptance // ""),
    notes:         (.notes // ""),
    dependencies:  $deps_array
  }
  # Preserve HELIX-specific extra fields
  + (if .["spec-id"] then {"spec-id": .["spec-id"]} else {} end)
  + (if .["execution-eligible"] != null then {"execution-eligible": .["execution-eligible"]} else {} end)
  + (if .["superseded-by"] then {"superseded-by": .["superseded-by"]} else {} end)
  + (if .replaces then {replaces: .replaces} else {} end)
  + (if .design then {design: .design} else {} end)
  + (if .["claimed-at"] then {"claimed-at": .["claimed-at"]} else {} end)
  + (if .["claimed-pid"] then {"claimed-pid": .["claimed-pid"]} else {} end)
  + (if .created_by then {created_by: .created_by} else {} end)
' "$input" > "${input}.tmp"

mv "${input}.tmp" "$input"

migrated="$(wc -l < "$input")"
echo "migrate: converted $migrated issues from old tracker format to bd-compatible bead format"
echo "migrate: original backed up to ${input}.bak"

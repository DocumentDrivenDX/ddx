#!/usr/bin/env bash
# Generate the concerns catalog page from workflows/concerns/ source files.
# Run from repo root: bash website/scripts/generate-concerns.sh
set -euo pipefail

CONCERNS_DIR="${1:-workflows/concerns}"
OUTPUT="${2:-website/content/docs/glossary/concerns.md}"

cat > "$OUTPUT" <<'HEADER'
---
title: Concerns
weight: 4
prev: /docs/glossary/actions
next: /docs/glossary/concepts
---

<!-- AUTO-GENERATED from workflows/concerns/ — do not edit manually -->
<!-- Regenerate with: bash website/scripts/generate-concerns.sh -->

# Concerns

Concerns are composable cross-cutting declarations from a shared library. They encode technology choices, quality requirements, and conventions that apply across multiple beads and phases.

## How Concerns Work

1. **Select** — During [Frame](/docs/glossary/phases#phase-1-frame), declare active concerns in `docs/helix/01-frame/concerns.md`
2. **Filter** — Each concern declares which areas it applies to (`all`, `ui`, `api`, `data`, `infra`, `cli`)
3. **Inject** — At execution time, area-matched concerns and their practices are loaded into context
4. **Digest** — [Context digests](/docs/glossary/concepts#context-digest) carry concern practices into beads, making them self-contained

## Project Concerns File

Every HELIX project declares its concerns in `docs/helix/01-frame/concerns.md`:

```markdown
# Project Concerns

## Active Concerns
- rust-cargo (tech-stack)
- security-owasp (security)

## Area Labels
| Label | Applies to |
|-------|-----------|
| all   | Every bead |
| api   | Server, endpoints |
| cli   | CLI tool |

## Project Overrides
### rust-cargo
- **MSRV**: 1.75 (lower than library default)
```

Project overrides take full precedence over library defaults.

## Concern Library

The library lives at `workflows/concerns/`. Each concern has two files:

- `concern.md` — Category, areas, components, constraints, quality gates
- `practices.md` — Phase-specific practices (requirements, design, implementation, testing)

HEADER

# Collect concerns by category
declare -A categories
declare -A concern_data

for dir in "$CONCERNS_DIR"/*/; do
  [[ -f "$dir/concern.md" ]] || continue
  name=$(basename "$dir")
  title=$(head -1 "$dir/concern.md" | sed 's/^# Concern: //')
  category=$(awk '/^## Category/{getline; print; exit}' "$dir/concern.md")
  areas=$(awk '/^## Areas/{getline; print; exit}' "$dir/concern.md")

  # Normalize category for grouping
  case "$category" in
    tech-stack)       group="Tech Stack" ;;
    security)         group="Security" ;;
    observability)    group="Observability" ;;
    accessibility)    group="Accessibility" ;;
    internationalization) group="Internationalization" ;;
    infrastructure)   group="Infrastructure" ;;
    microsite|demo)   group="Tooling" ;;
    testing)          group="Testing" ;;
    quality-attribute) group="Quality" ;;
    *)                group="Other" ;;
  esac

  # Extract key tools from first component line (strip markdown formatting)
  focus=$(awk '/^## Components/{found=1; next} found && /^- \*\*/{gsub(/^- \*\*[^*]+\*\*: */,""); print; exit}' "$dir/concern.md" 2>/dev/null || true)
  [[ -z "$focus" ]] && focus="$title"
  # Truncate long focus lines
  focus="${focus:0:80}"

  # Build table row
  categories["$group"]+="| **$name** | $areas | $focus |"$'\n'
done

# Output categories in a sensible order
for group in "Tech Stack" "Security" "Observability" "Accessibility" "Internationalization" "Quality" "Testing" "Infrastructure" "Tooling"; do
  [[ -z "${categories[$group]:-}" ]] && continue
  echo "### $group Concerns"
  echo ""
  echo "| Concern | Areas | Key Tools |"
  echo "|---------|-------|-----------|"
  echo -n "${categories[$group]}"
  echo ""
done >> "$OUTPUT"

cat >> "$OUTPUT" <<'FOOTER'

## Drift Signals

Tech-stack concerns can declare **drift signals** — patterns that indicate the project is straying from its declared technology choices. For example, the `typescript-bun` concern flags:

- `npm run` instead of `bun run`
- `prettier` or `eslint` instead of Biome
- `vitest` or `jest` instead of `bun:test`
- `@hono/node-server` instead of `Bun.serve()`
- `engines.node` in package.json

[Review](/docs/glossary/actions#review) and [align](/docs/glossary/actions#align) check for drift signals and report them as findings.

## Concerns in the Knowledge Chain

Concerns connect to other HELIX artifacts in a knowledge chain:

```
Spike/POC (gather evidence)
  → ADR (record decision with rationale)
    → Concern (index for context assembly)
      → Context Digest (injected into beads)
```

When a referenced ADR is superseded, [polish](/docs/glossary/actions#polish) flags the affected concern for re-evaluation.

## Where Concerns Are Used

Every HELIX action that involves technology or quality choices loads active concerns:

| Action | How it uses concerns |
|--------|---------------------|
| **build** | Loads practices, runs concern-declared quality gates |
| **review** | Checks for drift signals and practice violations |
| **design** | Concerns constrain architecture decisions |
| **evolve** | Detects technology changes conflicting with concerns |
| **align** | Flags concern drift across all layers (docs, designs, code) |
| **polish** | Enforces area labels, refreshes context digests, fixes tool references |
| **frame** | Concern selection happens during framing |
| **check** | Detects missing area labels, stale digests, missing concerns.md |
| **backfill** | Discovers concerns from project evidence |
FOOTER

echo "Generated $OUTPUT with $(grep -c '^\*\*' "$OUTPUT" 2>/dev/null || echo '?') concerns"

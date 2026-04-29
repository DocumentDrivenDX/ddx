---
name: helix-backfill
description: Run the HELIX documentation backfill action. Use when the user wants `helix backfill` behavior.
argument-hint: "[scope]"
disable-model-invocation: true
---

# Backfill

Execute the HELIX documentation backfill action.

## Steps

1. Read and apply `.ddx/plugins/helix/workflows/actions/backfill-helix-docs.md`.
2. Use `$ARGUMENTS` as the scope when provided.
3. Reconstruct only what the evidence supports.
4. Emit the required metadata markers, including `BACKFILL_STATUS` and `BACKFILL_REPORT`.

## Constraints

- Do not fabricate missing planning artifacts.
- Separate confirmed evidence from inference.
- Create follow-on tracker work when gaps need human guidance.

# Bead status migration survey

**Bead:** ddx-e3f25fdb (housekeeping sibling of ddx-673833f4 / TD-031)
**Date:** 2026-05-03
**Goal:** Enumerate distinct persisted `status` values across all bead stores within reach to determine whether non-canonical statuses (outside the bd/br-compatible 6: `open`, `in_progress`, `closed`, `blocked`, `proposed`, `cancelled`) exist and require migration.

## Method

For each bead store, ran:

```
ddx jq -r '.status' <path> | sort | uniq -c
```

`.bak` backup files were skipped (point-in-time copies of the active store).

## Results

### DDx (this repo) — active `.ddx/beads.jsonl`

| count | status      |
|------:|-------------|
|   154 | closed      |
|     1 | in_progress |
|   126 | open        |

### DDx (this repo) — archive `.ddx/beads-archive.jsonl`

| count | status      |
|------:|-------------|
|  1166 | closed      |

### Fizeau (`~/Projects/fizeau/.ddx/beads.jsonl`)

| count | status      |
|------:|-------------|
|   712 | closed      |
|     1 | in_progress |
|    18 | open        |

No archive present (`~/Projects/fizeau/.ddx/beads-archive.jsonl` does not exist).

### Axon (`~/Projects/axon/.ddx/beads.jsonl`)

| count | status      |
|------:|-------------|
|   974 | closed      |
|    23 | open        |

No archive present (`~/Projects/axon/.ddx/beads-archive.jsonl` does not exist).

### Cross-repo target ~/Projects/agent

The bead description names `~/Projects/agent (Fizeau bead store)`. That path does not exist on this machine; Fizeau lives at `~/Projects/fizeau` and was surveyed under that name above. Treating the description's `~/Projects/agent` as a labeling slip for `~/Projects/fizeau`.

## Distinct statuses observed across all surveyed stores

`closed`, `in_progress`, `open` — three values, all within the bd/br-compatible 6-value canonical set codified in `cli/internal/bead/types.go` and TD-031 (per ADR-004).

## Conclusion

**No migration needed.** Every persisted status across every surveyed store (active + archive in this repo, plus active stores in `~/Projects/fizeau` and `~/Projects/axon`) is already within the canonical bd/br-compatible enum. No non-canonical statuses were observed, so no child migration bead is being filed.

The names that appear in code (`done`, `needs_human`, `pending`, `ready`, `review`, `needs_investigation`) are — per TD-031 — event kinds, derived queue categories, terminal phases, or labels, not persisted bead `status` values, and the survey confirms they are not leaking into the persisted enum.

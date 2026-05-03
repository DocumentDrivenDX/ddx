# REACH-BACKFILL decomposition

Bead: ddx-83440482 — checks: systematic backfill — apply production-reachability across cli/ and resolve all violations

## Why decomposed (Step 0 size check)

This bead's contract is a sweep: file children per cluster, drain them, achieve zero violations. The bead's own description prescribes the workflow ("File one child bead per violation cluster… Drain the children"). A single execute-bead attempt cannot land 283 wire/delete decisions across 21 packages and still pass the bead's AC. Per the executor instructions: "A clean decomposition is a successful attempt."

## How violations were enumerated

The installed `production-reachability` check is diff-scoped (it flags only newly-added unreached symbols in the current diff range), so a base==head ac run reports `pass — no diff range`. For backfill we need an exhaustive scan, which is exactly what the underlying engine — `golang.org/x/tools/cmd/deadcode` (RTA) — produces. Run from `cli/`:

    go run golang.org/x/tools/cmd/deadcode@v0.42.0 -json ./...

That output is preserved verbatim at `initial-violations.json` and flattened to `initial-violations.tsv` (one row per unreached symbol: `package\tfile\tline\tsymbol`).

Total unreached symbols: **283** across **21** packages.

## Cluster grouping

One child bead per Go package (sensible, matches blame boundaries, keeps each child small enough to drain in a single attempt). Per-package counts:

| Child bead | Package | Count |
| --- | --- | --- |
| ddx-0131ebf0 | cmd | 69 |
| ddx-83b8994f | internal/agent | 66 |
| ddx-895fd8bb | internal/server/perf | 17 |
| ddx-c96fc86c | internal/persona | 16 |
| ddx-a78f836f | internal/testutils | 15 |
| ddx-2850c4dc | internal/metric | 14 |
| ddx-90901b22 | internal/server | 12 |
| ddx-503c34fa | internal/git | 9 |
| ddx-2da07e5c | internal/agent/try | 9 |
| ddx-ae4b7393 | internal/evidence | 8 |
| ddx-91fe7a1a | internal/config | 8 |
| ddx-b42dd3a0 | internal/bead | 8 |
| ddx-9df0636c | internal/escalation | 6 |
| ddx-cd51f35b | internal/agent/testfixtures | 6 |
| ddx-4c5beab2 | internal/server/graphql | 5 |
| ddx-9f6baafe | internal/exec | 5 |
| ddx-abb40ce5 | internal/agent/escalation | 4 |
| ddx-7f4cdb7a | internal/update | 2 |
| ddx-8c273456 | internal/federation | 2 |
| ddx-d0d8d615 | internal/registry | 1 |
| ddx-a7fac0fc | internal/metaprompt | 1 |

Each child carries the per-package symbol list, the WIRE-or-DELETE decision rule from the parent, and a per-package AC (zero remaining unreached symbols + tests green).

Dependency edges: parent ddx-83440482 depends on all 21 children, so `ddx bead ready` will not surface the parent until every cluster lands.

## Cross-bead linkage

- **WIRE bead ddx-09d2990c** (already closed prior to this run) was annotated with a supersession note pointing at ddx-83440482 / ddx-83b8994f. Its 4 originally-cited unwired functions are confirmed still dead in the current scan and are absorbed into the `internal/agent` child:
  - `internal/agent/escalation/ladder.go:93` — `Ladder.Next`
  - `internal/agent/retry_policy.go:113` — `EvaluateRetryPolicy`
  - `internal/agent/triage_dispatch.go:170` — `RunWithLockBackoff`
  - `internal/agent/triage_dispatch.go:278` — `DispatchPostAttempt`
- The Codex audit additions named in the parent description (jsonlFallbackForCollection, Store.breakStaleLock, NewStoreWithBackend, WithSpokeHTTPClient, Server.execStore, GraphQL NewResolver) are subsumed by the per-package children for `internal/bead`, `internal/federation`, `internal/server`, and `internal/server/graphql`.

## What still has to happen for the parent to close

1. Each of the 21 children drained (each lands a wire or delete change for the symbols it owns; tests green; per-package deadcode count = 0 for that package).
2. Final `ddx ac run --check production-reachability` against the post-merge tip emits `status=pass` with no violations.
3. Final summary written to `.ddx/executions/<final-run-id>/reach-backfill-summary.md` with the AC#7 fields (total / wired / deleted / follow-ups / LOC delta).

That final summary belongs to the last run that closes the parent — not this attempt.

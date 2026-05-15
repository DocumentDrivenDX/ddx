# Reach Backfill Summary

## Closure Verdict

`ddx-83440482` is **not close-ready** on 2026-05-15.

Current-tree deadcode RTA still reports residual live violations in `cli/`, so the parent epic must stay open until the newly-filed follow-up beads land.

## Evidence Used

- Final current-tree artifact: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`
- Orphan-pending audit: `rg -n -P '//\s*wiring:pending(?!\s+ddx-)' cli` returned no hits
- Queue check: `ddx work plan --limit=0` now skips `ddx-83440482` as a ready epic because it again has open children

## Counts

- Total violations found in the current-tree final artifact: `235`
- Wired count in this verification pass: `0`
- Deleted count in this verification pass: `0`
- Pending count in this verification pass: `0`
- Follow-up beads filed from this verification pass: `16`

Historical context:

- The parent epic notes the original 2026-05-03 sweep found `283` total dead symbols.
- The current-tree residual count is `235`, so `48` symbols have been removed from the dead set since the original sweep.
- This verification bead did not reclassify those `48` into WIRE vs DELETE because it only performed closure evidence capture and follow-up filing.

## Residual Package Breakdown

- `cmd`: `39`
- `internal/agent`: `136`
- `internal/artifacttypes`: `3`
- `internal/bead`: `14`
- `internal/bead/axon`: `12`
- `internal/ddxroot`: `1`
- `internal/docprose`: `2`
- `internal/registry`: `1`
- `internal/server`: `24`
- `internal/testutils`: `3`

## Follow-up Beads Filed

- `ddx-d1fce33d` — `checks: residual production-reachability — cmd runtime helpers (5 unreached)`
- `ddx-b6859802` — `checks: residual production-reachability — cmd test helpers`
- `ddx-bb8e7d07` — `checks: residual production-reachability — internal/agent candidate cycle`
- `ddx-5baa6a15` — `checks: residual production-reachability — internal/agent compare and benchmark`
- `ddx-2f964bac` — `checks: residual production-reachability — internal/agent execute-bead flow`
- `ddx-496a9346` — `checks: residual production-reachability — internal/agent support utilities`
- `ddx-97375097` — `checks: residual production-reachability — internal/agent runtime support`
- `ddx-01cce920` — `checks: residual production-reachability — internal/artifacttypes`
- `ddx-bd927304` — `checks: residual production-reachability — internal/bead core`
- `ddx-8bc79046` — `checks: residual production-reachability — internal/bead axon subscription`
- `ddx-a7a11433` — `checks: residual production-reachability — internal/ddxroot`
- `ddx-dea7f96b` — `checks: residual production-reachability — internal/docprose`
- `ddx-c6b07648` — `checks: residual production-reachability — internal/registry installer`
- `ddx-f89eb4ec` — `checks: residual production-reachability — internal/server review session`
- `ddx-83d662a9` — `checks: residual production-reachability — internal/server review prompt`
- `ddx-b273b31f` — `checks: residual production-reachability — internal/server providers and workers`
- `ddx-17c54930` — `checks: residual production-reachability — internal/testutils fixture repo`

## Gate Results

- `cd cli && go test ./internal/bead -run TestReadyExecutionBreakdown_ClassifiesEpicClosureCandidates -count=1` passed
- `lefthook run pre-commit` passed
- `cd cli && go test ./...` failed before any `Test*` executed because `cli/internal/server/embed.go:5` embeds `all:frontend/build`, but `cli/frontend/` is absent in this worktree

Separate build-gate bead filed for the unrelated full-suite failure:

- `ddx-039d932b` — `build: restore frontend/build for go test setup`

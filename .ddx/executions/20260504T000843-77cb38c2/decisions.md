# Decisions log — ddx-abb40ce5

Backfill of production-reachability violations in package `internal/agent/escalation` (4 listed symbols).

All four symbols live in a single file (`cli/internal/agent/escalation/ladder.go`) that is imported by no production code. The package was added by ddx-51ac997b (story:10 escalation work) but the wiring step was scheduled as a separate bead ddx-cfedee8e (open; "wire ladder into executor closure; drop tier→profile mutation"). The audit ddx-09d2990c (closed) documented this as a process failure: features built but never wired to production.

Decision rule applied: WIRE if a production caller exists or could be added within the scope of this backfill bead; DELETE if not. WIRE is unavailable here because the wiring is an explicit, scoped refactor (ddx-cfedee8e) — doing it inside this backfill bead would exceed scope. The PEND-with-annotation path was tried first but rejected because AC #3 of this bead requires `deadcode` RTA (the bare CLI, which does not honor `// wiring:pending`) to report zero remaining symbols in the package; PEND-only would leave AC #3 unsatisfied.

| Symbol | Decision | Rationale |
| --- | --- | --- |
| internal/agent/escalation/ladder.go:32 — NoViableProviderError.Error | DELETE | no production caller; recreated by ddx-cfedee8e |
| internal/agent/escalation/ladder.go:48 — NewLadder | DELETE | no production caller; recreated by ddx-cfedee8e |
| internal/agent/escalation/ladder.go:70 — Ladder.Tiers | DELETE | no production caller; recreated by ddx-cfedee8e |
| internal/agent/escalation/ladder.go:93 — Ladder.Next | DELETE | no production caller; recreated by ddx-cfedee8e |

## Files changed

- Deleted `cli/internal/agent/escalation/ladder.go` (107 LOC).
- Deleted `cli/internal/agent/escalation/ladder_test.go` (226 LOC).
- Empty package directory `cli/internal/agent/escalation/` removed.

## Originating-bead bookkeeping

- ddx-51ac997b reopened (status: closed → open). Its AC ("Ladder type + Next() in cli/internal/agent/escalation/, unit tests, skip-tier NoViableProviderError, tests use RoutingActual.Power") is unsatisfied again after deletion. The AC is still the right design — re-satisfaction will land as part of ddx-cfedee8e's wiring work.
- ddx-cfedee8e (open) is the natural home for re-creating the ladder primitive together with its production callsite, since the bead's existing scope is "wire the ladder (S10_2) into singleTierAttempt / executor closure". No scope expansion required: it now owns both creation and wiring instead of just wiring.

## Verification

- `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` (from cli/): zero remaining `internal/agent/escalation/...` entries (AC #3 satisfied).
- `go test ./internal/...` (from cli/): all packages green.
- `go test ./...` (from cli/): all packages green except 7 pre-existing `cmd/` failures (TestReviewEvidenceApproveAttributesToTier, TestAcceptance_US028..US031, TestAcceptance_US034, TestInstallationPerformance) — confirmed failing on base rev f0617096 prior to this change.

## Pending follow-ups

None. No `// wiring:pending` annotations were retained.

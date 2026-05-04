# ddx-2da07e5c decisions log

Backfill production-reachability for `internal/agent/try` package.
9 symbols decided per bead's AC. Status legend: WIRE | DELETE | PENDING.

| File:line | Symbol | Decision | Reason |
|---|---|---|---|
| internal/agent/try/attempt.go:67 | Attempt | DELETE | Speculative C2 shell wrapping ExecuteBeadWithConfig + LandBeadResult + RunPostMergeReview. Originating bead ddx-6db5d14a AC #6 explicitly: "No production callers yet." Caller migration was deferred to C3+; C3 (ddx-a921ff01, closed) extracted RunPostMergeReview as a free function and wired the legacy loop directly to it (execute_bead_loop.go:840), bypassing try.Attempt. Per sibling backfill bead ddx-83b8994f precedent (66 unused symbols deleted including identical "introduced ahead of callers" cases), DELETE. |
| internal/agent/try/attempt.go:144 | reportFromResult | DELETE | Helper for Attempt; deleted with Attempt. |
| internal/agent/try/outcome.go:29 | Disposition.String | DELETE | Method on the Disposition vocabulary type that has no production consumers. Originating bead ddx-240f2082 AC #6: "No callers of try.Outcome yet (introduced in C2)." |
| internal/agent/try/outcome.go:71 | AllParkReasons | DELETE | Vocabulary enumeration helper with no production consumers (only used by ParkReasonValid below + tests). |
| internal/agent/try/outcome.go:90 | ParkReasonValid | DELETE | Validator on speculative ParkReason vocabulary; only consumer was ToOutcome (also deleted). |
| internal/agent/try/outcome.go:99 | ParkReason.String | DELETE | Method on speculative vocabulary type with no production consumers. |
| internal/agent/try/outcome.go:123 | dispositionToStatus | DELETE | Internal helper for FromOutcome; deleted with FromOutcome. |
| internal/agent/try/outcome.go:159 | FromOutcome | DELETE | Half of the round-trip adapter pair; both sides have no production consumers. |
| internal/agent/try/outcome.go:182 | ToOutcome | DELETE | Half of the round-trip adapter pair; both sides have no production consumers. |

## Resolution

Whole package `cli/internal/agent/try/` deleted (attempt.go, attempt_test.go, outcome.go, outcome_test.go).

## Why DELETE over WIRE/PENDING

The C-series refactor (parent ddx-5cb6e6cd, OPEN) intends to migrate the orchestrator to consume try.Outcome / try.Attempt across beads C4–C14 (all OPEN). Wiring the symbols now would require pulling forward C4–C7 work (move conflict recovery, no-changes adjudication, push-failed/conflict, Guard contract) into this backfill bead — well outside its scope.

`// wiring:pending` annotation alone does not satisfy AC #3 ("deadcode RTA reports zero remaining dead symbols in `internal/agent/try`") since deadcode RTA does not honor source annotations.

DELETE satisfies AC #3 trivially (no symbols → no dead symbols) and matches the consistent precedent from sibling backfill bead ddx-83b8994f. The C4–C14 beads can recreate the package as part of their own slices when they actually wire callers; their descriptions are self-contained.

## Verification

- `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from cli/ — no remaining symbols in `internal/agent/try` (package no longer exists).
- `cd cli && go test ./...` — green.

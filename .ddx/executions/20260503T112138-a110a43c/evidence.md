# C0 baseline fixture evidence — ddx-b52fc7f2

Foundation bead for the execute-bead refactor (proposal §6.1 R4).

## Deliverables

- `cli/internal/agent/refactor_baseline_test.go` — `TestRefactorBaseline_FixturesStable`
  loads canonical event sequences for each lifecycle from `refactorBaselineScenarios`,
  re-encodes them with `json.Marshal` (sorted keys), and asserts byte-identity against
  the golden JSONL fixture on disk. A second subtest fails the build if a `.jsonl`
  file lands without a matching scenario.
- `cli/internal/agent/testdata/refactor_baseline/*.jsonl` — 11 fixtures, one per
  canonical lifecycle:
  - `merged_success_close.jsonl`
  - `success_review_approve_close.jsonl`
  - `success_review_block_reopen_triage.jsonl`
  - `land_conflict_recover.jsonl`
  - `no_changes_verified.jsonl`
  - `no_changes_unjustified.jsonl`
  - `decomposition.jsonl`
  - `push_failed.jsonl`
  - `push_conflict.jsonl`
  - `preserved_needs_review.jsonl`
  - `default_failure.jsonl`
- `/tmp/execute-bead-refactor-proposal.md` §6.1 R4 — cross-reference paragraph
  appended pointing C2-C8 children at the fixture set.

## Acceptance criteria mapping

1. ✅ `cli/internal/agent/testdata/refactor_baseline/` populated with 11 .jsonl files.
2. ✅ Each fixture sequences the canonical event kinds: routing, execute-bead,
   review (where applicable), reopen (where applicable), triage-decision (where
   applicable), loop-error (where applicable), land-conflict-auto-recovered,
   decomposition-recommendation, push-conflict, bead.result. The
   `refactorBaselineExpectedKinds` map encodes the required ordering and is
   asserted by every subtest.
3. ✅ `TestRefactorBaseline_FixturesStable` confirms byte-identity by
   re-marshalling the in-code scenario through `encoding/json` (which sorts
   map keys deterministically) and `bytes.Equal` against the on-disk fixture.
4. ✅ `go test -run TestRefactorBaseline ./internal/agent/` green
   (12 subtests pass).
5. ✅ Cross-reference added to /tmp/execute-bead-refactor-proposal.md §6.1 R4.

## Regenerating

Intentional event-shape changes regenerate via:

```
UPDATE_BASELINE=1 go test -run TestRefactorBaseline ./internal/agent/
```

Followed by committing the updated `.jsonl` files alongside the producing change.

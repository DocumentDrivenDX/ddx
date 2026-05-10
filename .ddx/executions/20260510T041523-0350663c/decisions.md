# Decisions: internal/escalation dead-symbol backfill (ddx-9df0636c)

## Original 6 symbols from bead description

All 6 were already wired into the production call graph before this execution ran.
The initial-violations.json was captured 2026-05-03; callers were added between
then and 2026-05-10.

| Symbol | File | Decision | Reason |
|--------|------|----------|--------|
| ShouldEscalate | escalation.go:30 | WIRE (already wired) | Called from cmd/agent_execute_loop_escalation.go:134 and internal/server/workers.go:676 |
| FormatTierAttemptBody | escalation.go:44 | WIRE (already wired) | Called from internal/server/workers.go:695 |
| BuildEscalationSummary | escalation.go:89 | WIRE (already wired) | Called internally by AppendEscalationSummaryEvent and from internal/agent/zero_config_escalation_integration_test.go |
| AppendEscalationSummaryEvent | escalation.go:118 | WIRE (already wired) | Called from internal/server/workers.go:929 |
| IsInfrastructureFailure | infrastructure.go:58 | WIRE (already wired) | Called from cmd/agent_execute_loop_escalation.go:134 and internal/server/workers.go:676 |
| CostCapTracker.Spent | infrastructure.go:185 | WIRE (already wired) | Called from internal/server/workers.go:883 and cmd/agent_cmd.go:1659 |

## Additional dead symbol found by deadcode (not in original bead list)

| Symbol | File | Decision | Reason |
|--------|------|----------|--------|
| TierToProfile | tierinfer.go:148 | DELETE | ModelTier is a string type whose values ("smart","standard","cheap") already match routing profile names. Production callers use string(tier) directly (see cmd/try.go:269,273). TierToProfile was a redundant identity wrapper — design moved on when ModelTier was defined as a string alias. Removed function and updated unit test (TestTierToProfile) and integration test (zero_config_escalation_integration_test.go:37). |

## Verification

```
go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...
# zero results for internal/escalation

go test ./internal/escalation/... ./internal/agent/... ./cmd/...
# all green
```

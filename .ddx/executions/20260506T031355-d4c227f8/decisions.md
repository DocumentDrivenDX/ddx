# ddx-9df0636c decisions

`go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from `cli/` does not report any unreachable symbols in `internal/escalation` on this tree.

- WIRE `ShouldEscalate` via `cli/cmd/agent_execute_loop_escalation.go:56` and `cli/internal/server/workers.go:655`, both reachable from `cli/main.go:31`.
- WIRE `FormatTierAttemptBody` via `cli/internal/server/workers.go:674`.
- WIRE `BuildEscalationSummary` via `cli/internal/escalation/escalation.go:122`, which feeds `AppendEscalationSummaryEvent` in the production worker path.
- WIRE `AppendEscalationSummaryEvent` via `cli/internal/server/workers.go:882`.
- WIRE `IsInfrastructureFailure` via `cli/cmd/agent_execute_loop_escalation.go:56` and `cli/internal/server/workers.go:655`.
- WIRE `CostCapTracker.Spent` via `cli/internal/server/workers.go:836`.

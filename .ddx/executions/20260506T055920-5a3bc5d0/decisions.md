# Decisions

- WIRE `internal/escalation/escalation.go:30` `ShouldEscalate` - reached from the production execute-loop in `cli/cmd/agent_execute_loop_escalation.go:56` and `cli/internal/server/workers.go:655`, both reachable from `cli/main.go:15` via `cmd.Execute`.
- WIRE `internal/escalation/escalation.go:44` `FormatTierAttemptBody` - reached from the production tier-attempt event path in `cli/internal/server/workers.go:674`.
- WIRE `internal/escalation/escalation.go:89` `BuildEscalationSummary` - reached from `AppendEscalationSummaryEvent` in `internal/escalation/escalation.go:118`, which is called by the production execute-loop in `cli/internal/server/workers.go:887`.
- WIRE `internal/escalation/escalation.go:118` `AppendEscalationSummaryEvent` - called by the production execute-loop in `cli/internal/server/workers.go:887`.
- WIRE `internal/escalation/infrastructure.go:58` `IsInfrastructureFailure` - reached from the production execute-loop in `cli/cmd/agent_execute_loop_escalation.go:56` and `cli/internal/server/workers.go:655`.
- WIRE `internal/escalation/infrastructure.go:185` `CostCapTracker.Spent` - reached from the production cost-cap path in `cli/cmd/agent_cmd.go:1777`.

`internal/escalation/escalation.go:30` `ShouldEscalate` - WIRE: used by the production execute-loop in `cli/cmd/agent_execute_loop_escalation.go` and `cli/internal/server/workers.go`.
`internal/escalation/escalation.go:44` `FormatTierAttemptBody` - WIRE: used by the production tier-attempt event writer in `cli/internal/server/workers.go`.
`internal/escalation/escalation.go:89` `BuildEscalationSummary` - WIRE: used by `AppendEscalationSummaryEvent` in production event emission.
`internal/escalation/escalation.go:118` `AppendEscalationSummaryEvent` - WIRE: called from the production execute-loop in `cli/internal/server/workers.go`.
`internal/escalation/infrastructure.go:58` `IsInfrastructureFailure` - WIRE: used by the production execute-loop gate in `cli/cmd/agent_execute_loop_escalation.go` and `cli/internal/server/workers.go`.
`internal/escalation/infrastructure.go:185` `CostCapTracker.Spent` - WIRE: used by the production cost-cap handling in `cli/cmd/agent_cmd.go` and `cli/internal/server/workers.go`.

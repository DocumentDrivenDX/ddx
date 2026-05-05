ddx-9df0636c decisions

WIRE internal/escalation/escalation.go:30 ShouldEscalate — called from cli/cmd/agent_execute_loop_escalation.go:56 and cli/internal/server/workers.go:701 to gate tier escalation.
WIRE internal/escalation/escalation.go:44 FormatTierAttemptBody — called from cli/internal/server/workers.go:720 to format tier-attempt events.
WIRE internal/escalation/escalation.go:89 BuildEscalationSummary — called from internal/escalation/escalation.go:122 by AppendEscalationSummaryEvent, which is emitted from cli/internal/server/workers.go:928.
WIRE internal/escalation/escalation.go:118 AppendEscalationSummaryEvent — called from cli/internal/server/workers.go:928 after execute-loop attempts complete.
WIRE internal/escalation/infrastructure.go:58 IsInfrastructureFailure — called from cli/cmd/agent_execute_loop_escalation.go:56 and cli/internal/server/workers.go:701 to defer infrastructure failures instead of escalating them.
WIRE internal/escalation/infrastructure.go:185 CostCapTracker.Spent — called from cli/cmd/agent_cmd.go:1765, cli/internal/server/workers.go:882, and cli/internal/agent/execute_bead_post_review.go:196 to report capped spend.

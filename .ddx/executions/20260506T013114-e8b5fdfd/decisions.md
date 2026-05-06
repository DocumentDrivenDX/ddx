WIRE internal/escalation/escalation.go:30 ShouldEscalate via cli/cmd/agent_execute_loop_escalation.go:56 and cli/internal/server/workers.go:655 from main() reachable execute-loop paths.
WIRE internal/escalation/escalation.go:44 FormatTierAttemptBody via cli/internal/server/workers.go:674 from main() reachable tier-attempt event emission.
WIRE internal/escalation/escalation.go:89 BuildEscalationSummary via cli/internal/server/workers.go:882 and cli/internal/agent/zero_config_escalation_integration_test.go:92; production summary emission is on the main() path.
WIRE internal/escalation/escalation.go:118 AppendEscalationSummaryEvent via cli/internal/server/workers.go:882 from the worker escalation summary path.
WIRE internal/escalation/infrastructure.go:58 IsInfrastructureFailure via cli/cmd/agent_execute_loop_escalation.go:56 and cli/internal/server/workers.go:655 from main() reachable execute-loop paths.
WIRE internal/escalation/infrastructure.go:185 CostCapTracker.Spent via cli/internal/server/workers.go:836 and cli/internal/agent/execute_bead_post_review.go:118 in production cost-cap telemetry.

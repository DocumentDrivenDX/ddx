WIRE internal/escalation/escalation.go:30 ShouldEscalate - already used by the server and CLI escalation loops to gate tier escalation.
WIRE internal/escalation/escalation.go:44 FormatTierAttemptBody - now used when the server worker records per-attempt tier events.
WIRE internal/escalation/escalation.go:89 BuildEscalationSummary - reached through production AppendEscalationSummaryEvent after each escalation run.
WIRE internal/escalation/escalation.go:118 AppendEscalationSummaryEvent - now called from the server worker escalation trail emission path.
WIRE internal/escalation/infrastructure.go:58 IsInfrastructureFailure - already used by production retry/escalation decision paths.
WIRE internal/escalation/infrastructure.go:185 CostCapTracker.Spent - used by production cost-cap checks to format cap-trip details.

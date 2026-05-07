WIRE internal/escalation/escalation.go:30, internal/server/workers.go:655, and cmd/agent_execute_loop_escalation.go:56 keep ShouldEscalate on the production execute-loop path.
WIRE internal/escalation/escalation.go:44 is used by internal/server/workers.go:674 to emit tier-attempt bead events.
WIRE internal/escalation/escalation.go:89 is reachable through internal/escalation/escalation.go:118 and internal/server/workers.go:887 when the worker emits escalation summaries.
WIRE internal/escalation/escalation.go:118 is called from internal/server/workers.go:887 to append escalation-summary bead events.
WIRE internal/escalation/infrastructure.go:58 is used by internal/server/workers.go:655 and cmd/agent_execute_loop_escalation.go:56 to defer transient infrastructure failures instead of escalating them.
WIRE internal/escalation/infrastructure.go:185 is used by internal/server/workers.go:841 to report cost-cap spend.

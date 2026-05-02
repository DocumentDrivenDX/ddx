package agent

import agentlib "github.com/DocumentDrivenDX/fizeau"

// Compile-time references to fizeau v0.10.0 fields added by the Role +
// CorrelationID + RoutingActual.Power epic (agent-3bb96bf5). Ensures the
// upstream symbols resolve from DDx Go code. The DDx-facing public type for
// ExecuteRequest is ServiceExecuteRequest.
var (
	_ = agentlib.ServiceExecuteRequest{}.Role
	_ = agentlib.ServiceExecuteRequest{}.CorrelationID
	_ = agentlib.RouteRequest{}.Role
	_ = agentlib.RouteRequest{}.CorrelationID
	_ = agentlib.ServiceRoutingActual{}.Power
	_ = agentlib.RouteDecision{}.Power
)

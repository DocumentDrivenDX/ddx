package config

// AgentPassthrough carries the resolved harness/provider/model routing
// constraints from the operator invocation through to the upstream agent
// service as a single, opaque unit.
type AgentPassthrough struct {
	Harness  string
	Provider string
	Model    string
}

package agent

// RunAgent executes a prompt using the agentlib.FizeauService service path.
// This runs in-process - no subprocess, no binary lookup.
func RunAgent(r *Runner, opts RunArgs) (*Result, error) {
	return runAgentViaService(r, opts)
}

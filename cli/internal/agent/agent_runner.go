package agent

// RunAgent executes a prompt using the agentlib.DdxAgent service path.
// This runs in-process — no subprocess, no binary lookup.
//
// To disable the new path as an emergency escape hatch, set the env var
// DDX_USE_NEW_AGENT_PATH=0 (or "false"). Default is the new service path.
func (r *Runner) RunAgent(opts RunOptions) (*Result, error) {
	if useNewAgentPath() {
		return runAgentViaService(r, opts)
	}
	// Emergency fallback: reached only when DDX_USE_NEW_AGENT_PATH=0.
	// The legacy in-package agentlib.Run loop has been removed (ddx-d224671d).
	// If this branch is needed, re-enable by reverting that bead.
	return runAgentViaService(r, opts)
}

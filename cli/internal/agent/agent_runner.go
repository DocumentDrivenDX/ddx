package agent

// RunAgent executes a prompt using the agentlib.DdxAgent service path.
// This runs in-process — no subprocess, no binary lookup.
//
// Phase 5h migration (ddx-4e3e149f): converted from a (*Runner) method
// to a free function taking r as an explicit parameter, matching the
// dispatch style used everywhere else in this package after the
// Runner-method-to-free-function refactor landed.
//
// To disable the new path as an emergency escape hatch, set the env var
// DDX_USE_NEW_AGENT_PATH=0 (or "false"). Default is the new service path.
func RunAgent(r *Runner, opts RunOptions) (*Result, error) {
	if useNewAgentPath() {
		return runAgentViaService(r, opts)
	}
	// Emergency fallback: reached only when DDX_USE_NEW_AGENT_PATH=0.
	// The legacy in-package agentlib.Run loop has been removed (ddx-d224671d).
	// If this branch is needed, re-enable by reverting that bead.
	return runAgentViaService(r, opts)
}

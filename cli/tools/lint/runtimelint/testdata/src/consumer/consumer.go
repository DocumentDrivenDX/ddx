// Package consumer is outside cli/internal/agent/. Pattern 2
// (composite literal of a legacy options type) fires anywhere in the
// repo; patterns 1 and 3 do not, because they are scoped to the agent
// package.
package consumer

import "agent"

// Pattern 2: composite literals anywhere — flagged.
func patternComposite() {
	_ = agent.RunOptions{Prompt: "x"}              // want `runtimelint: composite literal of legacy options type RunOptions`
	_ = agent.ExecuteBeadLoopOptions{WorkerID: ""} // want `runtimelint: composite literal of legacy options type ExecuteBeadLoopOptions`
	_ = agent.ExecuteBeadOptions{FromRev: ""}      // want `runtimelint: composite literal of legacy options type ExecuteBeadOptions`
	_ = agent.CompareOptions{}                     // want `runtimelint: composite literal of legacy options type CompareOptions`
	_ = agent.QuorumOptions{}                      // want `runtimelint: composite literal of legacy options type QuorumOptions`
}

// Pattern 3 is OUT OF SCOPE here: function declared outside agent pkg
// with a legacy-options param must NOT be flagged.
func paramOutsideAgent(opts agent.RunOptions) {}

// Package allowed exercises the evidence:allow-unbounded annotation:
// every violation here carries a justifying annotation and must NOT
// produce diagnostics.
package allowed

import (
	"os"

	"agent"
)

func pattern1Allowed(opts *agent.RunOptions, dynamic string) {
	// evidence:allow-unbounded reason="test fixture: assignment is intentional"
	opts.Prompt = dynamic
}

func pattern1AllowedSameLine(opts *agent.RunOptions, dynamic string) {
	opts.Prompt = dynamic // evidence:allow-unbounded reason="test fixture inline"
}

func pattern3Allowed(path string) ([]byte, error) {
	// evidence:allow-unbounded reason="test fixture: ReadFile is bounded by caller"
	promptBytes, err := os.ReadFile(path)
	return promptBytes, err
}

// Empty reason="" must NOT suppress — the test for that case lives in
// the violations package via a unit test, not here.

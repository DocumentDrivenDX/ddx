// Package spechonesty hosts the Phase 2 spec-honesty analyzer scaffold.
// The diagnostic rules are implemented by sibling beads; this package
// exposes the analyzer hook that the read-only invariant test exercises.
package spechonesty

import "golang.org/x/tools/go/analysis"

// Analyzer is the spechonesty analyzer entrypoint.
var Analyzer = &analysis.Analyzer{
	Name: "spechonesty",
	Doc:  "phase 2 spec-honesty analyzer scaffold",
	Run: func(*analysis.Pass) (interface{}, error) {
		return nil, nil
	},
}

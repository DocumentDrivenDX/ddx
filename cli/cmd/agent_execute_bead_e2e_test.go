package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteBeadContextBudgetFromConfig proves that the migrated
// `ddx agent execute-bead` dispatch path threads ContextBudget from
// .ddx/config.yaml through config.LoadAndResolve + ExecuteBeadRuntime
// + ExecuteBeadWithConfig into the prompt assembly. SD-024 Stage 3
// behavioral test (bead ddx-d758f207).
//
// Before the migration, agent_execute_bead.go built ExecuteBeadOptions
// directly from CLI flags; evidence_caps.context_budget in
// .ddx/config.yaml had no path to the dispatch site. With the
// migration, LoadAndResolve resolves the durable knob into rcfg and
// ExecuteBeadWithConfig sources opts.ContextBudget from rcfg.
//
// The assertion targets a marker that only appears when ContextBudget
// is "minimal": the bead has no governing refs, and the buildPrompt
// branch for minimal-with-empty-refs emits the short
// "No governing references." note, while the default-with-empty-refs
// branch emits a longer pre-resolved-references note. The presence of
// the short marker (and absence of the long marker) proves the value
// flowed end-to-end from YAML to prompt.
func TestExecuteBeadContextBudgetFromConfig(t *testing.T) {
	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111", // no-changes outcome — keeps the run minimal
	}
	runner := &fakeAgentRunner{result: &agent.Result{
		ExitCode: 0,
		Harness:  "mock-harness",
	}}
	f := newExecuteBeadFactory(t, git, runner)

	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
evidence_caps:
  context_budget: minimal
`
	require.NoError(t, os.WriteFile(filepath.Join(f.WorkingDir, ".ddx", "config.yaml"), []byte(cfg), 0o644))

	_ = runExecuteBead(t, f, git, "my-bead")

	require.NotEmpty(t, runner.last.PromptFile,
		"agent runner must receive the synthesized prompt file path")
	body, err := os.ReadFile(runner.last.PromptFile)
	require.NoError(t, err, "synthesized prompt artifact must be readable")
	prompt := string(body)

	assert.Contains(t, prompt, "No governing references.",
		"minimal-budget prompt must include the short minimal marker")
	assert.NotContains(t, prompt, "were pre-resolved",
		"minimal-budget prompt must not include the default full-budget marker")
}

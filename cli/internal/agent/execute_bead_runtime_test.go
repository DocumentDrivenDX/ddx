package agent

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// capturingAgentRunner records the RunOptions passed to Run so the
// delegation test can assert each durable knob landed on the agent
// invocation.
type capturingAgentRunner struct {
	lastOpts RunOptions
}

func (r *capturingAgentRunner) Run(opts RunOptions) (*Result, error) {
	r.lastOpts = opts
	return &Result{ExitCode: 0}, nil
}

// TestExecuteBeadRuntimeDelegation verifies that ExecuteBeadWithConfig
// threads every execute-bead durable knob from a sealed ResolvedConfig
// through to the agent invocation, and that runtime intent
// (FromRev, WorkerID, AgentRunner) flows from the ExecuteBeadRuntime.
// SD-024 Stage 3.
func TestExecuteBeadRuntimeDelegation(t *testing.T) {
	const beadID = "ddx-rt-delegation-01"

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Harness: "claude",
		Model:   "claude-test-model",
	})
	rcfg := cfg.Resolve(config.CLIOverrides{
		Provider: "anthropic",
		ModelRef: "claude-mref",
		Effort:   "high",
	})

	runner := &capturingAgentRunner{}

	projectRoot := setupArtifactTestProjectRoot(t)
	gitOps := &artifactTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "deadbeef00000001",
		resultRev:   "deadbeef00000001", // no-changes outcome — keeps the run minimal
		wtSetupFn: func(wtPath string) {
			setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
		},
	}

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		WorkerID:    "worker-rt",
		AgentRunner: runner,
	}, gitOps)
	if err != nil {
		t.Fatalf("ExecuteBeadWithConfig: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.WorkerID != "worker-rt" {
		t.Errorf("WorkerID on result = %q, want %q", res.WorkerID, "worker-rt")
	}

	got := runner.lastOpts
	if got.Harness != "claude" {
		t.Errorf("Harness = %q, want %q", got.Harness, "claude")
	}
	if got.Model != "claude-test-model" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-test-model")
	}
	if got.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", got.Provider, "anthropic")
	}
	if got.ModelRef != "claude-mref" {
		t.Errorf("ModelRef = %q, want %q", got.ModelRef, "claude-mref")
	}
	if got.Effort != "high" {
		t.Errorf("Effort = %q, want %q", got.Effort, "high")
	}
}

// TestExecuteBeadRuntimeDelegation_ZeroValueRcfgPanics confirms
// ExecuteBeadWithConfig refuses an unsealed ResolvedConfig — the
// SD-024 sealed-construction invariant flows through to the
// execute-bead path.
func TestExecuteBeadRuntimeDelegation_ZeroValueRcfgPanics(t *testing.T) {
	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("ExecuteBeadWithConfig with zero-value ResolvedConfig must panic via requireSealed")
		}
	}()
	var rcfg config.ResolvedConfig
	_, _ = ExecuteBeadWithConfig(context.Background(), "", "ddx-x", rcfg, ExecuteBeadRuntime{}, nil)
}

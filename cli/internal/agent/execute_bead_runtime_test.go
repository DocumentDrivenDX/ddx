package agent

import (
	"context"
	"os"
	"path/filepath"
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

// TestExecutionsMirrorFromConfig verifies that an ExecutionsConfig.Mirror
// block on *Config flows through Resolve → ResolvedConfig.MirrorConfig() →
// ExecuteBeadWithConfig → ExecuteBead, and the bundle is mirrored to the
// configured destination after the worker writes result.json. SD-024 Stage 3.
func TestExecutionsMirrorFromConfig(t *testing.T) {
	const beadID = "ddx-mirror-cfg-01"

	projectRoot := setupArtifactTestProjectRoot(t)
	mirrorRoot := t.TempDir()
	async := false

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Harness: "claude",
		Model:   "claude-test-model",
		Mirror: &config.ExecutionsMirrorConfig{
			Kind:  "local",
			Path:  filepath.Join(mirrorRoot, "{attempt_id}"),
			Async: &async,
		},
	})
	rcfg := cfg.Resolve(config.CLIOverrides{})

	if rcfg.MirrorConfig() == nil {
		t.Fatal("ResolvedConfig.MirrorConfig() must be non-nil after Resolve")
	}

	gitOps := &artifactTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "cccc000000000001",
		resultRev:   "cccc000000000001",
		wtSetupFn: func(wtPath string) {
			setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
		},
	}

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: &artifactTestAgentRunner{},
	}, gitOps)
	if err != nil {
		t.Fatalf("ExecuteBeadWithConfig: %v", err)
	}
	if res == nil || res.AttemptID == "" {
		t.Fatalf("expected non-nil result with attempt id")
	}

	mirroredManifest := filepath.Join(mirrorRoot, res.AttemptID, "manifest.json")
	if _, err := os.Stat(mirroredManifest); err != nil {
		t.Errorf("expected mirrored manifest at %s: %v", mirroredManifest, err)
	}
	mirroredResult := filepath.Join(mirrorRoot, res.AttemptID, "result.json")
	if _, err := os.Stat(mirroredResult); err != nil {
		t.Errorf("expected mirrored result at %s: %v", mirroredResult, err)
	}

	entries, err := ReadMirrorIndex(projectRoot)
	if err != nil {
		t.Fatalf("ReadMirrorIndex: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.AttemptID == res.AttemptID && e.BeadID == beadID {
			found = true
		}
	}
	if !found {
		raw, _ := os.ReadFile(filepath.Join(projectRoot, ExecutionsMirrorIndexFile))
		t.Errorf("attempt %s not in mirror index. raw=%s", res.AttemptID, raw)
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

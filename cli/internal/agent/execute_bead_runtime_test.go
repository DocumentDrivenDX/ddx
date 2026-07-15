package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/require"
)

// capturingAgentRunner records the RunArgs passed to Run so the
// delegation test can assert each durable knob landed on the agent
// invocation.
type capturingAgentRunner struct {
	lastOpts RunArgs
}

func (r *capturingAgentRunner) Run(opts RunArgs) (*Result, error) {
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
		Model: "claude-test-model",
	})
	rcfg := cfg.Resolve(config.CLIOverrides{
		Harness:  "claude",
		Model:    "claude-test-model",
		Provider: "anthropic",
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
	if got.Effort != "high" {
		t.Errorf("Effort = %q, want %q", got.Effort, "high")
	}
	if got.Env[DDXModeEnvKey] != DDXModeBeadExecution {
		t.Errorf("Env[%q] = %q, want %q", DDXModeEnvKey, got.Env[DDXModeEnvKey], DDXModeBeadExecution)
	}
}

func TestLifecycleClearRoutingPinsRequiresExplicitAutonomousRouteEvidence(t *testing.T) {
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{Model: "gpt-5.4-mini"})
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "codex", Model: "gpt-5.4-mini"})
	runner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		return &Result{ExitCode: 0, Harness: opts.Harness, Model: opts.Model}, nil
	})

	_, err := dispatchViaResolvedConfig(context.Background(), t.TempDir(), nil, runner, rcfg, AgentRunRuntime{
		Prompt:           "lifecycle",
		ClearRoutingPins: true,
	})
	if err == nil {
		t.Fatal("expected pinned lifecycle dispatch without replacement route to fail")
	}
	if !strings.Contains(err.Error(), "cannot clear pinned routing") {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := dispatchViaResolvedConfig(context.Background(), t.TempDir(), nil, runner, rcfg, AgentRunRuntime{
		Prompt:           "lifecycle",
		ClearRoutingPins: true,
		HarnessOverride:  "codex",
		ModelOverride:    "gpt-5.4-mini",
	})
	if err != nil {
		t.Fatalf("explicit replacement route should be allowed: %v", err)
	}
	if result.Harness != "codex" || result.Model != "gpt-5.4-mini" {
		t.Fatalf("replacement route = %s/%s, want codex/gpt-5.4-mini", result.Harness, result.Model)
	}
}

func TestDispatchViaResolvedConfig_UsesProviderShimExecutableResolver(t *testing.T) {
	resetProviderShimStateForTest()
	t.Cleanup(resetProviderShimStateForTest)

	stub := &passthroughTestService{}
	SetServiceRunFactory(func(string) (agentlib.FizeauService, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetServiceRunFactory(nil) })

	fakeDDX := filepath.Join(t.TempDir(), "ddx")
	writeExecutable(t, fakeDDX, "#!/bin/sh\nexit 0\n")
	originalLookup := providerShimExecutableLookup
	providerShimExecutableLookup = func() (string, error) { return fakeDDX, nil }
	t.Cleanup(func() { providerShimExecutableLookup = originalLookup })

	initialPATH := os.Getenv("PATH")
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{Model: "haiku"})
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "agent", Model: "haiku"})

	_, err := dispatchViaResolvedConfig(context.Background(), t.TempDir(), nil, nil, rcfg, AgentRunRuntime{
		Prompt: "test",
	})
	require.NoError(t, err)
	require.True(t, stub.executeCalled, "dispatchViaResolvedConfig must still reach the service adapter")
	require.NotEqual(t, initialPATH, os.Getenv("PATH"), "dispatchViaResolvedConfig must install a provider PATH shim")
	require.Contains(t, os.Getenv("PATH"), "ddx-provider-shim-", "dispatchViaResolvedConfig must prepend the provider shim dir")
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
		Model: "claude-test-model",
		Mirror: &config.ExecutionsMirrorConfig{
			Kind:  "local",
			Path:  filepath.Join(mirrorRoot, "{attempt_id}"),
			Async: &async,
		},
	})
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "claude", Model: "claude-test-model"})

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

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
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

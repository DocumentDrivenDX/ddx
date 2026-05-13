package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
)

// TestWorkerDispatchAdapterEmptyArgsDefaultsProfile pins ddx-755f5881 AC #1:
// workerDispatchAdapter.DispatchWorker with an empty rawArgs and no
// workers.default_spec must produce a spec with Profile: "default", watch mode,
// and the default idle interval, with no route pins set. This is the contract
// that eliminates the historical
// 19-burn drain-queue failure mode where an empty input fanned out into
// per-tier ladder iteration with no upstream synthesis target.
func TestWorkerDispatchAdapterEmptyArgsDefaultsProfile(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	// Bare config — no workers block, no agent.routing block.
	cfg := "version: \"1.0\"\nlibrary:\n  path: \".ddx/plugins/ddx\"\n  repository:\n    url: \"https://example.com/lib\"\n    branch: \"main\"\n"
	if err := writeFile(filepath.Join(root, ".ddx", "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	// Stub out the bead worker so StartExecuteLoop does not require a real
	// agent service. The executor blocks until ctx is cancelled (Stop below).
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store: s,
			Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				<-ctx.Done()
				return agent.ExecuteBeadReport{BeadID: beadID, Status: agent.ExecuteBeadStatusExecutionFailed, Detail: "canceled"}, ctx.Err()
			}),
		}
	}

	adapter := &workerDispatchAdapter{manager: m}
	result, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, nil)
	if err != nil {
		t.Fatalf("dispatch with empty args: %v", err)
	}
	defer func() { _ = m.Stop(result.ID) }()

	// Read the persisted spec.json directly — it is the actual contract
	// passed into StartExecuteLoop.
	specBytes, err := os.ReadFile(filepath.Join(m.rootDir, result.ID, "spec.json"))
	if err != nil {
		t.Fatalf("read spec.json: %v", err)
	}
	var spec ExecuteLoopWorkerSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		t.Fatalf("unmarshal spec.json: %v", err)
	}
	if spec.Profile != "default" {
		t.Errorf("Profile: want %q, got %q", "default", spec.Profile)
	}
	if spec.Harness != "" {
		t.Errorf("Harness must be empty on default path, got %q", spec.Harness)
	}
	if spec.Model != "" {
		t.Errorf("Model must be empty on default path, got %q", spec.Model)
	}
	if spec.Provider != "" {
		t.Errorf("Provider must be empty on default path, got %q", spec.Provider)
	}
	if spec.Effort != "" {
		t.Errorf("Effort must be empty on default path, got %q", spec.Effort)
	}
	if spec.Mode != executeloop.ModeWatch {
		t.Errorf("Mode: want watch, got %q", spec.Mode)
	}
	if spec.IdleInterval.Duration != 30*time.Second {
		t.Errorf("IdleInterval: want 30s, got %v", spec.IdleInterval.Duration)
	}
	if spec.LabelFilter != "" {
		t.Errorf("LabelFilter must be empty on default path, got %q", spec.LabelFilter)
	}
	if spec.ReviewHarness != "" {
		t.Errorf("ReviewHarness must be empty on default path, got %q", spec.ReviewHarness)
	}
	if spec.ReviewModel != "" {
		t.Errorf("ReviewModel must be empty on default path, got %q", spec.ReviewModel)
	}
}

// TestWorkerDispatchAdapterHistoricalDrainConfigNoSynthesis pins ddx-755f5881
// AC #4: on the default dispatch path (no rawArgs, no workers.default_spec),
// the dispatched spec has only default profile/watch/idle runtime intent —
// model and harness are empty so no synthesis fan-out occurs.
func TestWorkerDispatchAdapterHistoricalDrainConfigNoSynthesis(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	if err := writeFile(filepath.Join(root, ".ddx", "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store: s,
			Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				<-ctx.Done()
				return agent.ExecuteBeadReport{BeadID: beadID, Status: agent.ExecuteBeadStatusExecutionFailed, Detail: "canceled"}, ctx.Err()
			}),
		}
	}

	adapter := &workerDispatchAdapter{manager: m}
	result, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, nil)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	defer func() { _ = m.Stop(result.ID) }()

	specBytes, err := os.ReadFile(filepath.Join(m.rootDir, result.ID, "spec.json"))
	if err != nil {
		t.Fatalf("read spec.json: %v", err)
	}
	var spec ExecuteLoopWorkerSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		t.Fatalf("unmarshal spec.json: %v", err)
	}
	if spec.Profile != "default" {
		t.Errorf("Profile: want %q, got %q", "default", spec.Profile)
	}
	if spec.Model != "" {
		t.Errorf("Model must be empty on default path, got %q", spec.Model)
	}
	if spec.Harness != "" {
		t.Errorf("Harness must be empty on default path, got %q", spec.Harness)
	}
	if spec.Mode != executeloop.ModeWatch {
		t.Errorf("Mode: want watch, got %q", spec.Mode)
	}
	if spec.IdleInterval.Duration != 30*time.Second {
		t.Errorf("IdleInterval: want 30s, got %v", spec.IdleInterval.Duration)
	}
}

func TestGraphQL_WorkerDispatch_UsesExecuteLoopSpec(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	cfg := "version: \"1.0\"\nlibrary:\n  path: \".ddx/plugins/ddx\"\n  repository:\n    url: \"https://example.com/lib\"\n    branch: \"main\"\n"
	if err := writeFile(filepath.Join(root, ".ddx", "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	installFastSuccessWorker(m)

	rawBytes, _ := json.Marshal(map[string]any{
		"harness":             "fiz",
		"model":               "qwen/qwen3.6",
		"profile":             "default",
		"provider":            "openrouter",
		"effort":              "high",
		"label_filter":        "phase:reliability",
		"mode":                "watch",
		"idle_interval":       "19s",
		"no_review":           true,
		"review_harness":      "review-harness",
		"review_model":        "review-model",
		"opaque_passthrough":  true,
		"max_cost_usd":        0.95,
		"request_timeout":     "53s",
		"rate_limit_max_wait": "92s",
		"min_power":           7,
		"max_power":           8,
		"from_rev":            "HEAD~1",
		"spec_version":        executeloop.SpecCurrentVersion,
		"future_graphql_key":  "ignored",
	})
	raw := string(rawBytes)

	adapter := &workerDispatchAdapter{manager: m}
	result, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, &raw)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	t.Cleanup(func() {
		_ = m.Stop(result.ID)
		_ = waitForWorkerExit(t, m, result.ID, 5*time.Second)
	})

	specBytes, err := os.ReadFile(filepath.Join(m.rootDir, result.ID, "spec.json"))
	if err != nil {
		t.Fatalf("read spec.json: %v", err)
	}
	var spec ExecuteLoopWorkerSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		t.Fatalf("unmarshal spec.json: %v", err)
	}
	if spec.ProjectRoot != root {
		t.Errorf("ProjectRoot: want %q, got %q", root, spec.ProjectRoot)
	}
	if spec.Harness != "fiz" {
		t.Errorf("Harness: got %q", spec.Harness)
	}
	if spec.Model != "qwen/qwen3.6" {
		t.Errorf("Model: got %q", spec.Model)
	}
	if spec.Profile != "default" {
		t.Errorf("Profile: got %q", spec.Profile)
	}
	if spec.Provider != "openrouter" {
		t.Errorf("Provider: got %q", spec.Provider)
	}
	if spec.Effort != "high" {
		t.Errorf("Effort: got %q", spec.Effort)
	}
	if spec.LabelFilter != "phase:reliability" {
		t.Errorf("LabelFilter: got %q", spec.LabelFilter)
	}
	if spec.Mode != executeloop.ModeWatch {
		t.Errorf("Mode: got %q", spec.Mode)
	}
	if spec.IdleInterval.Duration != 19*time.Second {
		t.Errorf("IdleInterval: got %v", spec.IdleInterval.Duration)
	}
	if !spec.NoReview {
		t.Errorf("NoReview: got false")
	}
	if spec.ReviewHarness != "review-harness" {
		t.Errorf("ReviewHarness: got %q", spec.ReviewHarness)
	}
	if spec.ReviewModel != "review-model" {
		t.Errorf("ReviewModel: got %q", spec.ReviewModel)
	}
	if !spec.OpaquePassthrough {
		t.Errorf("OpaquePassthrough: got false")
	}
	if spec.MaxCostUSD != 0.95 {
		t.Errorf("MaxCostUSD: got %.2f", spec.MaxCostUSD)
	}
	if spec.RequestTimeout.Duration != 53*time.Second {
		t.Errorf("RequestTimeout: got %v", spec.RequestTimeout.Duration)
	}
	if spec.RateLimitMaxWait.Duration != 92*time.Second {
		t.Errorf("RateLimitMaxWait: got %v", spec.RateLimitMaxWait.Duration)
	}
	if spec.MinPower != 7 {
		t.Errorf("MinPower: got %d", spec.MinPower)
	}
	if spec.MaxPower != 8 {
		t.Errorf("MaxPower: got %d", spec.MaxPower)
	}
	if spec.FromRev != "HEAD~1" {
		t.Errorf("FromRev: got %q", spec.FromRev)
	}
	if spec.SpecVersion != executeloop.SpecCurrentVersion {
		t.Errorf("SpecVersion: got %d", spec.SpecVersion)
	}
}

func TestGraphQL_WorkerDispatch_DefaultSpec(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"it\"\nworkers:\n  default_spec:\n    profile: cheap\n    effort: low\n"
	if err := os.WriteFile(filepath.Join(root, ".ddx", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	installFastSuccessWorker(m)

	raw := `{"mode":"once","opaque_passthrough":true}`
	adapter := &workerDispatchAdapter{manager: m}
	result, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, &raw)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	_ = waitForWorkerExit(t, m, result.ID, 5*time.Second)

	specBytes, err := os.ReadFile(filepath.Join(m.rootDir, result.ID, "spec.json"))
	if err != nil {
		t.Fatalf("read spec.json: %v", err)
	}
	var spec ExecuteLoopWorkerSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		t.Fatalf("unmarshal spec.json: %v", err)
	}
	if spec.Profile != "cheap" {
		t.Errorf("Profile: want cheap, got %q", spec.Profile)
	}
	if spec.Effort != "low" {
		t.Errorf("Effort: want low, got %q", spec.Effort)
	}
	if spec.Mode != executeloop.ModeOnce {
		t.Errorf("Mode: want once, got %q", spec.Mode)
	}
}

func TestGraphQL_WorkerDispatch_RejectsLegacyPollInterval(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	raw := `{"poll_interval":"30s"}`
	adapter := &workerDispatchAdapter{manager: m}
	_, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, &raw)
	if err == nil {
		t.Fatal("expected legacy poll_interval error, got nil")
	}
	if !strings.Contains(err.Error(), "poll_interval is not supported") {
		t.Fatalf("error should reject poll_interval, got: %v", err)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

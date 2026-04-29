package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// TestWorkerDispatchAdapterEmptyArgsDefaultsProfile pins ddx-755f5881 AC #1:
// workerDispatchAdapter.DispatchWorker with an empty rawArgs and no
// workers.default_spec must produce a spec with Profile: "default" and no
// other knobs set. This is the contract that eliminates the historical
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
	// passed into StartExecuteLoop. WorkerRecord drops a few fields
	// (MinTier/MaxTier) so it cannot fully verify "no other fields".
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
	if spec.ModelRef != "" {
		t.Errorf("ModelRef must be empty on default path, got %q", spec.ModelRef)
	}
	if spec.Effort != "" {
		t.Errorf("Effort must be empty on default path, got %q", spec.Effort)
	}
	if spec.MinTier != "" {
		t.Errorf("MinTier must be empty on default path, got %q", spec.MinTier)
	}
	if spec.MaxTier != "" {
		t.Errorf("MaxTier must be empty on default path, got %q", spec.MaxTier)
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
// AC #4: the same historical drain-queue config (with agent.routing
// profile_ladders + model_overrides) that previously caused the 19-burn
// failure mode must no longer drive DDx-side synthesis on the default
// dispatch path. The deprecated routing fields are NOT consulted; the
// dispatched spec is just {Profile: "default"}, so a downstream worker
// either succeeds or returns a single typed error — it can no longer fan
// out into per-tier iteration with mismatched harness+model pairs.
func TestWorkerDispatchAdapterHistoricalDrainConfigNoSynthesis(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	// Mirror the shape of the user's historical config: routing block with
	// profile_ladders and model_overrides naming a model that previously
	// triggered the gemini+minimax misroute fan-out.
	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  routing:
    profile_ladders:
      default: [cheap, standard, smart]
    model_overrides:
      cheap: minimax/minimax-m2.7
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

	agent.ResetRoutingCallCounters()
	adapter := &workerDispatchAdapter{manager: m}
	result, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, nil)
	if err != nil {
		t.Fatalf("dispatch with historical config: %v", err)
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
		t.Errorf("Model must be empty — historical model_overrides must NOT drive synthesis on default path, got %q", spec.Model)
	}
	if spec.Harness != "" {
		t.Errorf("Harness must be empty on default path, got %q", spec.Harness)
	}
	// The dispatch step itself must not consult the ladder helpers.
	if got := agent.ResolveProfileLadderCallCount(); got != 0 {
		t.Errorf("ResolveProfileLadder must not be called on default dispatch path, got %d", got)
	}
	if got := agent.ResolveTierModelRefCallCount(); got != 0 {
		t.Errorf("ResolveTierModelRef must not be called on default dispatch path, got %d", got)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

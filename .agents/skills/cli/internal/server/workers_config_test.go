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
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// writeFakeWorkerRecord dumps a status.json into the manager's workers
// directory so List() picks it up as a running drain worker — used to
// exercise the workers.max_count cap without spawning a real goroutine.
func writeFakeWorkerRecord(t *testing.T, m *WorkerManager, rec WorkerRecord) {
	t.Helper()
	dir := filepath.Join(m.rootDir, rec.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestWorkerDispatchAdapterEnforcesMaxCount covers the workers.max_count
// safety rail (ddx-b6cf025c). When max_count is set and the count of
// running execute-loop workers is already at the cap, the adapter must
// refuse with a clear error rather than silently starting a new worker.
func TestWorkerDispatchAdapterEnforcesMaxCount(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"it\"\nworkers:\n  max_count: 1\n"
	if err := os.WriteFile(filepath.Join(root, ".ddx", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	writeFakeWorkerRecord(t, m, WorkerRecord{
		ID:          "worker-pre-existing",
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC(),
	})

	adapter := &workerDispatchAdapter{manager: m}
	_, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, nil)
	if err == nil {
		t.Fatal("expected error when max_count reached, got nil")
	}
	if !strings.Contains(err.Error(), "max_count") {
		t.Fatalf("error should mention max_count, got: %v", err)
	}
}

// TestWorkerDispatchAdapterMaxCountAllowsWhenUnderLimit verifies the cap
// is inclusive (>=) not strict (>): dispatching with cap=2 and 1 running
// worker succeeds.
func TestWorkerDispatchAdapterMaxCountAllowsWhenUnderLimit(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"it\"\nworkers:\n  max_count: 2\n  default_spec:\n    profile: cheap\n    effort: low\n"
	if err := os.WriteFile(filepath.Join(root, ".ddx", "config.yaml"), []byte(cfg), 0o644); err != nil {
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

	writeFakeWorkerRecord(t, m, WorkerRecord{
		ID:          "worker-pre-existing",
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC(),
	})

	adapter := &workerDispatchAdapter{manager: m}
	result, err := adapter.DispatchWorker(context.Background(), "execute-loop", root, nil)
	if err != nil {
		t.Fatalf("dispatch under cap: %v", err)
	}
	defer func() { _ = m.Stop(result.ID) }()

	// Also verify default_spec propagated: profile=cheap, effort=low.
	rec, err := m.Show(result.ID)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if rec.Profile != "cheap" {
		t.Errorf("record.Profile: want cheap, got %q", rec.Profile)
	}
	if rec.Effort != "low" {
		t.Errorf("record.Effort: want low, got %q", rec.Effort)
	}
}

// TestCountRunningDrainWorkersFiltersByProjectAndKind verifies the helper
// only counts execute-loop workers in state=running for the target
// projectRoot — not other kinds, other projects, or stopped workers.
func TestCountRunningDrainWorkersFiltersByProjectAndKind(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	adapter := &workerDispatchAdapter{manager: m}

	writeFakeWorkerRecord(t, m, WorkerRecord{
		ID: "w-drain-running", Kind: "execute-loop", State: "running",
		ProjectRoot: root, StartedAt: time.Now().UTC(),
	})
	writeFakeWorkerRecord(t, m, WorkerRecord{
		ID: "w-drain-stopped", Kind: "execute-loop", State: "stopped",
		ProjectRoot: root, StartedAt: time.Now().UTC(),
	})
	writeFakeWorkerRecord(t, m, WorkerRecord{
		ID: "w-plugin", Kind: "plugin-action", State: "running",
		ProjectRoot: root, StartedAt: time.Now().UTC(),
	})
	writeFakeWorkerRecord(t, m, WorkerRecord{
		ID: "w-other-project", Kind: "execute-loop", State: "running",
		ProjectRoot: "/different/project", StartedAt: time.Now().UTC(),
	})

	got := adapter.countRunningDrainWorkers(root)
	if got != 1 {
		t.Fatalf("countRunningDrainWorkers: want 1, got %d", got)
	}
}

// Ensure we import bead so setupBeadStore compiles in this package.
var _ = bead.StatusOpen

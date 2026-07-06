package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/spf13/cobra"
)

func newServerManagedCmd(t *testing.T, workerID string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "work"}
	cmd.Flags().String("server-managed", "", "")
	if workerID != "" {
		if err := cmd.Flags().Set("server-managed", workerID); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}
	return cmd
}

func projectWithWorkerDir(t *testing.T, workerID string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(ddxroot.JoinProject(root, "workers"), workerID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir worker dir: %v", err)
	}
	return root
}

// TestServerManagedWorker_WritesOperatorAttentionResult verifies that a
// server-managed worker persists a structured result.json carrying the
// operator-attention outcome, so the supervising server can park it instead of
// respawning it. See ddx-3d57bc30.
func TestServerManagedWorker_WritesOperatorAttentionResult(t *testing.T) {
	const workerID = "worker-oa"
	root := projectWithWorkerDir(t, workerID)
	cmd := newServerManagedCmd(t, workerID)

	result := &agent.ExecuteBeadLoopResult{
		StopCondition:     "operator_attention",
		OperatorAttention: &agent.OperatorAttentionStop{Message: "dirty root"},
	}
	writeServerManagedResult(cmd, root, result)

	path := filepath.Join(ddxroot.JoinProject(root, "workers"), workerID, "result.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected result.json to be written: %v", err)
	}
	var got serverpkg.ManagedWorkerResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal result.json: %v", err)
	}
	if got.StopCondition != "operator_attention" || !got.OperatorAttention {
		t.Fatalf("unexpected result.json: %+v", got)
	}
	if !got.IsRestartBlocking() {
		t.Fatal("written result must classify as restart-blocking")
	}
}

func TestServerManagedWorker_NoResultWhenFlagAbsent(t *testing.T) {
	const workerID = "worker-none"
	root := projectWithWorkerDir(t, workerID)

	// A command without the server-managed flag (e.g. a direct `ddx try`) must
	// not write a result file.
	cmd := &cobra.Command{Use: "try"}
	writeServerManagedResult(cmd, root, &agent.ExecuteBeadLoopResult{StopCondition: "drained"})

	// Also: flag present but empty (not server-managed) is a no-op.
	cmd2 := newServerManagedCmd(t, "")
	writeServerManagedResult(cmd2, root, &agent.ExecuteBeadLoopResult{StopCondition: "drained"})

	path := filepath.Join(ddxroot.JoinProject(root, "workers"), workerID, "result.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no result.json without server-managed id, stat err=%v", err)
	}
}

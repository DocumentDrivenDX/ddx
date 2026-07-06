package server

import (
	"os"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// initInTreeProject creates a <root>/.ddx dir so ddxroot.JoinProject resolves
// in-tree and deterministically for the test.
func initInTreeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(ddxroot.InTree(root), 0o755); err != nil {
		t.Fatalf("mkdir .ddx: %v", err)
	}
	return root
}

func TestManagedWorkerResult_RoundTripAndClassification(t *testing.T) {
	root := initInTreeProject(t)
	const workerID = "worker-oa"
	dir := managedWorkerResultDir(root, workerID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir worker dir: %v", err)
	}

	// Absent result → not readable, caller falls back to exit-code path.
	if _, ok := readManagedWorkerResult(dir); ok {
		t.Fatal("expected no result before write")
	}

	if err := WriteManagedWorkerResult(root, workerID, ManagedWorkerResult{
		StopCondition:     "operator_attention",
		OperatorAttention: true,
	}); err != nil {
		t.Fatalf("WriteManagedWorkerResult: %v", err)
	}

	res, ok := readManagedWorkerResult(dir)
	if !ok {
		t.Fatal("expected result after write")
	}
	if res.StopCondition != "operator_attention" || !res.OperatorAttention {
		t.Fatalf("round-trip mismatch: %+v", res)
	}
	if !res.IsRestartBlocking() {
		t.Fatal("operator-attention result must be restart-blocking")
	}

	// A normal drain must not be restart-blocking.
	drained := ManagedWorkerResult{StopCondition: "drained"}
	if drained.IsRestartBlocking() {
		t.Fatal("a drained result must not be restart-blocking")
	}
}

// TestSupervisor_OperatorAttentionTerminalDoesNotRespawn is the regression
// guard for ddx-3d57bc30: an operator-attention terminal must suppress the
// immediate relaunch that previously produced a ~10s respawn thrash, while a
// successful drain stays restartable.
func TestSupervisor_OperatorAttentionTerminalDoesNotRespawn(t *testing.T) {
	root := initInTreeProject(t)
	sup := NewWorkerSupervisor(NewWorkerManager(root))
	state := DefaultWorkerDesiredState(root)
	now := time.Now().UTC()

	if !sup.canStartMore(state, now) {
		t.Fatal("expected canStartMore=true with no terminal history")
	}

	sup.recordTerminalHistory([]WorkerRecord{{
		ID:         "worker-oa",
		Kind:       "work",
		State:      "exited",
		Status:     "operator_attention",
		ReapReason: "operator_attention",
	}}, now)

	if sup.canStartMore(state, now) {
		t.Fatal("operator-attention terminal must suppress relaunch (respawn thrash)")
	}
}

func TestSupervisor_SuccessfulDrainRemainsRestartable(t *testing.T) {
	root := initInTreeProject(t)
	sup := NewWorkerSupervisor(NewWorkerManager(root))
	state := DefaultWorkerDesiredState(root)
	now := time.Now().UTC()

	sup.recordTerminalHistory([]WorkerRecord{{
		ID:     "worker-ok",
		Kind:   "work",
		State:  "exited",
		Status: "success",
	}}, now)

	if !sup.canStartMore(state, now) {
		t.Fatal("a successful drain must remain restartable")
	}
}

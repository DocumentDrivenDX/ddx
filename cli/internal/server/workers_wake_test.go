package server

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// AC: WakeProject signals every running execute-loop worker bound to a
// project. The send must be non-blocking — even when the worker has not
// yet received from its wake channel — and must coalesce: a second wake
// while one is still pending is dropped (cap-1 buffer).
func TestWorkerManager_WakeProject_BroadcastsAndCoalesces(t *testing.T) {
	dir := t.TempDir()
	m := NewWorkerManager(dir)

	wakeCh := make(chan struct{}, 1)
	m.workers["worker-test"] = &workerHandle{
		record: WorkerRecord{ProjectRoot: dir},
		wakeCh: wakeCh,
	}

	if got := m.WakeProject(dir); got != 1 {
		t.Errorf("first wake: want 1 worker signalled, got %d", got)
	}
	// Wake again before the channel is drained — must coalesce, not block.
	if got := m.WakeProject(dir); got != 1 {
		t.Errorf("coalesced wake: want 1, got %d", got)
	}
	// Drain — exactly one wake should be present.
	select {
	case <-wakeCh:
	default:
		t.Fatal("wake channel must have one pending signal")
	}
	select {
	case <-wakeCh:
		t.Fatal("wake must coalesce; second receive should not have a pending signal")
	default:
	}
}

// AC: WakeProject ignores workers bound to other projects.
func TestWorkerManager_WakeProject_IgnoresOtherProjects(t *testing.T) {
	m := NewWorkerManager(t.TempDir())
	otherWake := make(chan struct{}, 1)
	m.workers["other-worker"] = &workerHandle{
		record: WorkerRecord{ProjectRoot: "/some/other/project"},
		wakeCh: otherWake,
	}
	if got := m.WakeProject("/different/project"); got != 0 {
		t.Errorf("want 0 wakes for unrelated project, got %d", got)
	}
	select {
	case <-otherWake:
		t.Error("other-project worker must not be signalled")
	default:
	}
}

// AC: WakeProject is a no-op for plugin-action workers (wakeCh nil) and
// does not panic.
func TestWorkerManager_WakeProject_NilWakeChSafe(t *testing.T) {
	dir := t.TempDir()
	m := NewWorkerManager(dir)
	m.workers["plugin-worker"] = &workerHandle{
		record: WorkerRecord{ProjectRoot: dir},
		wakeCh: nil,
	}
	if got := m.WakeProject(dir); got != 0 {
		t.Errorf("plugin-action worker (nil wakeCh) must not be counted; got %d", got)
	}
}

// Compile-time guard: WorkerManager satisfies the graphql.ExecuteLoopWaker
// shape used by the resolver. We do not import the graphql package here to
// avoid a cycle, but we duplicate the signature in a local interface.
var _ interface {
	WakeProject(string) int
} = (*WorkerManager)(nil)

var _ = agent.ExecuteBeadLoopRuntime{} // ensure agent import is referenced

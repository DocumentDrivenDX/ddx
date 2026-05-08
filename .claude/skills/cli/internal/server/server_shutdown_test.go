package server

// TC-SERVER-SHUTDOWN-001: Server.Shutdown invokes beadHub.Close and coordinatorRegistry.StopAll.

import (
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// spyBeadHub wraps a real WatcherHub and records whether Close was called.
type spyBeadHub struct {
	*bead.WatcherHub
	closeCalled bool
}

func (s *spyBeadHub) Close() {
	s.closeCalled = true
	s.WatcherHub.Close()
}

// TC-SERVER-SHUTDOWN-001: Shutdown calls beadHub.Close and StopAll in sequence.
func TestServerShutdown(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "shutdown-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	// Inject a spy so we can observe the Close() call.
	spy := &spyBeadHub{WatcherHub: bead.NewWatcherHub(250 * time.Millisecond)}
	srv.beadHub = spy

	// Prime one coordinator so StopAll has an entry to clear.
	_ = srv.workers.LandCoordinators.Get(workDir)

	if err := srv.Shutdown(); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	// Verify beadHub.Close() was invoked.
	if !spy.closeCalled {
		t.Error("Shutdown did not call beadHub.Close()")
	}

	// Verify StopAll was invoked: the registry must be empty after Shutdown.
	all := srv.workers.LandCoordinators.AllMetrics()
	if len(all) != 0 {
		t.Errorf("Shutdown did not call StopAll: coordinator registry has %d entries", len(all))
	}
}

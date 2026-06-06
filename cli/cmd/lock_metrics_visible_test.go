package cmd

import (
	"os"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLockMetrics_VisibleInDdxLogs asserts that the structured lock events
// emitted around an execute-bead run's tracker-lock hold surface via the
// lock-events accessor (lockmetrics.Load reads .ddx/metrics/locks.jsonl, the
// stream `ddx work`/`ddx run` enable). The sample run is the durable-audit
// finalize step the execute-bead loop performs, which acquires and releases
// the tracker lock through the instrumented wrapper.
func TestLockMetrics_VisibleInDdxLogs(t *testing.T) {
	root := t.TempDir()

	// Enable the same file sink `ddx work`/`ddx run` install at startup.
	lockmetrics.SetSink(lockmetrics.FileSink(root))
	t.Cleanup(func() { lockmetrics.SetSink(nil) })

	// Sample execute-bead finalize step: acquires and releases the tracker
	// lock around the durable-audit commit attempt.
	require.NoError(t, agent.CommitDurableAuditOutputs(root, "20260527T-sample"))

	events, err := lockmetrics.Load(root)
	require.NoError(t, err)
	require.NotEmpty(t, events, "expected lock events visible via the accessor")

	var sawAcquire, sawRelease bool
	for _, ev := range events {
		if ev.LockName != "tracker.lock" {
			continue
		}
		switch ev.Event {
		case "acquire":
			sawAcquire = true
		case "release":
			sawRelease = true
			assert.NotEmpty(t, ev.ReleasedAt, "release event must carry released_at")
			assert.Equal(t, os.Getpid(), ev.HolderPID)
			assert.Equal(t, "durable_audit", ev.Operation)
		}
	}
	assert.True(t, sawAcquire, "tracker.lock acquire event must be visible via the accessor")
	assert.True(t, sawRelease, "tracker.lock release event must be visible via the accessor")
}

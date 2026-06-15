//go:build !windows

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequestTimeoutReapsProviderAndAttemptChildren proves that when the
// absolute request-timeout cap fires, the attempt-scoped provider process tree
// is reaped and process-cleanup evidence is written (ddx-9febbad2).
func TestRequestTimeoutReapsProviderAndAttemptChildren(t *testing.T) {
	dir := t.TempDir()
	claudePID := startFakeProviderChild(t, dir, "claude")
	codexPID := startFakeProviderChild(t, dir, "codex")
	waitForProviderChildren(t, os.Getpid(), claudePID, codexPID)

	projectRoot := t.TempDir()
	const attemptID = "attempt-rt-reap"

	ev := reapRequestTimeoutAttempt(projectRoot, attemptID, "ddx-bead", "do", "", os.Getpid(), 2*time.Second, 5*time.Second, time.Now().UTC())

	assertProcessGone(t, claudePID)
	assertProcessGone(t, codexPID)

	require.GreaterOrEqual(t, len(ev.Reaped), 2, "both attempt-scoped provider children must be reaped")
	assert.Equal(t, requestTimeoutCleanupReaped, ev.CleanupResult)
	assert.NotZero(t, ev.ProviderPID, "evidence must name the reaped provider PID")

	path := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, requestTimeoutArtifact)
	require.FileExists(t, path, "process-cleanup evidence must be written after request-timeout reap")
}

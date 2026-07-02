//go:build !linux

package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionCleanupManager_UnsupportedPlatformReportsProcessCleanupUnavailable(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-unavailable",
		AttemptID:    "20260608T140000-ffffffff",
		StartedAt:    time.Now().UTC(),
		WorktreePath: filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-unavailable-20260608T140000-ffffffff"),
	}))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	require.Len(t, summary.ProcessFindings, 1)
	finding := summary.ProcessFindings[0]
	assert.False(t, finding.WouldKill)
	assert.False(t, finding.Terminated)
	assert.Contains(t, finding.StaleReason, "unavailable")
	require.NotEmpty(t, summary.Warnings)
	assert.Equal(t, "attempt_process_cleanup_unavailable", summary.Warnings[0].Class)
}

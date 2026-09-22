//go:build linux

package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBeadWorkerNoChangesVerificationTimeoutKeepsOpenAndReaps(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-timeoutverify", Title: "Verification tree must be reaped"}
	require.NoError(t, store.Create(context.Background(), b))

	projectRoot := t.TempDir()
	shellPIDFile := filepath.Join(projectRoot, "inner-shell.pid")
	childPIDFile := filepath.Join(projectRoot, "sleep.pid")
	command := nestedPIDCaptureCommand(shellPIDFile, childPIDFile, "sleep 30")

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "verification_command: " + command,
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                     "worker",
		NoChangesVerificationTimeout: time.Second,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: projectRoot,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Contains(t, got.Labels, NoChangesLabelUnverified)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawUnverified bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventUnverified {
			sawUnverified = true
			assert.Contains(t, ev.Body, "exit_code=-1")
			assert.Contains(t, ev.Body, "verification_command timed out after 1s")
		}
	}
	assert.True(t, sawUnverified, "timeout must be recorded as no_changes_unverified")

	shellPID := readPIDFile(t, shellPIDFile)
	childPID := readPIDFile(t, childPIDFile)
	var shellState, childState string
	require.Eventually(t, func() bool {
		shellState = processDeadOrZombieStatus(shellPID)
		childState = processDeadOrZombieStatus(childPID)
		return processDeadOrZombie(shellPID) && processDeadOrZombie(childPID)
	}, time.Second, 20*time.Millisecond, "shell proc state=%s child proc state=%s", procStateSnapshot{&shellState}, procStateSnapshot{&childState})
}

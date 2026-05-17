package agent

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBeadWorkerNoChangesVerifiedLongCommandCloses(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-longverify", Title: "Verification gate needs time"}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "verification_command: sh -lc 'sleep 0.05'",
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
		ProjectRoot: t.TempDir(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawVerified, sawTerminal bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventVerified {
			sawVerified = true
			assert.Contains(t, ev.Body, "exit_code=0")
			assert.Contains(t, ev.Body, "verification_command=sh -lc 'sleep 0.05'")
		}
		if ev.Summary == ExecuteBeadStatusAlreadySatisfied {
			sawTerminal = true
		}
	}
	assert.True(t, sawVerified, "no_changes_verified event must be emitted")
	assert.True(t, sawTerminal, "already_satisfied close must be recorded")
}

func TestExecuteBeadWorkerNoChangesVerificationTimeoutKeepsOpenAndReaps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group assertions are unix-specific")
	}

	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-timeoutverify", Title: "Verification tree must be reaped"}
	require.NoError(t, store.Create(b))

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

	got, err := store.Get(b.ID)
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
	require.Eventually(t, func() bool {
		return !processExists(shellPID) && !processExists(childPID)
	}, time.Second, 20*time.Millisecond)
}

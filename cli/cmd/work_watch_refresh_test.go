package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSequencedDDXBinary(t *testing.T, dir string, commits ...string) string {
	t.Helper()
	require.NotEmpty(t, commits)

	path := filepath.Join(dir, "ddx")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
counter_file="${DDX_REFRESH_COUNTER_FILE:?missing DDX_REFRESH_COUNTER_FILE}"
count=0
if [ -f "$counter_file" ]; then
  count=$(cat "$counter_file")
fi
count=$((count + 1))
printf '%%s' "$count" > "$counter_file"
commit=%q
if [ "$count" -gt 1 ]; then
  commit=%q
fi
if [ "${1:-}" = "version" ]; then
  echo "DDx v9.9.9"
  echo "Commit: $commit"
  echo "Built: 2026-06-07T00:00:00Z"
fi
`, commits[0], commits[len(commits)-1])
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func testWorkWatchCommand(t *testing.T, factory *CommandFactory, projectRoot string) *cobra.Command {
	t.Helper()

	root := factory.NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)
	require.NoError(t, workCmd.Flags().Set("project", projectRoot))
	require.NoError(t, workCmd.Flags().Set("watch", "true"))
	require.NoError(t, workCmd.Flags().Set("no-review", "true"))
	require.NoError(t, workCmd.Flags().Set("no-review-i-know-what-im-doing", "true"))
	workCmd.SetErr(io.Discard)
	return workCmd
}

func testLoopConfig() config.ResolvedConfig {
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	return config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
}

func createWatchRefreshStore(t *testing.T, projectRoot string, count int) *bead.Store {
	t.Helper()

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init(context.Background()))
	for i := 0; i < count; i++ {
		require.NoError(t, store.Create(context.Background(), &bead.Bead{
			ID:        fmt.Sprintf("ddx-refresh-%d", i+1),
			Title:     fmt.Sprintf("refresh bead %d", i+1),
			Status:    bead.StatusOpen,
			Priority:  i,
			IssueType: bead.DefaultType,
		}))
	}
	return store
}

func setWatchRefreshArgs(t *testing.T, args []string) {
	t.Helper()

	prev := append([]string(nil), os.Args...)
	os.Args = append([]string(nil), args...)
	t.Cleanup(func() { os.Args = prev })
}

func TestWorkWatch_ReexecsOnNewerInstalledBinaryBetweenBeads(t *testing.T) {
	projectRoot := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	store := createWatchRefreshStore(t, projectRoot, 2)
	binaryDir := t.TempDir()
	counterFile := filepath.Join(t.TempDir(), "refresh-counter")
	t.Setenv("DDX_REFRESH_COUNTER_FILE", counterFile)
	binaryPath := writeSequencedDDXBinary(t, binaryDir, "old-commit", "new-commit")

	var reexecCount int32
	var seenArgs []string
	var seenDir string
	factory := NewCommandFactory(projectRoot)
	factory.Commit = "old-commit"
	factory.workBinaryPathOverride = func() string { return binaryPath }
	factory.workBinaryReexecOverride = func(exe string, argv []string, env []string, dir string) error {
		_ = exe
		_ = env
		atomic.AddInt32(&reexecCount, 1)
		seenArgs = append([]string(nil), argv...)
		seenDir = dir
		return nil
	}
	workCmd := testWorkWatchCommand(t, factory, projectRoot)
	setWatchRefreshArgs(t, []string{"ddx", "work", "--watch", "--min-power", "40", "--max-power", "90", "--project", projectRoot})

	refreshCheck := factory.buildWorkBinaryRefreshCheck(workCmd, projectRoot, "", workSelfRefreshEnabled(workCmd))
	require.NotNil(t, refreshCheck)

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	execCalls := atomic.Int32{}
	worker := &agent.ExecuteBeadWorker{
		Store: store,
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			call := execCalls.Add(1)
			if call == 1 {
				close(firstStarted)
				<-releaseFirst
				return agent.ExecuteBeadReport{
					BeadID:    beadID,
					Status:    agent.ExecuteBeadStatusSuccess,
					ResultRev: "rev-" + beadID,
				}, nil
			}
			t.Fatalf("unexpected execution of %s after self-refresh should have stopped the loop", beadID)
			return agent.ExecuteBeadReport{}, nil
		}),
	}

	ctx := context.Background()
	runtime := agent.ExecuteBeadLoopRuntime{
		Mode:               executeloop.ModeWatch,
		IdleInterval:       5 * time.Millisecond,
		NoReview:           true,
		WorkerID:           "worker",
		ProjectRoot:        projectRoot,
		BinaryRefreshCheck: refreshCheck,
		SessionID:          "sess-watch-refresh",
	}

	resultCh := make(chan struct {
		result *agent.ExecuteBeadLoopResult
		err    error
	}, 1)
	go func() {
		result, err := worker.Run(ctx, testLoopConfig(), runtime)
		resultCh <- struct {
			result *agent.ExecuteBeadLoopResult
			err    error
		}{result: result, err: err}
	}()

	<-firstStarted
	close(releaseFirst)

	var result *agent.ExecuteBeadLoopResult
	var runErr error
	select {
	case outcome := <-resultCh:
		result, runErr = outcome.result, outcome.err
	case <-time.After(2 * time.Second):
		t.Fatal("watch worker did not stop after self-refresh")
	}

	require.NoError(t, runErr)
	require.NotNil(t, result)
	assert.Equal(t, "binary_refresh", result.ExitReason)
	assert.Equal(t, 1, result.Attempts)
	assert.EqualValues(t, 1, atomic.LoadInt32(&reexecCount))
	assert.Equal(t, []string{"ddx", "work", "--watch", "--min-power", "40", "--max-power", "90", "--project", projectRoot}, seenArgs)
	assert.Equal(t, cwd, seenDir)

	second, err := store.Get(context.Background(), "ddx-refresh-2")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, second.Status, "the second bead must remain untouched until the fresh process starts")
}

func TestWorkWatch_DoesNotReexecMidBead(t *testing.T) {
	projectRoot := t.TempDir()
	store := createWatchRefreshStore(t, projectRoot, 1)
	binaryDir := t.TempDir()
	counterFile := filepath.Join(t.TempDir(), "refresh-counter")
	t.Setenv("DDX_REFRESH_COUNTER_FILE", counterFile)
	binaryPath := writeSequencedDDXBinary(t, binaryDir, "old-commit", "new-commit")

	var reexecCount int32
	factory := NewCommandFactory(projectRoot)
	factory.Commit = "old-commit"
	factory.workBinaryPathOverride = func() string { return binaryPath }
	factory.workBinaryReexecOverride = func(exe string, argv []string, env []string, dir string) error {
		_ = exe
		_ = argv
		_ = env
		_ = dir
		atomic.AddInt32(&reexecCount, 1)
		return nil
	}
	workCmd := testWorkWatchCommand(t, factory, projectRoot)
	setWatchRefreshArgs(t, []string{"ddx", "work", "--watch", "--project", projectRoot})

	refreshCheck := factory.buildWorkBinaryRefreshCheck(workCmd, projectRoot, "", workSelfRefreshEnabled(workCmd))
	require.NotNil(t, refreshCheck)

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	worker := &agent.ExecuteBeadWorker{
		Store: store,
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			close(firstStarted)
			<-releaseFirst
			return agent.ExecuteBeadReport{
				BeadID:    beadID,
				Status:    agent.ExecuteBeadStatusSuccess,
				ResultRev: "rev-" + beadID,
			}, nil
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan struct {
		result *agent.ExecuteBeadLoopResult
		err    error
	}, 1)
	go func() {
		result, err := worker.Run(ctx, testLoopConfig(), agent.ExecuteBeadLoopRuntime{
			Mode:               executeloop.ModeWatch,
			IdleInterval:       5 * time.Millisecond,
			NoReview:           true,
			WorkerID:           "worker",
			ProjectRoot:        projectRoot,
			BinaryRefreshCheck: refreshCheck,
			SessionID:          "sess-watch-mid-bead",
		})
		resultCh <- struct {
			result *agent.ExecuteBeadLoopResult
			err    error
		}{result: result, err: err}
	}()

	<-firstStarted
	time.Sleep(30 * time.Millisecond)
	assert.EqualValues(t, 0, atomic.LoadInt32(&reexecCount), "refresh must not fire while the bead is still executing")
	close(releaseFirst)

	select {
	case outcome := <-resultCh:
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.result)
		assert.Equal(t, "binary_refresh", outcome.result.ExitReason)
		assert.EqualValues(t, 1, atomic.LoadInt32(&reexecCount))
	case <-time.After(2 * time.Second):
		t.Fatal("watch worker did not stop after the bead finished")
	}
}

func TestWorkWatch_NoReexecWhenBinaryUnchanged(t *testing.T) {
	projectRoot := t.TempDir()
	store := createWatchRefreshStore(t, projectRoot, 1)
	binaryDir := t.TempDir()
	counterFile := filepath.Join(t.TempDir(), "refresh-counter")
	t.Setenv("DDX_REFRESH_COUNTER_FILE", counterFile)
	binaryPath := writeSequencedDDXBinary(t, binaryDir, "stable-commit", "stable-commit")

	var reexecCount int32
	factory := NewCommandFactory(projectRoot)
	factory.Commit = "stable-commit"
	factory.workBinaryPathOverride = func() string { return binaryPath }
	factory.workBinaryReexecOverride = func(exe string, argv []string, env []string, dir string) error {
		_ = exe
		_ = argv
		_ = env
		_ = dir
		atomic.AddInt32(&reexecCount, 1)
		return nil
	}
	workCmd := testWorkWatchCommand(t, factory, projectRoot)
	setWatchRefreshArgs(t, []string{"ddx", "work", "--watch", "--project", projectRoot, "--self-refresh"})

	refreshCheck := factory.buildWorkBinaryRefreshCheck(workCmd, projectRoot, "", workSelfRefreshEnabled(workCmd))
	require.NotNil(t, refreshCheck)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := &agent.ExecuteBeadWorker{
		Store: store,
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			return agent.ExecuteBeadReport{
				BeadID:    beadID,
				Status:    agent.ExecuteBeadStatusSuccess,
				ResultRev: "rev-" + beadID,
			}, nil
		}),
	}

	resultCh := make(chan struct {
		result *agent.ExecuteBeadLoopResult
		err    error
	}, 1)
	go func() {
		result, err := worker.Run(ctx, testLoopConfig(), agent.ExecuteBeadLoopRuntime{
			Mode:               executeloop.ModeWatch,
			IdleInterval:       5 * time.Millisecond,
			NoReview:           true,
			WorkerID:           "worker",
			ProjectRoot:        projectRoot,
			BinaryRefreshCheck: refreshCheck,
			SessionID:          "sess-watch-unchanged",
		})
		resultCh <- struct {
			result *agent.ExecuteBeadLoopResult
			err    error
		}{result: result, err: err}
	}()

	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case outcome := <-resultCh:
		assert.ErrorIs(t, outcome.err, context.Canceled)
		require.NotNil(t, outcome.result)
		assert.EqualValues(t, 0, atomic.LoadInt32(&reexecCount))
	case <-time.After(2 * time.Second):
		t.Fatal("watch worker did not exit after cancellation")
	}
}

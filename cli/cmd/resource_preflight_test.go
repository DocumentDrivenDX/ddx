package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCommandResourceChecker struct {
	calls  int
	result agent.ExecutionResourceCheckResult
	err    error
}

func (f *fakeCommandResourceChecker) Check(ctx context.Context) (agent.ExecutionResourceCheckResult, error) {
	_ = ctx
	f.calls++
	return f.result, f.err
}

type fakeCleanupRunner struct {
	calls int
}

func (f *fakeCleanupRunner) Cleanup(ctx context.Context) (agent.ExecutionCleanupSummary, error) {
	_ = ctx
	f.calls++
	return agent.ExecutionCleanupSummary{ProjectRoot: "fake", TempRoot: "fake"}, nil
}

func seedOpenBead(t *testing.T, root, beadID string) {
	t.Helper()
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{ID: beadID, Title: "resource preflight bead"}))
}

func TestTryResourcePreflight_FailsBeforeClaim(t *testing.T) {
	projectRoot := t.TempDir()
	beadID := "ddx-resource-try"
	seedOpenBead(t, projectRoot, beadID)

	tempRoot := t.TempDir()
	cleanup := &fakeCleanupRunner{}
	checker := &agent.ExecutionResourcePreflight{
		ProjectRoot: projectRoot,
		TempRoot:    tempRoot,
		EvidenceRoots: []string{
			filepath.Join(projectRoot, ddxroot.DirName, "executions"),
		},
		CleanupRunner: cleanup,
		RootProbe: func(path string) (agent.ExecutionResourceRootCheck, error) {
			return agent.ExecutionResourceRootCheck{
				Path:       path,
				Writable:   true,
				BytesFree:  1,
				InodesFree: 1,
			}, nil
		},
	}

	factory := NewCommandFactory(projectRoot)
	factory.resourceCheckerOverride = checker
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		t.Fatalf("executor must not run when resource preflight fails")
		return agent.ExecuteBeadReport{}, fmt.Errorf("unexpected executor call")
	})

	out, err := executeCommand(factory.NewRootCommand(), "try", beadID)
	require.Error(t, err)
	assert.Contains(t, out, "resource_exhausted")
	assert.Equal(t, 1, cleanup.calls)

	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	got, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
}

func TestWorkResourcePreflight_FailsBeforeClaim(t *testing.T) {
	projectRoot := t.TempDir()
	beadID := "ddx-resource-work"
	seedOpenBead(t, projectRoot, beadID)

	checker := &fakeCommandResourceChecker{
		err: &agent.ResourceExhaustedError{
			Detail: "temp root and evidence root are full",
		},
	}

	factory := NewCommandFactory(projectRoot)
	factory.resourceCheckerOverride = checker

	out, err := executeCommand(factory.NewRootCommand(), "work", "--once")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(out), "resource_exhausted")
	assert.Equal(t, 1, checker.calls)

	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	got, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
}

func TestTryResourceExhaustionEndToEnd_Reclaimable(t *testing.T) {
	projectRoot := t.TempDir()
	beadID := "ddx-resource-try-reclaimable"
	seedOpenBead(t, projectRoot, beadID)

	result := agent.ExecutionResourceCheckResult{
		ProjectRoot: projectRoot,
		TempRoot:    filepath.Join(projectRoot, ddxroot.DirName, "tmp"),
		EvidenceRoots: []string{
			filepath.Join(projectRoot, ddxroot.DirName, "executions"),
		},
		CleanupSummary: agent.ExecutionCleanupSummary{
			ProjectRoot:                 projectRoot,
			TempRoot:                    filepath.Join(projectRoot, ddxroot.DirName, "tmp"),
			RemovedUnregisteredTempDirs: 1,
			BytesReclaimed:              1024,
		},
	}
	checker := &fakeCommandResourceChecker{
		result: result,
		err: &agent.ResourceExhaustedError{
			Detail: "temp root and evidence root are full",
			Result: result,
		},
	}

	factory := NewCommandFactory(projectRoot)
	factory.resourceCheckerOverride = checker
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		t.Fatalf("executor must not run when try resource preflight fails")
		return agent.ExecuteBeadReport{}, fmt.Errorf("unexpected executor call")
	})

	out, err := executeCommand(factory.NewRootCommand(), "try", beadID)
	require.Error(t, err)
	assert.Contains(t, out, "resource_exhausted")
	assert.Equal(t, 1, checker.calls)

	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	got, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	if got.Extra != nil {
		_, hasRetry := got.Extra["work-retry-after"]
		assert.False(t, hasRetry, "resource exhaustion must not write work-retry-after")
	}
}

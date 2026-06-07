package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoChangesWithoutRationaleDoesNotUseNoProgressCooldown verifies TD-031's
// no_changes rule: absent rationale is a bad attempt / triage signal, not a
// retry-later condition. The configured no_progress_cooldown still resolves,
// but no_changes does not consume it unless the try layer returns an explicit
// retry-later lifecycle action.
func TestNoChangesWithoutRationaleDoesNotUseNoProgressCooldown(t *testing.T) {
	const (
		cooldown = 47 * time.Minute
		beadID   = "ddx-npc-001"
		fixedRev = "feedface00112233"
	)

	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	// Real on-disk .ddx/config.yaml. max_no_changes_before_close is set
	// high enough that adjudication never closes the bead — the test proves
	// the cooldown knob is not used for unjustified no_changes.
	cfgYAML := `version: "1.0"
library:
  path: ./library
  repository:
    url: https://github.com/example/repo
    branch: main
workers:
  no_progress_cooldown: "47m"
  max_no_changes_before_close: 99
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfgYAML), 0o644))

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{ID: beadID, Title: "no-progress cooldown e2e", Priority: 0}))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusNoChanges,
				SessionID: "sess-npc",
				BaseRev:   fixedRev,
				ResultRev: fixedRev,
			}, nil
		}),
	}

	rcfg, err := config.LoadAndResolve(projectRoot, config.CLIOverrides{Assignee: "npc-worker"})
	require.NoError(t, err)
	require.Equal(t, cooldown, rcfg.NoProgressCooldown(),
		"LoadAndResolve must surface workers.no_progress_cooldown from .ddx/config.yaml")

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Attempts)
	require.Len(t, result.Results, 1)

	retryStr := result.Results[0].RetryAfter
	require.Empty(t, retryStr,
		"loop must not record RetryAfter on unjustified no_changes by default")

	got, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	require.NotNil(t, got.Extra)
	_, hasRetry := got.Extra["work-retry-after"]
	assert.False(t, hasRetry, "store must not persist work-retry-after for unjustified no_changes")
	assert.Contains(t, got.Labels, NoChangesLabelUnjustified)
	assert.Equal(t, "open", got.Status,
		"bead must remain open after a single unjustified no_changes attempt")
}

// TestMaxNoChangesBeforeCloseFromConfig was originally the SD-024 Stage 1
// behavioral proof that workers.max_no_changes_before_close in
// .ddx/config.yaml drove bead closure after the configured number of
// consecutive no_changes attempts. Under NoChangesContract (TD-031 §8.1,
// ddx-b24e9630) that count-based path no longer exists; the bead now closes
// on a verification_command marker in the rationale. The test is preserved
// as proof that LoadAndResolve still surfaces the knob and that a verified
// rationale closes on the first attempt.
func TestMaxNoChangesBeforeCloseFromConfig(t *testing.T) {
	const (
		threshold = 2
		beadID    = "ddx-mnc-001"
	)

	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	cfgYAML := `version: "1.0"
library:
  path: ./library
  repository:
    url: https://github.com/example/repo
    branch: main
workers:
  max_no_changes_before_close: 2
  no_progress_cooldown: "10m"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfgYAML), 0o644))

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{ID: beadID, Title: "max no_changes before close e2e", Priority: 0}))

	var execCalls atomic.Int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			execCalls.Add(1)
			return ExecuteBeadReport{
				BeadID:             id,
				Status:             ExecuteBeadStatusNoChanges,
				SessionID:          "sess-mnc",
				NoChangesRationale: "verification_command: true\noutput: ok",
			}, nil
		}),
		VerificationRunner: func(ctx context.Context, projectRoot, command string) (int, string, error) {
			return 0, "ok", nil
		},
	}

	rcfg, err := config.LoadAndResolve(projectRoot, config.CLIOverrides{Assignee: "mnc-worker"})
	require.NoError(t, err)
	require.Equal(t, threshold, rcfg.MaxNoChangesBeforeClose(),
		"LoadAndResolve must surface workers.max_no_changes_before_close from .ddx/config.yaml")

	runtime := ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(),
	}

	res1, err := worker.Run(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	require.Equal(t, 1, res1.Attempts, "iteration 1 must run exactly once")
	require.Equal(t, 1, res1.Successes, "verified rationale closes on the first attempt")

	final, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, "closed", final.Status, "bead must be closed under NoChangesContract verified path")

	events, err := store.Events(beadID)
	require.NoError(t, err)
	var sawAlreadySatisfied bool
	for _, ev := range events {
		if ev.Kind == "execute-bead" {
			if ev.Summary == ExecuteBeadStatusAlreadySatisfied ||
				ev.Body == ExecuteBeadStatusAlreadySatisfied ||
				containsToken(ev.Body, ExecuteBeadStatusAlreadySatisfied) {
				sawAlreadySatisfied = true
			}
		}
	}
	assert.True(t, sawAlreadySatisfied,
		"expected an execute-bead event recording already_satisfied; events=%+v", events)

	assert.Equal(t, int32(1), execCalls.Load(),
		"verified rationale closes on the first attempt; executor invoked exactly once")
}

func containsToken(body, token string) bool {
	for i := 0; i+len(token) <= len(body); i++ {
		if body[i:i+len(token)] == token {
			return true
		}
	}
	return false
}

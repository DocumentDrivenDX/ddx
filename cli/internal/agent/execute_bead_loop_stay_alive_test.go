package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type claimSuccessRateWarningStore struct {
	*bead.Store
	attempts atomic.Int32
	cancel   context.CancelFunc
}

func (s *claimSuccessRateWarningStore) Claim(id, assignee string) error {
	return s.claim(id, assignee, "", "")
}

func (s *claimSuccessRateWarningStore) ClaimWithOptions(id, assignee, session, worktree string) error {
	return s.claim(id, assignee, session, worktree)
}

func (s *claimSuccessRateWarningStore) claim(id, assignee, session, worktree string) error {
	attempt := s.attempts.Add(1)
	switch attempt {
	case 1, 2, 3, 5:
		if attempt == 5 && s.cancel != nil {
			defer s.cancel()
		}
		return fmt.Errorf("synthetic claim failure %d", attempt)
	default:
		return s.Store.ClaimWithOptions(id, assignee, session, worktree)
	}
}

// TestLoop_StaysAliveWithEmptyQueue covers watch mode: the loop must
// NOT exit when nextCandidate returns no
// eligible bead. It must reset the per-Run attempted/hookFailed maps and
// sleep, then poll again. Cancelling the context is the only way out.
func TestLoop_StaysAliveWithEmptyQueue(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run on an empty queue")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Use a tiny poll interval so the test runs quickly; cancel after enough
	// wall-clock for several empty-poll cycles to confirm the loop is still
	// running and not bailing out after the first empty poll.
	ctx, cancel := context.WithCancel(context.Background())
	pollInterval := 5 * time.Millisecond
	cancelAfter := 50 * time.Millisecond
	go func() {
		time.Sleep(cancelAfter)
		cancel()
	}()

	start := time.Now()
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: pollInterval,
	})
	elapsed := time.Since(start)

	// Context cancellation surfaces as the returned error. The key
	// invariant: the loop survived past the first empty poll.
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Attempts)
	assert.True(t, elapsed >= cancelAfter,
		"loop must run until ctx cancellation; ran for %s, expected >= %s", elapsed, cancelAfter)
}

func TestLoop_BinaryRefreshStopsBeforeClaim(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run after binary refresh requests a restart")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	checks := 0
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
		BinaryRefreshCheck: func(ctx context.Context) (bool, error) {
			checks++
			return true, nil
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, checks)
	assert.Equal(t, 0, result.Attempts)
	assert.Equal(t, "BinaryRefresh", result.StopCondition)
	assert.Equal(t, "binary_refresh", result.ExitReason)
	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
}

func TestWorkWatch_SystemicPreClaimErrorIdlesWithoutCooldown(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run while pre-claim hook is systemically blocked")
			return ExecuteBeadReport{}, nil
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	progressCh := make(chan ProgressEvent, 8)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case evt := <-progressCh:
				if evt.Phase == "loop.idle" && evt.Message == "preclaim_systemic" {
					cancel()
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Hour,
		Log:          &logBuf,
		ProgressCh:   progressCh,
		PreClaimHook: func(ctx context.Context) error {
			return fmt.Errorf("local branch main has diverged from origin (local=abc origin=def); reconcile manually before claiming")
		},
	})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Attempts)
	assert.Equal(t, 0, result.Failures)

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	gotSecond, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
	assert.Equal(t, bead.StatusOpen, gotSecond.Status)
	assert.Nil(t, gotFirst.Extra["work-retry-after"])
	assert.Nil(t, gotSecond.Extra["work-retry-after"])

	out := logBuf.String()
	assert.Contains(t, out, "systemic; leaving beads untouched")
	assert.Contains(t, out, "pre-claim hook blocked queue:")
	assert.Equal(t, 1, strings.Count(out, "systemic; leaving beads untouched"))
}

// TestPreClaimWarnSameFingerprintEscalatesAfterThreshold verifies that the
// loop escalates repeated identical pre-claim warn fingerprints across
// distinct bead IDs, stops before claiming the threshold bead, and leaves the
// remaining ready queue untouched.
func TestPreClaimWarnSameFingerprintEscalatesAfterThreshold(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	beadIDs := []string{
		"ddx-preclaim-warn-1",
		"ddx-preclaim-warn-2",
		"ddx-preclaim-warn-3",
		"ddx-preclaim-warn-4",
		"ddx-preclaim-warn-5",
	}
	for i, beadID := range beadIDs {
		require.NoError(t, store.Create(&bead.Bead{
			ID:       beadID,
			Title:    "warn bead " + beadID,
			Priority: i,
		}))
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				ResultRev: "rev-" + beadID,
			}, nil
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var eventSink, logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:                        executeloop.ModeDrain,
		Log:                         &logBuf,
		EventSink:                   &eventSink,
		NoReview:                    true,
		PreClaimWarnRepeatThreshold: DefaultPreClaimWarnRepeatThreshold,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeError,
				Reason:  "system_unready",
				Detail:  "shared readiness schema mismatch",
			}, nil
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, len(beadIDs)-1, result.Attempts)
	assert.Equal(t, len(beadIDs)-1, result.Successes)
	assert.Equal(t, 0, result.Failures)
	require.NotNil(t, result.OperatorAttention)
	assert.Equal(t, "preclaim_warn_repeated", result.OperatorAttention.Reason)
	assert.Equal(t, beadIDs[len(beadIDs)-1], result.OperatorAttention.BeadID)
	assert.Equal(t, "operator_attention", result.ExitReason)
	assert.Equal(t, "OperatorAttention", result.StopCondition)

	for _, beadID := range beadIDs[:len(beadIDs)-1] {
		got, err := store.Get(beadID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, got.Status)
		assert.Nil(t, got.Extra["work-retry-after"])
	}
	lastBead, err := store.Get(beadIDs[len(beadIDs)-1])
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, lastBead.Status)

	lines := strings.Split(strings.TrimSpace(eventSink.String()), "\n")
	var escalations []map[string]any
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] != "loop.operator_attention" {
			continue
		}
		data, ok := entry["data"].(map[string]any)
		require.True(t, ok)
		if data["reason"] == "preclaim_warn_repeated" {
			escalations = append(escalations, data)
		}
	}
	require.Len(t, escalations, 1, "exactly one repeated-warn escalation is expected")
	esc := escalations[0]
	assert.NotEmpty(t, esc["fingerprint"])
	assert.Equal(t, float64(DefaultPreClaimWarnRepeatThreshold), esc["count"])
	distinctIDs, ok := esc["distinct_bead_ids"].([]any)
	require.True(t, ok, "distinct_bead_ids must be a JSON array")
	assert.Len(t, distinctIDs, DefaultPreClaimWarnRepeatThreshold)
	assert.Equal(t, "preclaim_warn_repeated", esc["reason"])
	assert.Equal(t, "shared readiness schema mismatch", esc["example_detail"])
	assert.NotEmpty(t, esc["example_payload"])

	lastEvents, err := store.Events(beadIDs[len(beadIDs)-1])
	require.NoError(t, err)
	var operatorAttentionEvent *bead.BeadEvent
	for i := range lastEvents {
		if lastEvents[i].Kind == "operator_attention" && lastEvents[i].Summary == "preclaim_warn_repeated" {
			operatorAttentionEvent = &lastEvents[i]
			break
		}
	}
	require.NotNil(t, operatorAttentionEvent, "a durable operator_attention event must be recorded")
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(operatorAttentionEvent.Body), &body))
	assert.Equal(t, "preclaim_warn_repeated", body["reason"])
	assert.Equal(t, "shared readiness schema mismatch", body["example_detail"])
	assert.Equal(t, float64(DefaultPreClaimWarnRepeatThreshold), body["count"])
	assert.NotEmpty(t, body["fingerprint"])
}

// TestWorkWatch_PreClaimIdleEscalatesAfterRepeatedSameDetail covers
// ddx-df77e668 AC #3: when the worker idles on the same pre-claim blocker for
// preClaimIdleEscalationThreshold consecutive cycles, it emits a non-terminal
// loop.operator_attention event carrying the bead-id, detail, and elapsed-idle
// instead of looping silently. The beads are never cooldowned, and the loop
// keeps idling (the escalation does not stop the worker).
func TestWorkWatch_PreClaimIdleEscalatesAfterRepeatedSameDetail(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Errorf("executor must not run while pre-claim idles")
			return ExecuteBeadReport{}, nil
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	progressCh := make(chan ProgressEvent, 256)
	done := make(chan struct{})
	var idleCount int32
	go func() {
		for {
			select {
			case evt := <-progressCh:
				if evt.Phase == "loop.idle" && evt.Message == preClaimIdleReasonTrackerContention {
					if atomic.AddInt32(&idleCount, 1) >= int32(preClaimIdleEscalationThreshold)+2 {
						cancel()
						return
					}
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	var eventSink, logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Millisecond,
		Log:          &logBuf,
		EventSink:    &eventSink,
		ProgressCh:   progressCh,
		SessionID:    "sess-escalate",
		WorkerID:     "worker-escalate",
		PreClaimHook: func(ctx context.Context) error {
			return errors.New("landing worktree has staged changes after waiting 2s:\nM\t.ddx/beads.jsonl\nM\t.ddx/metrics/attempts.jsonl")
		},
	})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Attempts)
	assert.Equal(t, 0, result.Failures)
	// The escalation is non-terminal: it does not populate a terminal stop.
	assert.Nil(t, result.OperatorAttention)

	// Beads stay open and uncooldowned — tracker contention never faults a bead.
	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	gotSecond, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
	assert.Equal(t, bead.StatusOpen, gotSecond.Status)
	assert.Nil(t, gotFirst.Extra["work-retry-after"])

	// Exactly one operator-attention escalation event is emitted for the streak.
	lines := strings.Split(strings.TrimSpace(eventSink.String()), "\n")
	var escalations []map[string]any
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] != "loop.operator_attention" {
			continue
		}
		data, ok := entry["data"].(map[string]any)
		require.True(t, ok)
		if data["reason"] == "preclaim_idle_escalation" {
			escalations = append(escalations, data)
		}
	}
	require.Len(t, escalations, 1, "exactly one escalation event per same-detail streak")
	esc := escalations[0]
	assert.Contains(t, []any{first.ID, second.ID}, esc["bead_id"], "escalation must name a skipped bead")
	assert.Contains(t, esc["detail"], ".ddx/beads.jsonl", "escalation must carry the blocker detail")
	elapsed, ok := esc["elapsed_idle"].(string)
	require.True(t, ok, "escalation must carry elapsed_idle")
	assert.NotEmpty(t, elapsed)
	idleCountVal, ok := esc["idle_count"].(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, int(idleCountVal), preClaimIdleEscalationThreshold)

	assert.Contains(t, logBuf.String(), "operator attention: pre-claim idled")
}

// TestExecuteBeadLoop_ClaimSuccessRateWarnsBelowThreshold verifies the
// rolling claim-success signal: a full window of non-successful claim attempts
// emits exactly one operator-attention warning, a later successful claim
// clears the warned state, and the next below-threshold crossing warns again.
func TestExecuteBeadLoop_ClaimSuccessRateWarnsBelowThreshold(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	for i := 1; i <= 3; i++ {
		require.NoError(t, store.Create(&bead.Bead{
			ID:       fmt.Sprintf("ddx-claim-rate-%d", i),
			Title:    fmt.Sprintf("claim rate bead %d", i),
			Priority: i,
		}))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workerStore := &claimSuccessRateWarningStore{Store: store, cancel: cancel}
	worker := &ExecuteBeadWorker{
		Store: workerStore,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				ResultRev: "rev-" + beadID,
			}, nil
		}),
	}

	var logBuf, eventSink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:                      executeloop.ModeDrain,
		Log:                       &logBuf,
		EventSink:                 &eventSink,
		WorkerID:                  "worker-claim-rate",
		SessionID:                 "sess-claim-rate",
		ClaimSuccessRateWindow:    3,
		ClaimSuccessRateThreshold: 0.5,
	})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, 2, strings.Count(logBuf.String(), "claim success rate"), "warning should emit once per below-threshold crossing")

	lines := strings.Split(strings.TrimSpace(eventSink.String()), "\n")
	var warnings []map[string]any
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] != "loop.operator_attention" {
			continue
		}
		data, ok := entry["data"].(map[string]any)
		require.True(t, ok)
		if data["reason"] != "claim_success_rate_below_threshold" {
			continue
		}
		warnings = append(warnings, data)
	}

	require.Len(t, warnings, 2, "a later success must clear the warned state so a new crossing warns again")
	assert.Equal(t, float64(3), warnings[0]["window_size"])
	assert.Equal(t, float64(0.5), warnings[0]["threshold"])
	assert.Equal(t, float64(0), warnings[0]["successes"])
	assert.Equal(t, float64(3), warnings[0]["non_successes"])
	assert.Equal(t, float64(3), warnings[1]["window_size"])
	assert.Equal(t, float64(0.5), warnings[1]["threshold"])
	assert.Equal(t, float64(1), warnings[1]["successes"])
	assert.Equal(t, float64(2), warnings[1]["non_successes"])
}

func TestWorkLoop_PreDispatchDirtyImplementationPreservesAndContinues(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	first := &bead.Bead{ID: "ddx-preserve-0001", Title: "Preserve dirty root", IssueType: "task"}
	require.NoError(t, store.Create(first))

	projectRoot, _ := newScriptHarnessRepo(t, 1)
	dirtyRel := filepath.Join("cli", "internal", "agent", "dirty_impl.go")
	dirtyPath := filepath.Join(projectRoot, dirtyRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(dirtyPath), 0o755))
	dirtyContent := "package agent\n"
	require.NoError(t, os.WriteFile(dirtyPath, []byte(dirtyContent), 0o644))

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			call := atomic.AddInt32(&execCalls, 1)
			if call == 1 {
				return ExecuteBeadReport{}, fmt.Errorf(
					"%s%s; commit or clean those files before rerunning so the bead's [ddx-<id>] substantive commit stays intentional",
					preExecuteCheckpointDirtyMarker,
					filepath.ToSlash(dirtyRel),
				)
			}
			require.Empty(t, runGitInteg(t, projectRoot, "status", "--short", "--", filepath.ToSlash(dirtyRel)),
				"preserved dirty path must be restored to HEAD before the next dispatch")
			_, showErr := runGitIntegOutput(projectRoot, "show", "HEAD:"+filepath.ToSlash(dirtyRel))
			require.Error(t, showErr, "the dirty implementation file must not be folded into HEAD")
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-preserved",
				ResultRev: "c0ffee",
			}, nil
		}),
	}

	var logBuf, eventSink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Hour,
		Log:          &logBuf,
		WakeCh:       make(chan struct{}),
		EventSink:    &eventSink,
		ProjectRoot:  projectRoot,
		SessionID:    "sess-preserve-watch",
		WorkerID:     "worker-preserve-watch",
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotNil(t, result)
	assert.Nil(t, result.OperatorAttention)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, int32(2), atomic.LoadInt32(&execCalls), "watch mode must redispatch after preserving root dirt")

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotFirst.Status)

	lines := strings.Split(strings.TrimSpace(eventSink.String()), "\n")
	var preserved map[string]any
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] != "loop.pre_dispatch_dirty_preserved" {
			continue
		}
		data, ok := entry["data"].(map[string]any)
		require.True(t, ok, "loop.pre_dispatch_dirty_preserved must include data")
		preserved = data
		break
	}
	require.NotNil(t, preserved, "watch mode must emit a preservation event instead of operator attention")
	assert.Equal(t, first.ID, preserved["bead_id"])
	assert.Equal(t, projectRoot, preserved["project_root"])
	require.Len(t, preserved["dirty_paths"], 1)
	assert.Contains(t, preserved["dirty_paths"], filepath.ToSlash(dirtyRel))
	preserveRef, ok := preserved["preserve_ref"].(string)
	require.True(t, ok)
	require.NotEmpty(t, preserveRef)
	recoverCommand, ok := preserved["recover_command"].(string)
	require.True(t, ok)
	assert.Equal(t, "git stash apply "+preserveRef, recoverCommand)
	assert.NotEmpty(t, runGitInteg(t, projectRoot, "rev-parse", preserveRef))

	out := logBuf.String()
	assert.Contains(t, out, preserveRef)
	assert.Contains(t, out, filepath.ToSlash(dirtyRel))
	assert.Contains(t, out, recoverCommand)
	assert.Empty(t, runGitInteg(t, projectRoot, "status", "--short", "--", filepath.ToSlash(dirtyRel)))

	applyOut, applyErr := runGitIntegOutput(projectRoot, "stash", "apply", preserveRef)
	require.NoError(t, applyErr, applyOut)
	assert.Contains(t, runGitInteg(t, projectRoot, "status", "--short", "--", filepath.ToSlash(dirtyRel)), "?? "+filepath.ToSlash(dirtyRel))
}

func TestWorkWatchDoesNotPreserveJustLandedPathsBeforeNextClaim(t *testing.T) {
	if testing.Short() {
		t.Skip("watch timing is race-sensitive under -short; covered by the full suite")
	}

	projectRoot, _ := newScriptHarnessRepo(t, 2)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))

	trackedRel := filepath.Join("cli", "cmd", "run_test_helpers.go")
	trackedPath := filepath.Join(projectRoot, trackedRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(trackedPath), 0o755))
	require.NoError(t, os.WriteFile(trackedPath, []byte("package cmd\n"), 0o644))
	runGitInteg(t, projectRoot, "add", trackedRel)
	runGitInteg(t, projectRoot, "commit", "-m", "test: seed tracked helper")

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		`run if [ "$DDX_BEAD_ID" = "ddx-int-0001" ]; then mv cli/cmd/run_test_helpers.go cli/cmd/run_test_helpers_renamed.go; else printf 'package cmd\n' > cli/cmd/second_dispatch.go; fi`,
		`commit test: ${DDX_BEAD_ID}`,
	})

	originalLister := preDispatchDirtyPathLister
	t.Cleanup(func() {
		preDispatchDirtyPathLister = originalLister
	})
	var callCount atomic.Int32
	preDispatchDirtyPathLister = func(root string) ([]string, error) {
		require.Equal(t, projectRoot, root)
		switch callCount.Add(1) {
		case 1:
			return nil, nil
		case 2:
			return []string{
				filepath.ToSlash(trackedRel),
				"cli/cmd/run_test_helpers_renamed.go",
			}, nil
		case 3:
			return nil, nil
		default:
			return nil, nil
		}
	}

	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: scriptHarnessExecutor(t, projectRoot, dirFile),
	}

	var eventSink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Hour,
		EventSink:    &eventSink,
		ProjectRoot:  projectRoot,
		SessionID:    "sess-watch-stable-predispatch",
		WorkerID:     "worker-watch-stable-predispatch",
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 2, result.Successes)
	assert.Equal(t, 0, result.Failures)
	assert.Nil(t, result.OperatorAttention)
	assert.NotContains(t, eventSink.String(), `"type":"loop.pre_dispatch_dirty_preserved"`)

	first, getErr := store.Get("ddx-int-0001")
	require.NoError(t, getErr)
	second, getErr := store.Get("ddx-int-0002")
	require.NoError(t, getErr)
	assert.Equal(t, bead.StatusClosed, first.Status)
	assert.Equal(t, bead.StatusClosed, second.Status)

	refsOut, refsErr := runGitIntegOutput(projectRoot, "for-each-ref", "--format=%(refname)", "refs/ddx/pre-dispatch")
	require.NoError(t, refsErr)
	assert.Empty(t, strings.TrimSpace(refsOut))
}

func TestPreDispatchDirtyPreserveRequiresStableImplementationDirt(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		`create-file stable_after_recheck.txt ok`,
		`commit test: ${DDX_BEAD_ID}`,
	})

	originalLister := preDispatchDirtyPathLister
	t.Cleanup(func() {
		preDispatchDirtyPathLister = originalLister
	})
	var callCount atomic.Int32
	preDispatchDirtyPathLister = func(root string) ([]string, error) {
		require.Equal(t, projectRoot, root)
		switch callCount.Add(1) {
		case 1:
			return []string{"cli/internal/agent/transient_impl.go"}, nil
		case 2:
			return nil, nil
		default:
			return nil, nil
		}
	}

	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: scriptHarnessExecutor(t, projectRoot, dirFile),
	}

	var eventSink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Hour,
		EventSink:    &eventSink,
		ProjectRoot:  projectRoot,
		SessionID:    "sess-watch-transient-predispatch",
		WorkerID:     "worker-watch-transient-predispatch",
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)
	assert.Nil(t, result.OperatorAttention)
	assert.NotContains(t, eventSink.String(), `"type":"loop.pre_dispatch_dirty_preserved"`)

	got, getErr := store.Get("ddx-int-0001")
	require.NoError(t, getErr)
	assert.Equal(t, bead.StatusClosed, got.Status)

	refsOut, refsErr := runGitIntegOutput(projectRoot, "for-each-ref", "--format=%(refname)", "refs/ddx/pre-dispatch")
	require.NoError(t, refsErr)
	assert.Empty(t, strings.TrimSpace(refsOut))
}

func TestLoop_WatchCheckpointDirtyStopsWithoutRetry(t *testing.T) {
	inner, first, second := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	dirtyPaths := []string{"cli/cmd/execute_loop_shared.go", "cli/internal/agent/dirty_impl.go"}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{}, fmt.Errorf(
				"%s%s; commit or clean those files before rerunning so the bead's [ddx-<id>] substantive commit stays intentional",
				preExecuteCheckpointDirtyMarker,
				strings.Join(dirtyPaths, ", "),
			)
		}),
	}

	var logBuf, eventSink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Hour,
		Log:          &logBuf,
		WakeCh:       make(chan struct{}),
		EventSink:    &eventSink,
		ProjectRoot:  "/repo/watch",
		SessionID:    "sess-watch",
		WorkerID:     "worker-watch",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Attempts)
	assert.Equal(t, 0, result.Failures)
	assert.Empty(t, result.Results)
	assert.Equal(t, "OperatorAttention", result.StopCondition)
	assert.Equal(t, "operator_attention", result.ExitReason)
	require.NotNil(t, result.OperatorAttention)
	assert.Equal(t, "checkpoint_dirty", result.OperatorAttention.Reason)
	assert.Equal(t, first.ID, result.OperatorAttention.BeadID)
	assert.Equal(t, "/repo/watch", result.OperatorAttention.ProjectRoot)
	assert.Equal(t, dirtyPaths, result.OperatorAttention.DirtyPaths)
	assert.Contains(t, result.OperatorAttention.Message, "commit or clean")

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "only the first ready bead may be claimed while the tree is dirty")

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	gotSecond, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
	assert.Equal(t, bead.StatusOpen, gotSecond.Status)
	assert.Empty(t, gotFirst.Owner)
	assert.Empty(t, gotSecond.Owner)

	out := logBuf.String()
	assert.Contains(t, out, "operator attention: project worktree /repo/watch has uncommitted implementation changes; released ddx-0001.")
	assert.Contains(t, out, "commit or clean")
	assert.NotContains(t, out, "will retry")

	lines := strings.Split(strings.TrimSpace(eventSink.String()), "\n")
	var operatorAttention map[string]any
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] != "loop.operator_attention" {
			continue
		}
		data, ok := entry["data"].(map[string]any)
		require.True(t, ok, "loop.operator_attention must include data")
		operatorAttention = data
		break
	}
	require.NotNil(t, operatorAttention, "loop.operator_attention event must be emitted")
	assert.Equal(t, first.ID, operatorAttention["bead_id"])
	assert.Equal(t, "/repo/watch", operatorAttention["project_root"])
	assert.Equal(t, "checkpoint_dirty", operatorAttention["reason"])
	assert.Contains(t, operatorAttention["message"], "commit or clean")
	require.Len(t, operatorAttention["dirty_paths"], len(dirtyPaths))
}

func TestLoop_WatchCheckpointDirtyPreserveFailureFallsBackToOperatorAttention(t *testing.T) {
	inner, first, second := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	dirtyPaths := []string{"cli/internal/agent/dirty_impl.go"}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{}, fmt.Errorf(
				"%s%s; commit or clean those files before rerunning so the bead's [ddx-<id>] substantive commit stays intentional",
				preExecuteCheckpointDirtyMarker,
				strings.Join(dirtyPaths, ", "),
			)
		}),
		preDispatchDirtyPreserver: func(projectRoot string, dirtyPaths []string) (*PreDispatchDirtyPreservation, error) {
			return nil, errors.New("stash failed")
		},
	}

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode:        executeloop.ModeWatch,
		Log:         &logBuf,
		ProjectRoot: "/repo/watch",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "OperatorAttention", result.StopCondition)
	assert.Equal(t, "operator_attention", result.ExitReason)
	require.NotNil(t, result.OperatorAttention)
	assert.Equal(t, "checkpoint_dirty", result.OperatorAttention.Reason)
	assert.Equal(t, first.ID, result.OperatorAttention.BeadID)
	assert.Equal(t, "/repo/watch", result.OperatorAttention.ProjectRoot)
	assert.Equal(t, dirtyPaths, result.OperatorAttention.DirtyPaths)
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "checkpoint dirt fallback must still stop before a second claim")

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	gotSecond, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
	assert.Equal(t, bead.StatusOpen, gotSecond.Status)
	assert.Empty(t, gotFirst.Owner)
	assert.Empty(t, gotSecond.Owner)
	assert.Contains(t, logBuf.String(), "commit or clean")
}

func TestLoop_DrainCheckpointDirtyStopsQueue(t *testing.T) {
	inner, first, second := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			return ExecuteBeadReport{}, fmt.Errorf(
				"%s%s; commit or clean those files before rerunning so the bead's [ddx-<id>] substantive commit stays intentional",
				preExecuteCheckpointDirtyMarker,
				"cli/internal/agent/dirty_impl.go",
			)
		}),
	}

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode:        executeloop.ModeDrain,
		Log:         &logBuf,
		ProjectRoot: "/repo/drain",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), atomic.LoadInt32(&execCalls), "only the first bead may reach the checkpoint refusal")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "the second ready bead must not be claimed while the same dirty-tree blocker remains")
	assert.Equal(t, "OperatorAttention", result.StopCondition)
	assert.Equal(t, "operator_attention", result.ExitReason)
	require.NotNil(t, result.OperatorAttention)
	assert.Equal(t, "/repo/drain", result.OperatorAttention.ProjectRoot)
	assert.Equal(t, []string{"cli/internal/agent/dirty_impl.go"}, result.OperatorAttention.DirtyPaths)

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	gotSecond, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
	assert.Equal(t, bead.StatusOpen, gotSecond.Status)
	assert.Empty(t, gotFirst.Owner)
	assert.Empty(t, gotSecond.Owner)

	assert.Contains(t, logBuf.String(), "commit or clean")
}

func TestWorkWatchIdleStdout_PrintsQueueStatusAndHumanBlockers(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	seedWatchIdleQueue(t, store)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run on an idle queue")
			return ExecuteBeadReport{}, nil
		}),
		Now: fixedWatchStdoutTime,
	}

	ctx, cancel := context.WithCancel(context.Background())
	progressCh := make(chan ProgressEvent, 16)
	done := make(chan struct{})
	var idleEvents int32
	go func() {
		for {
			select {
			case evt := <-progressCh:
				if evt.Phase == "loop.idle" {
					atomic.AddInt32(&idleEvents, 1)
					cancel()
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: 30 * time.Second,
		Log:          &logBuf,
		ProgressCh:   progressCh,
	})
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	require.NotNil(t, result.QueueSnapshot)
	assert.Equal(t, 3, result.QueueSnapshot.HumanReviewBlockerCount)
	assert.Equal(t, 30, result.QueueSnapshot.HumanReviewBlockedTotal)
	require.Len(t, result.QueueSnapshot.HumanReviewBlockers, 3)
	assert.Equal(t, int32(1), atomic.LoadInt32(&idleEvents), "watch mode must still emit loop.idle progress")

	out := logBuf.String()
	assert.Contains(t, out, "12:34:56 idle: no execution-ready beads; sleeping 30s")
	assert.Contains(t, out, "queue: execution-ready=0")
	assert.Contains(t, out, "operator-attention=4") // 3 proposed blockers + 1 standalone proposed bead
	assert.Contains(t, out, "needs-human/investigation=3")
	assert.Contains(t, out, "cooldown/deferred=1")
	assert.Contains(t, out, "next-retry=")
	assert.Contains(t, out, "execution-ineligible=1")
	assert.Contains(t, out, "superseded=1")
	assert.Contains(t, out, "epics=1")
	assert.Contains(t, out, "epic-closure-candidates=1")
	assert.Contains(t, out, "30 beads blocked behind 3 needs-human blockers")
	assert.Contains(t, out, "1. ddx-human-1 Needs human 1 (10 downstream)")
	assert.Contains(t, out, "2. ddx-human-2 Needs human 2 (10 downstream)")
	assert.Contains(t, out, "3. ddx-human-3 Needs human 3 (10 downstream)")
}

func TestWorkWatchIdleStdout_RepeatedPollKeepsCompactQueueStatus(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	seedWatchIdleQueue(t, store)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run on an idle queue")
			return ExecuteBeadReport{}, nil
		}),
		Now: fixedWatchStdoutTime,
	}

	ctx, cancel := context.WithCancel(context.Background())
	wakeCh := make(chan struct{}, 1)
	progressCh := make(chan ProgressEvent, 16)
	done := make(chan struct{})
	var idleEvents int32
	go func() {
		for {
			select {
			case evt := <-progressCh:
				if evt.Phase != "loop.idle" {
					continue
				}
				if atomic.AddInt32(&idleEvents, 1) == 1 {
					wakeCh <- struct{}{}
				} else {
					cancel()
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:         executeloop.ModeWatch,
		IdleInterval: time.Hour,
		Log:          &logBuf,
		ProgressCh:   progressCh,
		WakeCh:       wakeCh,
	})
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, int32(2), atomic.LoadInt32(&idleEvents))

	out := logBuf.String()
	assert.Equal(t, 2, strings.Count(out, "idle: no execution-ready beads; sleeping 1h0m0s"))
	assert.Equal(t, 2, strings.Count(out, "queue: execution-ready=0"))
	assert.Equal(t, 2, strings.Count(out, "30 beads blocked behind 3 needs-human blockers"))
	assert.Equal(t, 1, strings.Count(out, "ddx-human-1 Needs human 1 (10 downstream)"))
	assert.Equal(t, 1, strings.Count(out, "ddx-human-2 Needs human 2 (10 downstream)"))
	assert.Equal(t, 1, strings.Count(out, "ddx-human-3 Needs human 3 (10 downstream)"))
}

func TestWorkWatchStdout_PrintsNextReadyTransitionAfterIdle(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	ctx, cancel := context.WithCancel(context.Background())
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			cancel()
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-after-idle",
				ResultRev: "feedface",
			}, nil
		}),
		Now: fixedWatchStdoutTime,
	}

	wakeCh := make(chan struct{}, 1)
	progressCh := make(chan ProgressEvent, 16)

	var logBuf bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Run the worker in a goroutine so the main goroutine can process progress
	// events synchronously, eliminating goroutine-scheduling races under load.
	type workerResult struct {
		result *ExecuteBeadLoopResult
		err    error
	}
	workerDone := make(chan workerResult, 1)
	go func() {
		r, e := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
			Mode:         executeloop.ModeWatch,
			IdleInterval: time.Hour,
			Log:          &logBuf,
			ProgressCh:   progressCh,
			WakeCh:       wakeCh,
		})
		workerDone <- workerResult{r, e}
	}()

	// Deterministic: on loop.idle, create the bead and send the wake signal
	// immediately from the main goroutine with no scheduling gap.
	var activeEvents int32
	var beadCreateErr error
	var wr workerResult
	idleHandled := false
	finished := false
	for !finished {
		select {
		case evt := <-progressCh:
			switch evt.Phase {
			case "loop.idle":
				if !idleHandled {
					idleHandled = true
					beadCreateErr = store.Create(&bead.Bead{
						ID:       "ddx-ready-after-idle",
						Title:    "spec: define full DDx temp cleanup in work cycle",
						Priority: 0,
					})
					wakeCh <- struct{}{}
				}
			case "loop.active":
				atomic.AddInt32(&activeEvents, 1)
			}
		case wr = <-workerDone:
			finished = true
		}
	}
	// Drain events emitted just before the worker signalled done.
drainLoop:
	for {
		select {
		case evt := <-progressCh:
			if evt.Phase == "loop.active" {
				atomic.AddInt32(&activeEvents, 1)
			}
		default:
			break drainLoop
		}
	}

	require.NoError(t, beadCreateErr)
	require.ErrorIs(t, wr.err, context.Canceled)
	require.NotNil(t, wr.result)
	assert.Equal(t, int32(1), atomic.LoadInt32(&activeEvents), "watch mode must still emit loop.active progress")

	out := logBuf.String()
	transition := "taking next ready bead from queue: ddx-ready-after-idle — spec: define full DDx temp cleanup in work cycle"
	header := "▶ ddx-ready-after-idle: spec: define full DDx temp cleanup in work cycle"
	assert.Contains(t, out, "\n12:34:56 "+transition+"\n")
	assert.Contains(t, out, header)
	assert.Less(t, strings.Index(out, transition), strings.Index(out, header))
}

// TestDrain_RoutingPreflightRunsOnce covers the bootstrap behavior required
// by ddx-848069a3: routing preflight runs once before the drain loop starts
// and does not repeat as additional beads are processed.
func TestDrain_RoutingPreflightRunsOnce(t *testing.T) {
	inner, first, second := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var executed []string
	var preflightCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-after-preflight",
				ResultRev: "deadbeef",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "claude", Model: "gpt-5"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
		RoutePreflight: func(ctx context.Context, harness, model string) error {
			atomic.AddInt32(&preflightCalls, 1)
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&preflightCalls), "preflight must run once at startup")
	assert.Equal(t, int32(2), atomic.LoadInt32(&store.claimCalls), "both ready beads must still be claimed")
	require.Len(t, executed, 2, "both ready beads must execute")
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 2, result.Successes)

	gotFirst, err := inner.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotFirst.Status)
	gotSecond, err := inner.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotSecond.Status)
}

// TestLoop_OnceFlagStillExits covers ddx-dc157075 back-compat: --once must
// still terminate the loop after exactly one ready bead is processed, even
// when more beads are queue-ready. This guards against an over-eager fix
// that drops the Once gate.
func TestLoop_OnceFlagStillExits(t *testing.T) {
	store, _, _ := newExecuteLoopTestStore(t)

	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-once",
				ResultRev: "cafebabe",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Even in watch-capable runtimes, once mode must still cause the loop to
	// exit after one bead.
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeOnce,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, executed, 1, "--once must process exactly one bead")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
}

// TestLoop_ExplicitDrainExits covers drain-and-exit semantics: an empty queue
// is terminal unless watch mode is selected.
func TestLoop_ExplicitDrainExits(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run when queue is empty")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Drain mode exits when the queue is empty rather than idling. A
	// bounded-time context guards against a regression that would hang.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	elapsed := time.Since(start)

	require.NoError(t, err, "drain mode must return cleanly without timing out")
	require.NotNil(t, result)
	assert.True(t, elapsed < time.Second,
		"explicit poll=0 must exit promptly when queue is empty; took %s", elapsed)
	assert.True(t, result.NoReadyWork)
	assert.Equal(t, 0, result.Attempts)
}

func fixedWatchStdoutTime() time.Time {
	return time.Date(2026, 5, 9, 12, 34, 56, 789000000, time.UTC)
}

func seedWatchIdleQueue(t *testing.T, store *bead.Store) {
	t.Helper()

	upstream := &bead.Bead{ID: "ddx-upstream", Title: "External blocker", Priority: 0}
	require.NoError(t, store.Create(upstream))
	require.NoError(t, store.UpdateWithLifecycleStatus(upstream.ID, bead.StatusBlocked, bead.LifecycleTransitionOptions{
		ExternalBlockerReason: "waiting for upstream",
		Reason:                "test fixture",
		Source:                "test",
	}, nil))

	for i := 1; i <= 3; i++ {
		blocker := &bead.Bead{
			ID:       fmt.Sprintf("ddx-human-%d", i),
			Title:    fmt.Sprintf("Needs human %d", i),
			Priority: i,
		}
		blocker.AddDep(upstream.ID, "blocks")
		require.NoError(t, store.Create(blocker))
		require.NoError(t, store.UpdateWithLifecycleStatus(blocker.ID, bead.StatusProposed, bead.LifecycleTransitionOptions{
			OperatorRequired: true,
			Reason:           "test fixture: operator attention required",
			Source:           "test",
		}, nil))

		prevID := blocker.ID
		for n := 1; n <= 10; n++ {
			downstream := &bead.Bead{
				ID:       fmt.Sprintf("ddx-down-%d-%02d", i, n),
				Title:    fmt.Sprintf("Downstream %d %02d", i, n),
				Priority: 4,
			}
			downstream.AddDep(prevID, "blocks")
			require.NoError(t, store.Create(downstream))
			prevID = downstream.ID
		}
	}

	require.NoError(t, store.Create(&bead.Bead{
		ID:       "ddx-proposed",
		Title:    "Proposed operator attention",
		Status:   bead.StatusProposed,
		Priority: 3,
	}))
	require.NoError(t, store.Create(&bead.Bead{
		ID:       "ddx-not-eligible",
		Title:    "Execution ineligible",
		Priority: 4,
		Extra:    map[string]any{bead.ExtraExecutionElig: false},
	}))
	require.NoError(t, store.Create(&bead.Bead{
		ID:       "ddx-superseded",
		Title:    "Superseded",
		Priority: 4,
		Extra:    map[string]any{"superseded-by": "ddx-replacement"},
	}))

	cooldown := &bead.Bead{ID: "ddx-cooldown", Title: "Retry later", Priority: 4}
	require.NoError(t, store.Create(cooldown))
	// Use time.Now().Add so the cooldown is always in the future regardless of when
	// the test runs relative to fixedWatchStdoutTime (which is a fixed past date).
	require.NoError(t, store.SetExecutionCooldown(cooldown.ID, time.Now().Add(24*time.Hour), ExecuteBeadStatusNoChanges, "retry later", ""))

	ordinaryEpic := &bead.Bead{ID: "ddx-epic-open", Title: "Open epic", IssueType: "epic", Priority: 4}
	ordinaryEpicChild := &bead.Bead{ID: "ddx-epic-open-child", Title: "Open epic child", Parent: ordinaryEpic.ID, Priority: 4}
	ordinaryEpicChild.AddDep(upstream.ID, "blocks")
	require.NoError(t, store.Create(ordinaryEpic))
	require.NoError(t, store.Create(ordinaryEpicChild))

	closureEpic := &bead.Bead{ID: "ddx-epic-closure", Title: "Closure epic", IssueType: "epic", Priority: 4}
	closedChild := &bead.Bead{
		ID:       "ddx-epic-closed-child",
		Title:    "Closed epic child",
		Status:   bead.StatusClosed,
		Parent:   closureEpic.ID,
		Priority: 4,
	}
	require.NoError(t, store.Create(closureEpic))
	require.NoError(t, store.Create(closedChild))
}

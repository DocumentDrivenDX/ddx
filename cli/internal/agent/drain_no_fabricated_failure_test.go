package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	drainReplayNormalBeadID       = "ddx-int-0001"
	drainReplayQuotaBeadID        = "ddx-int-0002"
	drainReplayCanceledBeadID     = "ddx-int-0003"
	drainReplayDisagreeBeadID     = "ddx-int-0004"
	drainReplayLongToolBeadID     = "ddx-int-0005"
	drainReplayNormalOutputFile   = "drain-normal.txt"
	drainReplayQuotaOutputFile    = "drain-quota.txt"
	drainReplayCanceledOutputFile = "drain-canceled.txt"
	drainReplayDisagreeOutputFile = "drain-disagree.txt"
	drainReplayLongOutputFile     = "drain-long-tool.txt"
)

const drainReplayReviewApproveJSON = `{"schema_version":1,"verdict":"APPROVE","summary":"looks good","per_ac":[{"number":1,"item":"AC one","grade":"pass","evidence":"reviewed"}]}`

const drainReplayReviewDisagreeJSON = `{"schema_version":1,"verdict":"APPROVE","summary":"looks good","per_ac":[{"number":1,"item":"AC one","grade":"pass","evidence":"reviewed"}]}`

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

type lockedBeadReviewer struct {
	mu      *sync.Mutex
	inner   *DefaultBeadReviewer
	cleanup func()
}

func (r *lockedBeadReviewer) ReviewBead(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res, err := r.inner.ReviewBead(ctx, beadID, resultRev, impl)
	if r.cleanup != nil {
		r.cleanup()
	}
	return res, err
}

func (r *lockedBeadReviewer) ReviewGroup(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewGroupResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res, err := r.inner.ReviewGroup(ctx, beadID, resultRev, impl)
	if r.cleanup != nil {
		r.cleanup()
	}
	return res, err
}

func scriptHarnessExecutorWithTempSessions(t *testing.T, projectRoot, directivePath string) ExecuteBeadExecutorFunc {
	t.Helper()
	runner := NewRunner(Config{})
	gitOps := &RealGitOps{}
	orchGitOps := &RealOrchestratorGitOps{}
	repoMu := landMutexFor(projectRoot)
	cleanupLogs := func() {
		_ = os.RemoveAll(filepath.Join(projectRoot, ddxroot.DirName, "agent-logs"))
	}

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Model: directivePath,
	})
	cfg.Agent.SessionLogDir = filepath.Join(t.TempDir(), "sessions")
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "script"})

	return ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		repoMu.Lock()
		defer repoMu.Unlock()

		res, err := ExecuteBeadWithConfig(ctx, projectRoot, beadID, rcfg, ExecuteBeadRuntime{
			AgentRunner: runner,
		}, gitOps)
		if err != nil {
			if res != nil {
				report := executeBeadResultToReport(res)
				cleanupLogs()
				return report, nil
			}
			report := ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: err.Error(),
			}
			cleanupLogs()
			return report, nil
		}

		landFromRev := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
		landing, landErr := LandBeadResult(projectRoot, res, orchGitOps, BeadLandingOptions{
			LandingAdvancer: func(r *ExecuteBeadResult) (*LandResult, error) {
				req := BuildLandRequestFromResult(projectRoot, r)
				return Land(projectRoot, req, RealLandingGitOps{})
			},
		})
		if landErr != nil {
			report := ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: landErr.Error(),
			}
			cleanupLogs()
			return report, nil
		}
		ApplyLandingToResult(res, landing)
		if syncErr := (RealLandingGitOps{}).SyncWorkTreeToHead(projectRoot, landFromRev); syncErr != nil {
			report := ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: syncErr.Error(),
			}
			cleanupLogs()
			return report, nil
		}
		report := executeBeadResultToReport(res)
		cleanupLogs()
		return report, nil
	})
}

type drainReplayReviewService struct {
	*passthroughTestService
}

func newDrainReplayReviewService() *drainReplayReviewService {
	return &drainReplayReviewService{
		passthroughTestService: &passthroughTestService{
			harnessInfos: []agentlib.HarnessInfo{{Name: "claude", Available: true}},
			listPolicies: []agentlib.PolicyInfo{
				{Name: "review-strong", MinPower: 70, MaxPower: 90},
				{Name: "review-fallback", MinPower: 10, MaxPower: 20},
			},
			listModels: []agentlib.ModelInfo{
				{ID: "review-model", Power: 90, Available: true, AutoRoutable: true},
				{ID: "review-lite", Power: 20, Available: true, AutoRoutable: true},
			},
		},
	}
}

func (s *drainReplayReviewService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	beadID := req.CorrelationID
	if parts := strings.Split(req.CorrelationID, ":"); len(parts) > 0 && parts[0] != "" {
		beadID = parts[0]
	}

	switch beadID {
	case drainReplayQuotaBeadID:
		events, err := s.successEvents(req, "quota-unknown", 35*time.Millisecond, drainReplayReviewApproveJSON, false)
		return events, err
	case drainReplayCanceledBeadID:
		events, err := s.successEvents(req, "context-canceled", 20*time.Millisecond, drainReplayReviewApproveJSON, false)
		return events, err
	case drainReplayDisagreeBeadID:
		events, err := s.successEvents(req, "review-disagreement", 20*time.Millisecond, drainReplayReviewDisagreeJSON, true)
		return events, err
	case drainReplayLongToolBeadID:
		events, err := s.successEvents(req, "long-tool-call", 80*time.Millisecond, drainReplayReviewApproveJSON, false)
		return events, err
	default:
		events, err := s.successEvents(req, "normal-route", 10*time.Millisecond, drainReplayReviewApproveJSON, false)
		return events, err
	}
}

func (s *drainReplayReviewService) successEvents(req agentlib.ServiceExecuteRequest, reason string, delay time.Duration, finalText string, includeToolProgress bool) (<-chan agentlib.ServiceEvent, error) {
	time.Sleep(delay)

	payload, err := json.Marshal(map[string]any{
		"harness":  "fiz",
		"provider": "anthropic",
		"model":    "claude-opus-4-7",
		"reason":   reason,
	})
	if err != nil {
		return nil, err
	}

	final, err := json.Marshal(map[string]any{
		"status":     "success",
		"exit_code":  0,
		"final_text": finalText,
	})
	if err != nil {
		return nil, err
	}

	events := make([]agentlib.ServiceEvent, 0, 4)
	events = append(events, agentlib.ServiceEvent{
		Type: "routing_decision",
		Time: time.Now().UTC(),
		Data: payload,
	})
	if includeToolProgress {
		progress, err := json.Marshal(map[string]any{
			"phase":           "tool",
			"state":           "complete",
			"task_id":         req.CorrelationID,
			"turn_index":      1,
			"tool_name":       "Bash",
			"action":          "sleep then continue",
			"subject":         "review replay",
			"output_bytes":    42,
			"output_lines":    3,
			"duration_ms":     delay.Milliseconds(),
			"session_summary": "long tool call that stays under the watchdog budget",
		})
		if err != nil {
			return nil, err
		}
		events = append(events, agentlib.ServiceEvent{
			Type: "progress",
			Time: time.Now().UTC(),
			Data: progress,
		})
	}
	events = append(events, agentlib.ServiceEvent{
		Type: "final",
		Time: time.Now().UTC(),
		Data: final,
	})

	ch := make(chan agentlib.ServiceEvent, len(events))
	for _, evt := range events {
		ch <- evt
	}
	close(ch)
	return ch, nil
}

func TestDrain_NoFabricatedFailureSignatures(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 5)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	dirTmp := t.TempDir()
	writeDirectiveFile(t, filepath.Join(dirTmp, drainReplayNormalBeadID+".txt"), []string{
		"create-file " + drainReplayNormalOutputFile + " normal route-decision scenario",
		"commit chore: drain replay normal scenario",
	})
	writeDirectiveFile(t, filepath.Join(dirTmp, drainReplayQuotaBeadID+".txt"), []string{
		"sleep-ms 75",
		"create-file " + drainReplayQuotaOutputFile + " quota unknown scenario",
		"commit chore: drain replay quota scenario",
	})
	writeDirectiveFile(t, filepath.Join(dirTmp, drainReplayCanceledBeadID+".txt"), []string{
		"sleep-ms 125",
		"create-file " + drainReplayCanceledOutputFile + " infra timeout scenario",
		"commit chore: drain replay canceled scenario",
	})
	writeDirectiveFile(t, filepath.Join(dirTmp, drainReplayDisagreeBeadID+".txt"), []string{
		fmt.Sprintf(
			"run mkdir -p .ddx/executions/$DDX_ATTEMPT_ID && printf '%%s' '%s' > .ddx/executions/$DDX_ATTEMPT_ID/ac-check.json",
			escapeForShell(`{"items":[{"ac":1,"result":"fail","evidence":"mechanical mismatch"}]}`),
		),
		"create-file " + drainReplayDisagreeOutputFile + " review disagreement scenario",
		"commit chore: drain replay disagreement scenario",
	})
	writeDirectiveFile(t, filepath.Join(dirTmp, drainReplayLongToolBeadID+".txt"), []string{
		"sleep-ms 150",
		"create-file " + drainReplayLongOutputFile + " long tool call scenario",
		"commit chore: drain replay long tool scenario",
	})

	store := makeLoopStore(t, ddxDir)
	require.NoError(t, store.AppendEvent(drainReplayDisagreeBeadID, bead.BeadEvent{
		Kind:      "review-error",
		Summary:   evidence.OutcomeReviewReviewerUnavailable,
		Body:      ReviewErrorEventBody(evidence.OutcomeReviewReviewerUnavailable, 1, "seeded-review-retry", "prior routing failure"),
		Actor:     "integration-worker",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	}))
	require.NoError(t, store.AppendEvent(drainReplayDisagreeBeadID, bead.BeadEvent{
		Kind:      ReviewACOverrideEventKind,
		Summary:   "1 AC grade(s) diverge from ac-check.json (reviewer_index=1)",
		Body:      "ac1: ac_check=fail reviewer=pass",
		Actor:     "integration-worker",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	}))

	beadExecutors := map[string]ExecuteBeadExecutorFunc{
		drainReplayNormalBeadID:   scriptHarnessExecutorWithTempSessions(t, projectRoot, filepath.Join(dirTmp, drainReplayNormalBeadID+".txt")),
		drainReplayQuotaBeadID:    scriptHarnessExecutorWithTempSessions(t, projectRoot, filepath.Join(dirTmp, drainReplayQuotaBeadID+".txt")),
		drainReplayCanceledBeadID: scriptHarnessExecutorWithTempSessions(t, projectRoot, filepath.Join(dirTmp, drainReplayCanceledBeadID+".txt")),
		drainReplayDisagreeBeadID: scriptHarnessExecutorWithTempSessions(t, projectRoot, filepath.Join(dirTmp, drainReplayDisagreeBeadID+".txt")),
		drainReplayLongToolBeadID: scriptHarnessExecutorWithTempSessions(t, projectRoot, filepath.Join(dirTmp, drainReplayLongToolBeadID+".txt")),
	}
	cleanupProjectRootLogs := func() {
		_ = os.RemoveAll(filepath.Join(projectRoot, ddxroot.DirName, "agent-logs"))
	}

	dispatchExec := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		execFn, ok := beadExecutors[beadID]
		if !ok {
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusExecutionFailed, Detail: "no executor configured"}, nil
		}
		return execFn(ctx, beadID)
	})

	reviewService := newDrainReplayReviewService()
	reviewLock := landMutexFor(projectRoot)
	reviewer := &lockedBeadReviewer{
		mu: reviewLock,
		inner: &DefaultBeadReviewer{
			ProjectRoot: projectRoot,
			BeadStore:   store,
			Service:     reviewService,
			Harness:     "claude",
			BeadEvents:  store,
			EventReader: store,
		},
		cleanup: cleanupProjectRootLogs,
	}

	var logBuf lockedBuffer
	cfgOpts := config.TestLoopConfigOpts{
		Assignee:         "integration-worker",
		ReviewMaxRetries: 2,
		Harness:          "script",
		Model:            "script-model",
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	var wg sync.WaitGroup
	results := make([]*ExecuteBeadLoopResult, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := &ExecuteBeadWorker{
				Store:    store,
				Executor: dispatchExec,
				Reviewer: reviewer,
			}
			results[i], errs[i] = worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
				Mode:            executeloop.ModeDrain,
				Log:             &logBuf,
				PostMergeReview: true,
				RoutePreflight: func(ctx context.Context, harness, model string) error {
					time.Sleep(25 * time.Millisecond)
					return nil
				},
			})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "worker %d returned an error", i)
		require.NotNilf(t, results[i], "worker %d returned a nil result", i)
	}

	for _, beadID := range []string{
		drainReplayNormalBeadID,
		drainReplayQuotaBeadID,
		drainReplayCanceledBeadID,
		drainReplayDisagreeBeadID,
		drainReplayLongToolBeadID,
	} {
		got, err := store.Get(context.Background(), beadID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, got.Status, "bead %s should merge", beadID)
		assert.NotEmpty(t, got.Extra["closing_commit_sha"], "bead %s should record a closing commit SHA", beadID)
	}

	var foundReviewACOverride bool
	var foundReviewerUnavailable bool
	for _, beadID := range []string{
		drainReplayNormalBeadID,
		drainReplayQuotaBeadID,
		drainReplayCanceledBeadID,
		drainReplayDisagreeBeadID,
		drainReplayLongToolBeadID,
	} {
		events, err := store.Events(beadID)
		require.NoError(t, err)
		for _, ev := range events {
			assert.NotContains(t, ev.Body, "no viable routing candidate", "forbidden fabricated routing signature must not leak into bead events")
			assert.NotContains(t, ev.Body, "progress_watchdog", "forbidden watchdog signature must not leak into bead events")
			assert.NotContains(t, ev.Body, "consecutive_wedge", "forbidden wedge signature must not leak into bead events")
			assert.NotContains(t, ev.Body, "production-reachability", "forbidden production-reachability signature must not leak into bead events")
			if ev.Kind == ReviewACOverrideEventKind {
				foundReviewACOverride = true
			}
			if ev.Kind == "review-error" && strings.Contains(ev.Body, evidence.OutcomeReviewReviewerUnavailable) {
				foundReviewerUnavailable = true
			}
		}
	}

	assert.True(t, foundReviewACOverride, "disagreement replay should surface review-ac-override telemetry")
	assert.True(t, foundReviewerUnavailable, "reviewer-unavailable replay should surface a review-error event")

	logOut := logBuf.String()
	assert.NotContains(t, logOut, "no viable routing candidate", "forbidden fabricated routing signature must not leak into worker output")
	assert.NotContains(t, logOut, "progress_watchdog", "forbidden watchdog signature must not leak into worker output")
	assert.NotContains(t, logOut, "consecutive_wedge", "forbidden wedge signature must not leak into worker output")
	assert.NotContains(t, logOut, "production-reachability", "forbidden production-reachability signature must not leak into worker output")

	assert.GreaterOrEqual(t, countClosedBeads(t, ddxDir), 5, "all fixture beads should be closed by the end of the drain")
}

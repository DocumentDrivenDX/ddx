package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reviewTimeoutService struct {
	*passthroughTestService
	workDir     string
	spawnClaude bool
	childPID    chan int
}

func newReviewTimeoutService(workDir string, spawnClaude bool) *reviewTimeoutService {
	return &reviewTimeoutService{
		passthroughTestService: &passthroughTestService{
			harnessInfos: []agentlib.HarnessInfo{{Name: "claude", Available: true}},
			listPolicies: []agentlib.PolicyInfo{{Name: "review", MinPower: 10, MaxPower: 100}},
			listModels:   []agentlib.ModelInfo{{ID: "claude-review", Power: 80, Available: true, AutoRoutable: true}},
		},
		workDir:     workDir,
		spawnClaude: spawnClaude,
		childPID:    make(chan int, 1),
	}
}

func (s *reviewTimeoutService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.executeCalled = true
	s.lastReq = req
	if s.spawnClaude {
		s.childPID <- startScopedProviderChild(req.WorkDir, "claude")
	}
	ch := make(chan agentlib.ServiceEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func TestReviewDispatchTimeoutEmitsOperatorAttention(t *testing.T) {
	projectRoot, head, store := newReviewArtifactsFixture(t)
	svc := newReviewTimeoutService(projectRoot, false)
	reviewer := &DefaultBeadReviewer{
		ProjectRoot:           projectRoot,
		BeadStore:             store,
		Service:               svc,
		ReviewDispatchTimeout: 120 * time.Millisecond,
	}
	b, err := store.Get(context.Background(), "ddx-review-happy")
	require.NoError(t, err)

	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Store:    store,
		Bead:     *b,
		Reviewer: reviewer,
		Rcfg: config.NewTestConfigForLoop(config.TestLoopConfigOpts{}).Resolve(
			config.TestLoopOverrides(config.TestLoopConfigOpts{}),
		),
		Report: ExecuteBeadReport{
			BeadID:      b.ID,
			AttemptID:   "impl-attempt-review-timeout",
			Status:      ExecuteBeadStatusSuccess,
			ResultRev:   head,
			Harness:     "claude",
			Model:       "claude-sonnet",
			ActualPower: 60,
		},
	})

	require.False(t, out.Approved)
	require.NotNil(t, out.Report)
	assert.Equal(t, ExecuteBeadStatusReviewMalfunction, out.Report.Status)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var reviewErr *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "review-error" {
			reviewErr = &events[i]
		}
	}
	require.NotNil(t, reviewErr, "timeout must emit operator-visible review-error evidence")
	assert.Equal(t, evidence.OutcomeReviewTransport, reviewErr.Summary)
	assert.Contains(t, reviewErr.Body, "failure_class="+evidence.OutcomeReviewTransport)
	assert.Contains(t, reviewErr.Body, "result_rev="+head)
	assert.Contains(t, reviewErr.Body, "review_attempt_id=")
	assert.Contains(t, reviewErr.Body, "elapsed_ms=")
	assert.Contains(t, reviewErr.Body, "cleanup_result=no_children")
}

func TestReviewDispatchTimeoutKillsProcessTree(t *testing.T) {
	projectRoot, head, store := newReviewArtifactsFixture(t)
	svc := newReviewTimeoutService(projectRoot, true)
	reviewer := &DefaultBeadReviewer{
		ProjectRoot:           projectRoot,
		BeadStore:             store,
		Service:               svc,
		ReviewDispatchTimeout: 120 * time.Millisecond,
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-review-happy", head, ImplementerRouting{
		Harness: "claude",
		Model:   "claude-sonnet",
		Correlation: map[string]string{
			"bead_id":    "ddx-review-happy",
			"attempt_id": "impl-attempt-process-tree",
			"result_rev": head,
		},
	})
	require.Error(t, err)
	require.NotNil(t, res)

	var pid int
	select {
	case pid = <-svc.childPID:
	case <-time.After(2 * time.Second):
		t.Fatal("fake claude child was not started")
	}
	assertProcessGone(t, pid)

	timeoutPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), requestTimeoutArtifact)
	raw, err := os.ReadFile(timeoutPath)
	require.NoError(t, err, "review timeout cleanup evidence must be written")
	var ev requestTimeoutEvidence
	require.NoError(t, json.Unmarshal(raw, &ev))
	assert.Equal(t, reasonRequestTimeout, ev.Reason)
	assert.Equal(t, "reviewer", ev.Phase)
	assert.Equal(t, requestTimeoutCleanupReaped, ev.CleanupResult)
	require.NotEmpty(t, ev.Reaped)
	assert.Equal(t, "claude", ev.Reaped[0].Provider)
}

func TestReviewDispatchSuccessPreservesExistingBehavior(t *testing.T) {
	projectRoot, head, store := newReviewArtifactsFixture(t)
	final, err := json.Marshal(map[string]any{
		"status":     "success",
		"exit_code":  0,
		"final_text": `{"schema_version":1,"verdict":"APPROVE","summary":"ok","per_ac":[{"number":1,"item":"AC one","grade":"pass","evidence":"reviewed"}]}`,
	})
	require.NoError(t, err)
	svc := &passthroughTestService{
		harnessInfos: []agentlib.HarnessInfo{{Name: "claude", Available: true}},
		listPolicies: []agentlib.PolicyInfo{{Name: "review", MinPower: 10, MaxPower: 100}},
		listModels:   []agentlib.ModelInfo{{ID: "claude-review", Power: 80, Available: true, AutoRoutable: true}},
		executeEvents: []agentlib.ServiceEvent{{
			Type: "final",
			Data: final,
		}},
	}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Service:     svc,
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-review-happy", head, ImplementerRouting{
		Harness: "claude",
		Model:   "claude-sonnet",
		Correlation: map[string]string{
			"bead_id":    "ddx-review-happy",
			"attempt_id": "impl-attempt-success",
			"result_rev": head,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, VerdictApprove, res.Verdict)
	assert.Equal(t, "ok", res.Rationale)
	assert.Equal(t, DefaultReviewDispatchTimeout, svc.lastReq.ProviderTimeout)
	assert.Contains(t, svc.lastReq.Metadata, "impl_attempt_id")
	assert.NotContains(t, svc.lastReq.Metadata, "DDX_AGENT_NAME")
}

func startScopedProviderChild(workDir, provider string) int {
	binDir, err := os.MkdirTemp("", "ddx-review-provider-*")
	if err != nil {
		panic(err)
	}
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		panic(err)
	}
	bin := filepath.Join(binDir, provider)
	if err := os.Symlink(sleepPath, bin); err != nil {
		panic(err)
	}
	cmd := exec.Command(bin, "120")
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	go func() {
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(binDir)
	}()
	return cmd.Process.Pid
}

func TestReviewDispatchTimeoutDetailReadsCleanupEvidence(t *testing.T) {
	projectRoot := t.TempDir()
	attemptID := "review-timeout-detail"
	dir := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, writeArtifactJSON(filepath.Join(dir, requestTimeoutArtifact), requestTimeoutEvidence{
		AttemptID:         attemptID,
		Reason:            reasonRequestTimeout,
		Phase:             "reviewer",
		ConfiguredTimeout: "120ms",
		Elapsed:           "121ms",
		CleanupResult:     requestTimeoutCleanupReaped,
		ProviderPID:       123,
	}))

	detail := reviewRequestTimeoutDetail(projectRoot, attemptID)
	assert.Contains(t, detail, "review_attempt_id="+attemptID)
	assert.Contains(t, detail, "cleanup_result="+requestTimeoutCleanupReaped)
	assert.Contains(t, detail, "provider_pid=123")
	assert.False(t, strings.Contains(detail, "\x00"))
}

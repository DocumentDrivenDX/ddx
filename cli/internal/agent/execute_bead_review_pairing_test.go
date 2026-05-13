package agent

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildReviewExecuteRequest verifies that the helper produces the runtime
// fields demanded by R4 pairing: Role=reviewer in the correlation map, the
// implementer's correlation keys are propagated, and MinPower is bumped one
// above the implementer's actual selected power.
func TestBuildReviewExecuteRequest(t *testing.T) {
	impl := ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai",
		Model:       "gpt-5",
		ActualPower: 70,
		Correlation: map[string]string{
			"bead_id":    "ddx-pairing-1",
			"attempt_id": "att-1",
			"session_id": "sess-1",
		},
	}
	got := BuildReviewExecuteRequest(impl, "claude", "review-strong")

	assert.Equal(t, "claude", got.HarnessOverride)
	assert.Empty(t, got.ModelOverride)
	assert.Equal(t, "review-strong", got.ProfileOverride)
	assert.True(t, got.ClearRoutingPins)
	assert.True(t, got.ClearProfile)
	assert.True(t, got.ClearMaxPower)
	assert.Equal(t, 71, got.MinPowerOverride, "MinPower must be impl.ActualPower+1 (R4 pairing)")
	assert.Equal(t, "reviewer", got.Role, "Role=reviewer must be set so the dispatch request is tagged correctly")
	assert.Equal(t, "ddx-pairing-1", got.CorrelationID, "single-review correlation ID should be derived from bead_id when no review_group_id is present")

	require.NotNil(t, got.Correlation)
	assert.Equal(t, "reviewer", got.Correlation["role"], "Role=reviewer must be set so events join correctly")
	assert.Equal(t, "ddx-pairing-1", got.Correlation["bead_id"], "implementer correlation keys must propagate")
	assert.Equal(t, "att-1", got.Correlation["attempt_id"])
	assert.Equal(t, "sess-1", got.Correlation["session_id"])
	assert.Equal(t, "codex", got.Correlation["impl_harness"])
	assert.Equal(t, "openai", got.Correlation["impl_provider"])
	assert.Equal(t, "gpt-5", got.Correlation["impl_model"])
	assert.Equal(t, "70", got.Correlation["impl_actual_power"])
}

func TestReviewerDispatch_UsesProfilePin(t *testing.T) {
	got := BuildReviewExecuteRequest(ImplementerRouting{Harness: "claude"}, "", "review-strong")
	assert.Empty(t, got.HarnessOverride)
	assert.Empty(t, got.ModelOverride)
	assert.Equal(t, "review-strong", got.ProfileOverride)
}

// TestBuildReviewExecuteRequest_ZeroPowerLeavesMinPowerZero verifies the
// edge case where the implementer's actual power was not reported (e.g., a
// virtual harness or a routing decision missing components.power). MinPower
// should remain zero so the reviewer falls back to rcfg.MinPower() rather
// than pinning to a synthetic +1 above zero.
func TestBuildReviewExecuteRequest_ZeroPowerLeavesMinPowerZero(t *testing.T) {
	got := BuildReviewExecuteRequest(ImplementerRouting{}, "claude", "review-strong")
	assert.Zero(t, got.MinPowerOverride)
}

// reviewPairingTestSetup creates a project root with a git repo, a bead store
// containing one bead, an initial commit, and returns the resolved HEAD. The
// reviewer dispatch reads `git show HEAD` for the diff section so a real
// commit is required.
func reviewPairingTestSetup(t *testing.T) (projectRoot, head string, store *bead.Store) {
	t.Helper()
	projectRoot = t.TempDir()
	out, err := exec.Command("git", "init", projectRoot).CombinedOutput()
	require.NoError(t, err, string(out))
	store = bead.NewStore(filepath.Join(projectRoot, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:         "ddx-pairing",
		Title:      "Pairing test",
		Acceptance: "1. AC one",
	}))
	out, err = exec.Command("git", "-C", projectRoot,
		"-c", "user.name=Test", "-c", "user.email=t@example.com",
		"commit", "--allow-empty", "-m", "init").CombinedOutput()
	require.NoError(t, err, string(out))
	rawHead, err := exec.Command("git", "-C", projectRoot, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	head = strings.TrimSpace(string(rawHead))
	return projectRoot, head, store
}

// reviewerOutputApprove is a canned reviewer JSON verdict used by the pairing
// tests. The strict parser (ParseReviewVerdict) requires a single JSON object
// inside a ```json``` fence; anything else returns review-error: unparseable.
const reviewerOutputApprove = "```json\n{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\",\"per_ac\":[{\"number\":1,\"item\":\"AC one\",\"grade\":\"pass\",\"evidence\":\"reviewed cli/internal/agent\"}]}\n```"

// TestReviewBead_HappyPath_DifferentProvider_NoDegradedEvent verifies that
// when the reviewer's resolved provider differs from the implementer's, no
// kind:review-pairing-degraded event is appended.
func TestReviewBead_HappyPath_DifferentProvider_NoDegradedEvent(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	events := &stubBeadEventAppender{}

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		BeadEvents:  events,
		Harness:     "claude",
		Runner: &reviewRunnerStub{result: &Result{
			Harness:     "claude",
			Provider:    "anthropic", // different from implementer
			Model:       "claude-opus-4-7",
			ActualPower: 95,
			Output:      reviewerOutputApprove,
		}},
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai", // different from reviewer
		Model:       "gpt-5",
		ActualPower: 70,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, VerdictApprove, res.Verdict)
	assert.Equal(t, "anthropic", res.ReviewerProvider)

	for _, ev := range events.events {
		assert.NotEqual(t, ReviewPairingDegradedEventKind, ev.Event.Kind,
			"happy path (different provider) must not emit review-pairing-degraded")
	}
}

// TestReviewBead_DegradedPath_SameProvider_EmitsEvent verifies that when the
// reviewer's resolved provider matches the implementer's, a typed
// kind:review-pairing-degraded event is appended whose body carries both
// implementer and reviewer routing details.
func TestReviewBead_DegradedPath_SameProvider_EmitsEvent(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	events := &stubBeadEventAppender{}

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		BeadEvents:  events,
		Harness:     "claude",
		Runner: &reviewRunnerStub{result: &Result{
			Harness:     "claude",
			Provider:    "anthropic",
			Model:       "claude-opus-4-7",
			ActualPower: 95,
			Output:      reviewerOutputApprove,
		}},
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "claude",
		Provider:    "anthropic", // SAME provider as reviewer — degraded
		Model:       "claude-sonnet-4-6",
		ActualPower: 70,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "anthropic", res.ReviewerProvider)

	var degraded *bead.BeadEvent
	for i := range events.events {
		ev := &events.events[i]
		if ev.Event.Kind == ReviewPairingDegradedEventKind {
			degraded = &ev.Event
			assert.Equal(t, "ddx-pairing", ev.BeadID)
			break
		}
	}
	require.NotNil(t, degraded, "degraded path (same provider) must emit kind:review-pairing-degraded")
	assert.Contains(t, degraded.Summary, "anthropic")

	// Body must include both implementer and reviewer routing details.
	body := degraded.Body
	assert.Contains(t, body, "impl_harness=claude")
	assert.Contains(t, body, "impl_provider=anthropic")
	assert.Contains(t, body, "impl_model=claude-sonnet-4-6")
	assert.Contains(t, body, "impl_actual_power=70")
	assert.Contains(t, body, "reviewer_harness=claude")
	assert.Contains(t, body, "reviewer_provider=anthropic")
	assert.Contains(t, body, "reviewer_model=claude-opus-4-7")
	assert.Contains(t, body, "reviewer_actual_power=95")
	assert.Contains(t, body, "min_power_requested=71")
}

func TestReviewBead_DoesNotInheritImplementerHarness(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)

	stub := &reviewRunnerStub{result: &Result{
		// Empty Harness/Provider in the result so we observe the dispatched default.
		Output: reviewerOutputApprove,
	}}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      stub,
		// Harness intentionally empty: tests the fallback path.
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:  "codex",
		Provider: "openai",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.ReviewerHarness)
}

func TestPostMergeReviewer_DispatchesWithStrongestAboveImplPowerAndNoModelPin(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	svc := &passthroughTestService{
		listPolicies: []agentlib.PolicyInfo{
			{Name: "standard", MinPower: 7, MaxPower: 8},
			{Name: "smart", MinPower: 9, MaxPower: 10},
			{Name: "frontier", MinPower: 71, MaxPower: 80},
		},
		listModels: []agentlib.ModelInfo{
			{ID: "standard-model", Power: 8, Available: true, AutoRoutable: true},
			{ID: "smart-model", Power: 10, Available: true, AutoRoutable: true},
			{ID: "frontier-model", Power: 72, Available: true, AutoRoutable: true},
		},
		executeEvents: []agentlib.ServiceEvent{
			{
				Type: "final",
				Data: []byte(`{"status":"success","final_text":"{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}"}`),
			},
		},
	}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Service:     svc,
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai",
		Model:       "gpt-5",
		ActualPower: 70,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "frontier", svc.lastReq.Policy)
	assert.Equal(t, 71, svc.lastReq.MinPower)
	assert.Empty(t, svc.lastReq.Model)
	assert.Empty(t, svc.lastReq.Harness)
	assert.Empty(t, svc.lastReq.Provider)
}

func TestReviewRouting_MissingActualPowerUsesSmartFloor(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	svc := &passthroughTestService{
		listPolicies: []agentlib.PolicyInfo{
			{Name: "smart", MinPower: 9, MaxPower: 10},
			{Name: "frontier", MinPower: 71, MaxPower: 80},
		},
		listModels: []agentlib.ModelInfo{
			{ID: "smart-model", Power: 10, Available: true, AutoRoutable: true},
			{ID: "frontier-model", Power: 72, Available: true, AutoRoutable: true},
		},
		executeEvents: []agentlib.ServiceEvent{
			{
				Type: "final",
				Data: []byte(`{"status":"success","final_text":"{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}"}`),
			},
		},
	}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Service:     svc,
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:  "codex",
		Provider: "openai",
		Model:    "gpt-5",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "smart", svc.lastReq.Policy)
	assert.Equal(t, 9, svc.lastReq.MinPower)
	assert.Empty(t, svc.lastReq.Model)
}

// TestReviewPairingDegraded_IsTelemetryOnly verifies AC 4: when the reviewer
// resolves to the same provider as the implementer, only a
// kind:review-pairing-degraded event is appended. Harness/model/profile pins
// are not mutated, no downgrade re-dispatch occurs, and the review verdict is
// still returned normally.
func TestReviewPairingDegraded_IsTelemetryOnly(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	events := &stubBeadEventAppender{}

	runner := &reviewRunnerStub{result: &Result{
		Harness:     "claude",
		Provider:    "anthropic", // same as implementer below — degraded pairing
		Model:       "claude-opus-4-7",
		ActualPower: 95,
		Output:      reviewerOutputApprove,
	}}

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		BeadEvents:  events,
		Harness:     "claude",
		Runner:      runner,
	}

	impl := ImplementerRouting{
		Harness:     "claude",
		Provider:    "anthropic", // SAME provider as reviewer → degraded
		Model:       "claude-sonnet-4-6",
		ActualPower: 70,
	}
	res, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, impl)
	require.NoError(t, err)
	require.NotNil(t, res)

	// The reviewer result must be a normal verdict — degraded pairing must not
	// block, error, or change the review outcome.
	assert.Equal(t, VerdictApprove, res.Verdict,
		"review verdict must not be affected by pairing degradation")

	// The runner must have been called exactly once — no downgrade re-dispatch
	// should have changed the Harness or Model in lastOpts beyond what
	// BuildReviewExecuteRequest would set.
	assert.Equal(t, "claude", runner.lastOpts.Harness,
		"dispatch harness must not be downgraded when pairing is degraded")

	// Exactly one review-pairing-degraded event.
	var degradedCount int
	for _, ev := range events.events {
		if ev.Event.Kind == ReviewPairingDegradedEventKind {
			degradedCount++
			// Body must NOT carry any downgrade_* mutation fields.
			assert.NotContains(t, ev.Event.Body, "downgraded_harness=",
				"pairing-degraded event body must not carry downgrade mutation fields")
			assert.NotContains(t, ev.Event.Body, "downgraded_model=",
				"pairing-degraded event body must not carry downgrade mutation fields")
		}
	}
	assert.Equal(t, 1, degradedCount,
		"degraded pairing must emit exactly one review-pairing-degraded event")
}

func TestReviewRouting_KnownActualPowerUsesNextFloor(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	svc := &passthroughTestService{
		listPolicies: []agentlib.PolicyInfo{
			{Name: "smart", MinPower: 9, MaxPower: 10},
			{Name: "frontier", MinPower: 71, MaxPower: 80},
		},
		listModels: []agentlib.ModelInfo{
			{ID: "smart-model", Power: 10, Available: true, AutoRoutable: true},
			{ID: "frontier-model", Power: 72, Available: true, AutoRoutable: true},
		},
		executeEvents: []agentlib.ServiceEvent{
			{
				Type: "final",
				Data: []byte(`{"status":"success","final_text":"{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}"}`),
			},
		},
	}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Service:     svc,
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:     "codex",
		Provider:    "openai",
		Model:       "gpt-5",
		ActualPower: 70,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "frontier", svc.lastReq.Policy)
	assert.Equal(t, 71, svc.lastReq.MinPower)
	assert.Empty(t, svc.lastReq.Model)
}

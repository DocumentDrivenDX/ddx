package agent

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
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
	got := BuildReviewExecuteRequest(impl, "claude", "claude-opus-4-7")

	assert.Equal(t, "claude", got.HarnessOverride)
	assert.Equal(t, "claude-opus-4-7", got.ModelOverride)
	assert.Equal(t, 71, got.MinPowerOverride, "MinPower must be impl.ActualPower+1 (R4 pairing)")

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

// TestBuildReviewExecuteRequest_ZeroPowerLeavesMinPowerZero verifies the
// edge case where the implementer's actual power was not reported (e.g., a
// virtual harness or a routing decision missing components.power). MinPower
// should remain zero so the reviewer falls back to rcfg.MinPower() rather
// than pinning to a synthetic +1 above zero.
func TestBuildReviewExecuteRequest_ZeroPowerLeavesMinPowerZero(t *testing.T) {
	got := BuildReviewExecuteRequest(ImplementerRouting{}, "claude", "claude-opus")
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
const reviewerOutputApprove = "```json\n{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}\n```"

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
		Model:       "claude-opus-4-7",
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
		Model:       "claude-opus-4-7",
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

// TestReviewBead_NoLongerDefaultsToImplementerHarness is the AC1 regression
// guard: the reviewer must NOT fall back to the implementer's harness when
// its own Harness is unset. With Harness="" and impl.Harness="codex", the
// resolved review harness must default to "claude".
func TestReviewBead_NoLongerDefaultsToImplementerHarness(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)

	stub := &reviewRunnerStub{result: &Result{
		// Empty Harness/Provider in the result so we observe the inferred default.
		Output: reviewerOutputApprove,
	}}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      stub,
		// Harness intentionally empty: tests the fallback path.
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, ImplementerRouting{
		Harness:  "codex", // implementer harness — must NOT be used as reviewer fallback
		Provider: "openai",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "claude", res.ReviewerHarness,
		"reviewer must default to claude — implementer harness fallback violates R4 (different reviewer)")
	assert.NotEqual(t, "codex", res.ReviewerHarness)
}

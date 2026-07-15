package agent

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewRequestRaisesMinPowerWithoutSettingHarnessProviderModel proves
// reviewer strength is expressed only through abstract MinPower. Concrete
// fields come exclusively from the immutable operator config at dispatch.
func TestReviewRequestRaisesMinPowerWithoutSettingHarnessProviderModel(t *testing.T) {
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
	got := BuildReviewExecuteRequest(impl)

	assert.Empty(t, got.HarnessOverride)
	assert.Empty(t, got.ProviderOverride)
	assert.Empty(t, got.ModelOverride)
	assert.Empty(t, got.ProfileOverride)
	assert.False(t, got.ClearRoutingPins)
	assert.False(t, got.ClearProfile)
	assert.Equal(t, 71, got.MinPowerOverride, "MinPower must be impl.ActualPower+1 (R4 pairing)")
	assert.Equal(t, "reviewer", got.Role, "Role=reviewer must be set so the dispatch request is tagged correctly")
	assert.Equal(t, "ddx-pairing-1", got.CorrelationID, "single-review correlation ID should be derived from bead_id when no review_group_id is present")

	require.NotNil(t, got.Correlation)
	assert.Equal(t, "reviewer", got.Correlation["role"], "Role=reviewer must be set so events join correctly")
	assert.Equal(t, "ddx-pairing-1", got.Correlation["bead_id"], "implementer correlation keys must propagate")
	assert.Equal(t, "att-1", got.Correlation["attempt_id"])
	assert.Equal(t, "sess-1", got.Correlation["session_id"])
	assert.NotContains(t, got.Correlation, "impl_harness")
	assert.NotContains(t, got.Correlation, "impl_provider")
	assert.NotContains(t, got.Correlation, "impl_model")
	assert.NotContains(t, got.Correlation, "impl_actual_power")
}

func TestReviewerDispatch_DoesNotSetConcreteRoutePins(t *testing.T) {
	got := BuildReviewExecuteRequest(ImplementerRouting{Harness: "claude"})
	assert.Empty(t, got.HarnessOverride)
	assert.Empty(t, got.ModelOverride)
	assert.Empty(t, got.ProfileOverride)
}

// TestBuildReviewExecuteRequest_ZeroPowerLeavesMinPowerZero verifies the
// edge case where the implementer's actual power was not reported (e.g., a
// virtual harness or a routing decision missing components.power). MinPower
// should remain zero so the reviewer falls back to rcfg.MinPower() rather
// than pinning to a synthetic +1 above zero.
func TestBuildReviewExecuteRequest_ZeroPowerLeavesMinPowerZero(t *testing.T) {
	got := BuildReviewExecuteRequest(ImplementerRouting{})
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
	store = bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
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

const legacyReviewPairingDegradedEventKind = "review-pairing-degraded"

// TestReviewBead_HappyPath_DifferentProvider_NoDegradedEvent verifies that
// returned provider identity remains result evidence and does not produce a
// route-comparison control event.
func TestReviewBead_HappyPath_DifferentProvider_NoDegradedEvent(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	events := &stubBeadEventAppender{}

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		BeadEvents:  events,
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
		assert.NotEqual(t, legacyReviewPairingDegradedEventKind, ev.Event.Kind,
			"happy path (different provider) must not emit review-pairing-degraded")
	}
}

// TestReviewBead_SameProviderIdentityIsEvidenceOnly verifies that provider
// equality is retained in the review result but is never classified by DDx.
func TestReviewBead_SameProviderIdentityIsEvidenceOnly(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	events := &stubBeadEventAppender{}

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		BeadEvents:  events,
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
	assert.Equal(t, VerdictApprove, res.Verdict)
	assert.Equal(t, "anthropic", res.ReviewerProvider)

	for _, ev := range events.events {
		assert.NotEqual(t, legacyReviewPairingDegradedEventKind, ev.Event.Kind,
			"same-provider identity must remain evidence, not become DDx control")
	}
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

func TestPostMergeReviewer_DispatchesWithStrongerMinPowerAndNoConcretePin(t *testing.T) {
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
	assert.Empty(t, svc.lastReq.Policy)
	assert.Equal(t, 71, svc.lastReq.MinPower)
	assert.Empty(t, svc.lastReq.Model)
	assert.Empty(t, svc.lastReq.Harness)
	assert.Empty(t, svc.lastReq.Provider)
	assert.NotContains(t, svc.lastReq.Metadata, "impl_harness")
	assert.NotContains(t, svc.lastReq.Metadata, "impl_provider")
	assert.NotContains(t, svc.lastReq.Metadata, "impl_model")
	assert.NotContains(t, svc.lastReq.Metadata, "impl_actual_power")
	assert.False(t, svc.listPoliciesCalled)
	assert.False(t, svc.listModelsCalled)
}

func TestReviewRequestOmitsImplementerConcreteRoute(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	newService := func() *passthroughTestService {
		return &passthroughTestService{executeEvents: []agentlib.ServiceEvent{{
			Type: "final",
			Data: []byte(`{"status":"success","final_text":"{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}"}`),
		}}}
	}

	run := func(impl ImplementerRouting) agentlib.ServiceExecuteRequest {
		svc := newService()
		reviewer := &DefaultBeadReviewer{ProjectRoot: projectRoot, BeadStore: store, Service: svc}
		res, err := reviewer.ReviewBead(context.Background(), "ddx-pairing", head, impl)
		require.NoError(t, err)
		require.NotNil(t, res)
		return svc.lastReq
	}

	commonCorrelation := map[string]string{
		"bead_id":    "ddx-pairing",
		"attempt_id": "att-identity-invariant",
		"result_rev": head,
	}
	first := run(ImplementerRouting{
		Harness: "codex", Provider: "openai", Model: "gpt-5", ActualPower: 70,
		Correlation: commonCorrelation,
	})
	second := run(ImplementerRouting{
		Harness: "claude", Provider: "anthropic", Model: "claude-opus", ActualPower: 70,
		Correlation: commonCorrelation,
	})

	assert.Equal(t, first.Prompt, second.Prompt)
	assert.Equal(t, first.MinPower, second.MinPower)
	assert.Equal(t, first.Metadata, second.Metadata)
	for _, req := range []agentlib.ServiceExecuteRequest{first, second} {
		assert.Empty(t, req.Harness)
		assert.Empty(t, req.Provider)
		assert.Empty(t, req.Model)
		assert.Empty(t, req.Policy)
		assert.Equal(t, 71, req.MinPower)
		assert.NotContains(t, req.Metadata, "impl_harness")
		assert.NotContains(t, req.Metadata, "impl_provider")
		assert.NotContains(t, req.Metadata, "impl_model")
		assert.NotContains(t, req.Metadata, "impl_actual_power")
	}
}

func TestReviewRouting_MissingActualPowerUsesStrongAbstractFloor(t *testing.T) {
	projectRoot, head, store := reviewPairingTestSetup(t)
	profiles := []agentlib.PolicyInfo{
		{Name: "review-mid", MinPower: 9, MaxPower: 10},
		{Name: "review-high", MinPower: 71, MaxPower: 80},
	}
	models := []agentlib.ModelInfo{
		{ID: "review-mid-model", Power: 10, Available: true, AutoRoutable: true},
		{ID: "review-high-model", Power: 72, Available: true, AutoRoutable: true},
	}
	svc := &passthroughTestService{
		listPolicies: profiles,
		listModels:   models,
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
	assert.Empty(t, svc.lastReq.Policy)
	assert.Equal(t, lifecycleStrongMinPower, svc.lastReq.MinPower)
	assert.Empty(t, svc.lastReq.Model)
	assert.False(t, svc.listPoliciesCalled)
	assert.False(t, svc.listModelsCalled)
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
	assert.Empty(t, svc.lastReq.Policy)
	assert.Equal(t, 71, svc.lastReq.MinPower)
	assert.Empty(t, svc.lastReq.Model)
	assert.False(t, svc.listPoliciesCalled)
	assert.False(t, svc.listModelsCalled)
}

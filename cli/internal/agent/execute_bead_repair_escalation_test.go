package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recoveryTrackingStore struct {
	*bead.Store
	updateWithLifecycleStatusCalls int
	parkToProposedCalls            int
	closeWithEvidenceCalls         int
}

func (s *recoveryTrackingStore) UpdateWithLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error {
	s.updateWithLifecycleStatusCalls++
	return s.Store.UpdateWithLifecycleStatus(id, status, opts, mutate)
}

func (s *recoveryTrackingStore) ParkToProposed(id string, reason bead.ParkReason, mutate func(*bead.Bead)) error {
	s.parkToProposedCalls++
	return s.Store.ParkToProposed(id, reason, mutate)
}

func (s *recoveryTrackingStore) CloseWithEvidence(id, sessionID, commitSHA string) error {
	s.closeWithEvidenceCalls++
	return s.Store.CloseWithEvidence(id, sessionID, commitSHA)
}

func newPowerLadderRecoveryBead(t *testing.T, store *bead.Store, id string) *bead.Bead {
	t.Helper()
	b := &bead.Bead{
		ID:       id,
		Title:    "repair exhaustion recovery context",
		Priority: 3,
		Description: "PROBLEM\n" +
			"When review_fixable_gap repair retries consume the available abstract MinPower ladder, DDx must preserve the candidate result_rev and enter TD-031 auto-recovery.\n\n" +
			"PARENT\n" +
			"ddx-b79130f2 (100% reliability program epic)\n\n" +
			"DEPS\n" +
			"Related (not hard deps): ddx-83f007b3 preserve on exhaust; ddx-84fd24cb mixed-commit pin.\n\n" +
			"GOVERNING\n" +
			"TD-031 §classify_result retry_power and auto-recovery; FEAT-010 MinPower ladder; reliability P1/P7; operator thrash policy (no demotion).",
		Acceptance: "1. TestRepairExhaustedAfterPowerLadderPreservesAndRecovers\n2. TestRepairExhaustedAfterPowerLadderDoesNotLandOrDemote\n3. TestRepairExhaustedAfterPowerLadderRetainsRecoveryContext",
	}
	require.NoError(t, store.Create(context.Background(), b))
	return b
}

// TestReviewBlock_EscalatesImplementerWithoutBeadRetryFloorMetadata asserts that when
// repair-cycle-exhausted is returned and the escalation ladder has a higher
// powerClass available, the bead remains open without persisting a bead retry
// floor.
func TestReviewBlock_EscalatesImplementerWithoutBeadRetryFloorMetadata(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce01", Title: "Repair cycle exhausted escalation"}
	require.NoError(t, store.Create(context.Background(), b))

	worker := &ExecuteBeadWorker{
		Store: store,
		EscalationNextFloor: func(actualPower int) (int, error) {
			if actualPower < 70 {
				return 70, nil
			}
			return 0, fmt.Errorf("ladder exhausted")
		},
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusRepairCycleExhausted,
				ActualPower: 50,
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead must remain open after escalation")
	assert.NotContains(t, got.Extra, legacyRetryFloorKey)
}

func TestRepairFixableGapDoesNotRaiseOnTransportStop(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:    "ddx-rce-transport",
		Title: "Repair cycle exhausted transport stop attempt",
		Extra: map[string]any{
			"powerClass-pin":      "keep-me",
			"work-failed-routes":  []string{"route-a"},
			"requested_min_power": 42,
		},
	}
	require.NoError(t, store.Create(context.Background(), b))

	var floorCalls int
	err := applyRepairCycleExhaustedEscalation(
		context.Background(),
		store,
		b.ID,
		"worker",
		ExecuteBeadReport{
			BeadID:            b.ID,
			Status:            ExecuteBeadStatusRepairCycleExhausted,
			ActualPower:       55,
			OutcomeReason:     FailureModeProviderConnectivity,
			Disrupted:         true,
			DisruptionReason:  FailureModeProviderConnectivity,
			Provider:          "anthropic",
			Model:             "claude-sonnet-4-6",
			Harness:           "claude",
			RequestedMinPower: 42,
		},
		55,
		time.Now().UTC(),
		func(actualPower int) (int, error) {
			floorCalls++
			return actualPower + 10, nil
		},
		nil,
	)
	require.NoError(t, err)

	assert.Zero(t, floorCalls, "transport stop_attempt must not request a stronger MinPower")

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "transport stop_attempt must remain open")
	assert.Equal(t, "keep-me", got.Extra["powerClass-pin"])
	assert.Equal(t, []any{"route-a"}, got.Extra["work-failed-routes"])
	assert.Equal(t, float64(42), got.Extra["requested_min_power"])
}

func TestRepairFixableGapDoesNotRaiseOnAuthOrQuotaStop(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason string
	}{
		{name: "auth", reason: FailureModeProviderAuth},
		{name: "quota", reason: FailureModeProviderQuota},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := bead.NewStore(t.TempDir())
			require.NoError(t, store.Init(context.Background()))

			b := &bead.Bead{
				ID:    "ddx-rce-" + tc.name,
				Title: "Repair cycle exhausted " + tc.name + " stop attempt",
			}
			require.NoError(t, store.Create(context.Background(), b))

			var floorCalls int
			err := applyRepairCycleExhaustedEscalation(
				context.Background(),
				store,
				b.ID,
				"worker",
				ExecuteBeadReport{
					BeadID:            b.ID,
					Status:            ExecuteBeadStatusRepairCycleExhausted,
					ActualPower:       61,
					OutcomeReason:     tc.reason,
					Disrupted:         true,
					DisruptionReason:  tc.reason,
					Provider:          "openai",
					Model:             "gpt-5",
					Harness:           "agent",
					RequestedMinPower: 61,
				},
				61,
				time.Now().UTC(),
				func(actualPower int) (int, error) {
					floorCalls++
					return actualPower + 10, nil
				},
				nil,
			)
			require.NoError(t, err)

			assert.Zero(t, floorCalls, "typed %s stop_attempt must not request a stronger MinPower", tc.name)

			got, err := store.Get(context.Background(), b.ID)
			require.NoError(t, err)
			assert.Equal(t, bead.StatusOpen, got.Status, "typed %s stop_attempt must remain open", tc.name)
			assert.NotContains(t, got.Extra, legacyRetryFloorKey)
		})
	}
}

func TestRepairFixableGapStopAttemptDoesNotInspectRouteIdentity(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:    "ddx-rce-route",
		Title: "Repair cycle exhausted route identity regression",
		Extra: map[string]any{
			"powerClass-pin":     "operator-locked",
			"work-failed-routes": []string{"route-a", "route-b"},
		},
	}
	require.NoError(t, store.Create(context.Background(), b))

	err := applyRepairCycleExhaustedEscalation(
		context.Background(),
		store,
		b.ID,
		"worker",
		ExecuteBeadReport{
			BeadID:            b.ID,
			Status:            ExecuteBeadStatusRepairCycleExhausted,
			ActualPower:       58,
			OutcomeReason:     FailureModeProviderConnectivity,
			Disrupted:         true,
			DisruptionReason:  FailureModeProviderConnectivity,
			Provider:          "local-ollama",
			Model:             "qwen2.5-coder",
			Harness:           "codex",
			RequestedMinPower: 58,
		},
		58,
		time.Now().UTC(),
		func(int) (int, error) {
			t.Fatal("typed stop_attempt must not request a higher MinPower")
			return 0, nil
		},
		nil,
	)
	require.NoError(t, err)

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Equal(t, "operator-locked", got.Extra["powerClass-pin"])
	assert.Equal(t, []any{"route-a", "route-b"}, got.Extra["work-failed-routes"])
}

func TestRepairExhaustedAfterBlockEntersAutoRecovery(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce03", Title: "Repair cycle exhausted after BLOCK"}
	require.NoError(t, store.Create(context.Background(), b))

	var hookCalls int
	var observedClass RecoveryFailureClass
	hook := PostLadderExhaustionHook(func(_ context.Context, beadID string, class RecoveryFailureClass, review PostLadderExhaustionContext) (*PostLadderExhaustionResult, error) {
		hookCalls++
		assert.Equal(t, b.ID, beadID)
		observedClass = class
		assert.Equal(t, "rg-block", review.ReviewGroupID)
		assert.Equal(t, ReviewTerminalClassSpecGap, review.ReviewClassification)
		require.Len(t, review.ReviewFindings, 1)
		assert.Equal(t, "missing regression test", review.ReviewFindings[0].Summary)
		return &PostLadderExhaustionResult{Attempted: true, Succeeded: true, Path: Reframe}, nil
	})

	report := ExecuteBeadReport{
		BeadID:               b.ID,
		Status:               ExecuteBeadStatusRepairCycleExhausted,
		ActualPower:          50,
		ReviewVerdict:        string(VerdictBlock),
		ReviewRationale:      "missing regression test",
		ReviewGroupID:        "rg-block",
		ReviewClassification: ReviewTerminalClassSpecGap,
		ReviewPerAC: []ReviewAC{{
			Number:   1,
			Item:     "Add regression test",
			Grade:    "BLOCK",
			Evidence: "pkg/foo_test.go:42",
		}},
		ReviewFindings: []Finding{{
			Severity: "block",
			Summary:  "missing regression test",
			Location: "pkg/foo_test.go:42",
		}},
	}

	err := applyRepairCycleExhaustedEscalation(context.Background(), store, b.ID, "worker", report, report.ActualPower, time.Now().UTC(), func(int) (int, error) {
		return 0, fmt.Errorf("ladder exhausted")
	}, hook)
	require.NoError(t, err)

	assert.Equal(t, 1, hookCalls, "repair exhaustion must invoke TD-031 auto-recovery")
	assert.Equal(t, SpecGap, observedClass, "evidenced BLOCK must recover using the spec-gap class")

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "recovery hook should short-circuit the old park-to-proposed fallback")

	event := findRepairCycleExhaustedEvent(t, store, b.ID)
	var body repairCycleExhaustedEventBody
	require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
	assert.Equal(t, "rg-block", body.ReviewGroupID)
	require.Len(t, body.ReviewFindings, 1)
	assert.Equal(t, "missing regression test", body.ReviewFindings[0].Summary)
}

func TestRepairExhaustedRecoveryLinksReviewGroup(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce04", Title: "Repair cycle exhausted links review group"}
	require.NoError(t, store.Create(context.Background(), b))

	report := ExecuteBeadReport{
		BeadID:               b.ID,
		Status:               ExecuteBeadStatusRepairCycleExhausted,
		ActualPower:          65,
		ReviewVerdict:        string(VerdictBlock),
		ReviewRationale:      "spec gap needs a rewrite",
		ReviewGroupID:        "rg-link",
		ReviewClassification: ReviewTerminalClassSpecGap,
		ReviewPerAC: []ReviewAC{{
			Number:   2,
			Item:     "Document edge case",
			Grade:    "BLOCK",
			Evidence: "docs/notes.md:12",
		}},
		ReviewFindings: []Finding{{
			Severity: "block",
			Summary:  "spec gap needs a rewrite",
			Location: "docs/notes.md:12",
		}},
	}

	hook := PostLadderExhaustionHook(func(_ context.Context, beadID string, class RecoveryFailureClass, review PostLadderExhaustionContext) (*PostLadderExhaustionResult, error) {
		assert.Equal(t, b.ID, beadID)
		assert.Equal(t, SpecGap, class)
		assert.Equal(t, "rg-link", review.ReviewGroupID)
		assert.Equal(t, ReviewTerminalClassSpecGap, review.ReviewClassification)
		require.Len(t, review.ReviewFindings, 1)
		assert.Equal(t, "spec gap needs a rewrite", review.ReviewFindings[0].Summary)
		return &PostLadderExhaustionResult{Attempted: true, Succeeded: true, Path: Reframe}, nil
	})

	err := applyRepairCycleExhaustedEscalation(context.Background(), store, b.ID, "worker", report, report.ActualPower, time.Now().UTC(), func(int) (int, error) {
		return 0, fmt.Errorf("ladder exhausted")
	}, hook)
	require.NoError(t, err)

	event := findRepairCycleExhaustedEvent(t, store, b.ID)
	var body repairCycleExhaustedEventBody
	require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
	assert.Equal(t, "rg-link", body.ReviewGroupID)
	assert.Equal(t, ReviewTerminalClassSpecGap, body.ReviewClassification)
	require.Len(t, body.ReviewFindings, 1)
	assert.Equal(t, "spec gap needs a rewrite", body.ReviewFindings[0].Summary)
}

func TestRepairExhaustedFixableGapEntersAutoRecovery(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce05", Title: "Repair cycle exhausted after fixable gap"}
	require.NoError(t, store.Create(context.Background(), b))

	var hookCalls int
	var observedClass RecoveryFailureClass
	hook := PostLadderExhaustionHook(func(_ context.Context, beadID string, class RecoveryFailureClass, review PostLadderExhaustionContext) (*PostLadderExhaustionResult, error) {
		hookCalls++
		assert.Equal(t, b.ID, beadID)
		observedClass = class
		assert.Equal(t, "rg-gap", review.ReviewGroupID)
		assert.Equal(t, ReviewFindingClassFixableGap, review.ReviewClassification)
		require.Len(t, review.ReviewFindings, 1)
		assert.Equal(t, "small fixable gap", review.ReviewFindings[0].Summary)
		return &PostLadderExhaustionResult{Attempted: true, Succeeded: true, Path: Decompose}, nil
	})

	report := ExecuteBeadReport{
		BeadID:               b.ID,
		Status:               ExecuteBeadStatusRepairCycleExhausted,
		ActualPower:          40,
		ReviewVerdict:        string(VerdictRequestChanges),
		ReviewRationale:      "small fixable gap",
		ReviewGroupID:        "rg-gap",
		ReviewClassification: ReviewFindingClassFixableGap,
		ReviewPerAC: []ReviewAC{{
			Number:   3,
			Item:     "Add missing assertion",
			Grade:    "REQUEST_CHANGES",
			Evidence: "pkg/foo_test.go:84",
		}},
		ReviewFindings: []Finding{{
			Severity: "warn",
			Summary:  "small fixable gap",
			Location: "pkg/foo_test.go:84",
		}},
	}

	err := applyRepairCycleExhaustedEscalation(context.Background(), store, b.ID, "worker", report, report.ActualPower, time.Now().UTC(), func(int) (int, error) {
		return 0, fmt.Errorf("ladder exhausted")
	}, hook)
	require.NoError(t, err)

	assert.Equal(t, 1, hookCalls, "fixable-gap exhaustion must invoke TD-031 auto-recovery")
	assert.Equal(t, PersistentExecutionFailed, observedClass, "fixable-gap recovery should reuse the persistent execution failure path")

	event := findRepairCycleExhaustedEvent(t, store, b.ID)
	var body repairCycleExhaustedEventBody
	require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
	assert.Equal(t, "rg-gap", body.ReviewGroupID)
	assert.Equal(t, ReviewFindingClassFixableGap, body.ReviewClassification)
}

func findRepairCycleExhaustedEvent(t *testing.T, store *bead.Store, beadID string) bead.BeadEvent {
	t.Helper()
	events, err := store.Events(beadID)
	require.NoError(t, err)
	for _, ev := range events {
		if ev.Kind == ExecuteBeadStatusRepairCycleExhausted {
			return ev
		}
	}
	t.Fatalf("repair-cycle-exhausted event not found")
	return bead.BeadEvent{}
}

func TestRepairExhaustedAfterPowerLadderPreservesAndRecovers(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := newPowerLadderRecoveryBead(t, store, "ddx-rce-power-ladder")
	reviewRev := "repair-rev"
	candidateRef := candidateIterationRef("attempt-power-ladder", 1)
	preserveRef := candidateRef

	hookCalls := 0
	hook := PostLadderExhaustionHook(func(_ context.Context, beadID string, class RecoveryFailureClass, review PostLadderExhaustionContext) (*PostLadderExhaustionResult, error) {
		hookCalls++
		assert.Equal(t, b.ID, beadID)
		assert.Equal(t, PersistentExecutionFailed, class)
		assert.Equal(t, reviewRev, review.ResultRev)
		assert.Equal(t, candidateRef, review.CandidateRef)
		assert.Equal(t, preserveRef, review.PreserveRef)
		assert.Equal(t, "rg-power-ladder", review.ReviewGroupID)
		assert.Equal(t, ReviewFindingClassFixableGap, review.ReviewClassification)
		require.Equal(t, "ddx-b79130f2", review.Parent)
		assert.ElementsMatch(t, []string{"ddx-83f007b3", "ddx-84fd24cb"}, review.RelatedDeps)
		assert.ElementsMatch(t, []string{"TD-031", "FEAT-010"}, review.GoverningRefs)
		require.Len(t, review.ReviewFindings, 1)
		assert.Equal(t, "missing regression coverage", review.ReviewFindings[0].Summary)
		return &PostLadderExhaustionResult{Attempted: true, Succeeded: true, Path: Reframe}, nil
	})

	report := ExecuteBeadReport{
		BeadID:               b.ID,
		Status:               ExecuteBeadStatusRepairCycleExhausted,
		ActualPower:          50,
		ResultRev:            reviewRev,
		CandidateRef:         candidateRef,
		PreserveRef:          preserveRef,
		ReviewVerdict:        string(VerdictBlock),
		ReviewRationale:      "missing regression coverage",
		ReviewGroupID:        "rg-power-ladder",
		ReviewClassification: ReviewFindingClassFixableGap,
		ReviewPerAC: []ReviewAC{{
			Number:   1,
			Item:     "Add regression coverage",
			Grade:    "REQUEST_CHANGES",
			Evidence: "cli/internal/agent/execute_bead_repair_escalation_test.go:1",
		}},
		ReviewFindings: []Finding{{
			Severity: "warn",
			Summary:  "missing regression coverage",
			Location: "cli/internal/agent/execute_bead_repair_escalation_test.go:1",
		}},
	}

	err := applyRepairCycleExhaustedEscalation(
		context.Background(),
		store,
		b.ID,
		"worker",
		report,
		report.ActualPower,
		time.Now().UTC(),
		func(int) (int, error) {
			return 0, fmt.Errorf("ladder exhausted")
		},
		hook,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, hookCalls, "repair exhaustion must enter TD-031 auto-recovery")

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "recovery success must keep the bead open")
	assert.Equal(t, 3, got.Priority, "recovery success must not demote priority")

	event := findRepairCycleExhaustedEvent(t, store, b.ID)
	assert.Equal(t, ExecuteBeadStatusRepairCycleExhausted, event.Kind)
	assert.Contains(t, event.Body, `"review_classification":"review_fixable_gap"`)
}

func TestRepairExhaustedAfterPowerLadderDoesNotLandOrDemote(t *testing.T) {
	baseStore := bead.NewStore(t.TempDir())
	require.NoError(t, baseStore.Init(context.Background()))
	store := &recoveryTrackingStore{Store: baseStore}

	b := newPowerLadderRecoveryBead(t, baseStore, "ddx-rce-no-land")
	report := ExecuteBeadReport{
		BeadID:               b.ID,
		Status:               ExecuteBeadStatusRepairCycleExhausted,
		ActualPower:          50,
		ResultRev:            "repair-rev",
		CandidateRef:         candidateIterationRef("attempt-no-land", 1),
		PreserveRef:          candidateIterationRef("attempt-no-land", 1),
		ReviewVerdict:        string(VerdictBlock),
		ReviewRationale:      "missing regression coverage",
		ReviewGroupID:        "rg-no-land",
		ReviewClassification: ReviewFindingClassFixableGap,
		ReviewFindings: []Finding{{
			Severity: "warn",
			Summary:  "missing regression coverage",
			Location: "cli/internal/agent/execute_bead_repair_escalation_test.go:1",
		}},
	}

	err := applyRepairCycleExhaustedEscalation(
		context.Background(),
		store,
		b.ID,
		"worker",
		report,
		report.ActualPower,
		time.Now().UTC(),
		func(int) (int, error) {
			return 0, fmt.Errorf("ladder exhausted")
		},
		PostLadderExhaustionHook(func(context.Context, string, RecoveryFailureClass, PostLadderExhaustionContext) (*PostLadderExhaustionResult, error) {
			return &PostLadderExhaustionResult{Attempted: true, Succeeded: true, Path: Reframe}, nil
		}),
	)
	require.NoError(t, err)

	got, err := baseStore.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "recovery success must keep the bead open")
	assert.Equal(t, 3, got.Priority, "recovery success must not demote priority")
	assert.Equal(t, 0, store.updateWithLifecycleStatusCalls, "recovery success must not demote or close the bead")
	assert.Equal(t, 0, store.parkToProposedCalls, "recovery success must not park the bead")
	assert.Equal(t, 0, store.closeWithEvidenceCalls, "recovery success must not land the bead")
}

func TestRepairExhaustedAfterPowerLadderRetainsRecoveryContext(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := newPowerLadderRecoveryBead(t, store, "ddx-rce-context")
	report := ExecuteBeadReport{
		BeadID:               b.ID,
		Status:               ExecuteBeadStatusRepairCycleExhausted,
		ActualPower:          50,
		ResultRev:            "repair-rev",
		CandidateRef:         candidateIterationRef("attempt-context", 1),
		PreserveRef:          candidateIterationRef("attempt-context", 1),
		ReviewVerdict:        string(VerdictBlock),
		ReviewRationale:      "missing regression coverage",
		ReviewGroupID:        "rg-context",
		ReviewClassification: ReviewFindingClassFixableGap,
		ReviewPerAC: []ReviewAC{{
			Number:   1,
			Item:     "Add regression coverage",
			Grade:    "REQUEST_CHANGES",
			Evidence: "cli/internal/agent/execute_bead_repair_escalation_test.go:1",
		}},
		ReviewFindings: []Finding{{
			Severity: "warn",
			Summary:  "missing regression coverage",
			Location: "cli/internal/agent/execute_bead_repair_escalation_test.go:1",
		}},
	}

	var gotContext PostLadderExhaustionContext
	err := applyRepairCycleExhaustedEscalation(
		context.Background(),
		store,
		b.ID,
		"worker",
		report,
		report.ActualPower,
		time.Now().UTC(),
		func(int) (int, error) {
			return 0, fmt.Errorf("ladder exhausted")
		},
		PostLadderExhaustionHook(func(_ context.Context, beadID string, class RecoveryFailureClass, review PostLadderExhaustionContext) (*PostLadderExhaustionResult, error) {
			assert.Equal(t, b.ID, beadID)
			assert.Equal(t, PersistentExecutionFailed, class)
			gotContext = review
			return &PostLadderExhaustionResult{Attempted: true, Succeeded: true, Path: Reframe}, nil
		}),
	)
	require.NoError(t, err)

	assert.Equal(t, "ddx-b79130f2", gotContext.Parent)
	assert.ElementsMatch(t, []string{"ddx-83f007b3", "ddx-84fd24cb"}, gotContext.RelatedDeps)
	assert.ElementsMatch(t, []string{"TD-031", "FEAT-010"}, gotContext.GoverningRefs)
	assert.Equal(t, "repair-rev", gotContext.ResultRev)
	assert.Equal(t, candidateIterationRef("attempt-context", 1), gotContext.CandidateRef)
	assert.Equal(t, candidateIterationRef("attempt-context", 1), gotContext.PreserveRef)
	assert.Equal(t, "rg-context", gotContext.ReviewGroupID)
	assert.Equal(t, ReviewFindingClassFixableGap, gotContext.ReviewClassification)
	require.Len(t, gotContext.ReviewFindings, 1)
	assert.Equal(t, "missing regression coverage", gotContext.ReviewFindings[0].Summary)
}

// TestReviewBlock_StillFailsAtTopPowerClass_ParkProposed asserts that when
// repair-cycle-exhausted occurs at the top powerClass (EscalationNextFloor errors),
// the bead is parked to proposed for operator review.
func TestReviewBlock_StillFailsAtTopPowerClass_ParkProposed(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-rce02", Title: "Repair cycle exhausted at top powerClass"}
	require.NoError(t, store.Create(context.Background(), b))

	worker := &ExecuteBeadWorker{
		Store: store,
		EscalationNextFloor: func(actualPower int) (int, error) {
			return 0, fmt.Errorf("ladder exhausted")
		},
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusRepairCycleExhausted,
				ActualPower: 90,
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "bead must be parked to proposed when ladder exhausted")
	assert.Empty(t, got.Owner, "proposed bead must not be owned")
}

// TestReviewBlock_NonFixableClassification_DoesNotEscalate asserts that
// review_terminal_block (non-fixable: spec gap, too_large, unsafe) is NOT in
// EscalatableStatuses — a smarter model cannot fix a structural spec problem.
func TestReviewBlock_NonFixableClassification_DoesNotEscalate(t *testing.T) {
	assert.False(t, escalation.EscalatableStatuses[ExecuteBeadStatusReviewTerminalBlock],
		"review_terminal_block must not trigger escalation — spec gap / too_large requires operator decision")
}

package try

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttempt_WrapsLegacyExecutor(t *testing.T) {
	out, err := Attempt(context.Background(), nil, "ddx-test", AttemptOpts{
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{BeadID: beadID, Status: StatusSuccess, ResultRev: "abc123"}, nil
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, "ddx-test", out.Report.BeadID)
	assert.Equal(t, StatusSuccess, out.Report.Status)
	assert.Equal(t, "abc123", out.Report.ResultRev)
}

func TestAttempt_NoChangesRejectsLegacyNeedsInvestigationStatus(t *testing.T) {
	store := &attemptStore{}

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:             beadID,
				Status:             StatusNoChanges,
				NoChangesRationale: "status: needs_investigation\nreason: provider quota unknown",
			}, nil
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, StatusNoChanges, out.Report.Status)
	require.NotNil(t, out.NoChanges)
	assert.False(t, out.NoChanges.Satisfied)
	assert.Equal(t, NoChangesActionBadAttemptNoCooldown, out.NoChanges.Action)
	assert.False(t, out.NoChanges.CooldownEligible)
	assert.Equal(t, NoChangesEventLegacyStatusRejected, out.NoChanges.EventKind)
	assert.Empty(t, out.NoChanges.Label)
	assert.Contains(t, out.NoChanges.EventBody, "status: needs_investigation is no longer accepted")
	assert.Contains(t, out.NoChanges.EventBody, "ddx bead migrate --lifecycle")
	assert.Contains(t, out.NoChanges.EventBody, "provider quota unknown")
	assert.Equal(t, 1, store.noChangesCountCalls)
}

func TestAttempt_NoChanges_WiresAdjudicateNoChangesContract(t *testing.T) {
	store := &attemptStore{}
	runnerCalls := 0

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:             beadID,
				Status:             StatusNoChanges,
				NoChangesRationale: "verification_command: true\noutput: passes",
			}, nil
		}),
		VerificationRunner: func(ctx context.Context, projectRoot, command string) (int, string, error) {
			runnerCalls++
			assert.Equal(t, "true", command)
			return 0, "ok", nil
		},
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, StatusAlreadySatisfied, out.Report.Status)
	require.NotNil(t, out.NoChanges)
	assert.True(t, out.NoChanges.Satisfied)
	assert.Equal(t, NoChangesActionCloseAlreadySatisfied, out.NoChanges.Action)
	assert.False(t, out.NoChanges.CooldownEligible)
	assert.Equal(t, NoChangesEventVerified, out.NoChanges.EventKind)
	assert.Empty(t, out.NoChanges.Label)
	assert.Equal(t, 1, runnerCalls)
}

func TestAttempt_NoChangesVerified_ReturnsCloseAlreadySatisfied(t *testing.T) {
	store := &attemptStore{}

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:             beadID,
				Status:             StatusNoChanges,
				NoChangesRationale: "verification_command: true",
			}, nil
		}),
		VerificationRunner: func(context.Context, string, string) (int, string, error) {
			return 0, "already green", nil
		},
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, StatusAlreadySatisfied, out.Report.Status)
	require.NotNil(t, out.NoChanges)
	assert.True(t, out.NoChanges.Satisfied)
	assert.Equal(t, NoChangesActionCloseAlreadySatisfied, out.NoChanges.Action)
	assert.False(t, out.NoChanges.CooldownEligible)
	assert.Equal(t, NoChangesEventVerified, out.NoChanges.EventKind)
	assert.Empty(t, out.NoChanges.Label)
	assert.Equal(t, 1, store.noChangesCountCalls)
}

func TestAttempt_NoChangesOpen_ReturnsSmartRetryNoCooldown(t *testing.T) {
	store := &attemptStore{}

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:             beadID,
				Status:             StatusNoChanges,
				NoChangesRationale: "status: open\nreason: rerun with a stronger model\nsuggested_action: retry with smart agent",
			}, nil
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, StatusNoChanges, out.Report.Status)
	require.NotNil(t, out.NoChanges)
	assert.False(t, out.NoChanges.Satisfied)
	assert.Equal(t, NoChangesActionKeepOpenSmartRetry, out.NoChanges.Action)
	assert.False(t, out.NoChanges.CooldownEligible)
	assert.Equal(t, NoChangesEventAutonomousRetry, out.NoChanges.EventKind)
	assert.Empty(t, out.NoChanges.Label)
	assert.Equal(t, "open", out.NoChanges.LifecycleStatus)
	assert.Equal(t, "rerun with a stronger model", out.NoChanges.Reason)
	assert.Equal(t, "retry with smart agent", out.NoChanges.SuggestedAction)
	assert.Empty(t, out.Report.RetryAfter)
}

func TestAttempt_NoChangesBlockedInternalScopeStaysAutonomous(t *testing.T) {
	store := &attemptStore{}

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:             beadID,
				Status:             StatusNoChanges,
				NoChangesRationale: "status: blocked\nreason: External blocker: cannot be satisfied inside this bead alone; requires a broader API migration outside this bead\nfollow_up_needed: split into executable children",
			}, nil
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	require.NotNil(t, out.NoChanges)
	assert.Equal(t, NoChangesActionKeepOpenSmartRetry, out.NoChanges.Action)
	assert.Equal(t, NoChangesEventAutonomousRetry, out.NoChanges.EventKind)
	assert.Equal(t, "open", out.NoChanges.LifecycleStatus)
	assert.Contains(t, out.NoChanges.SuggestedAction, "decompose")
}

func TestAttempt_NoChangesUnjustified_ReturnsBadAttemptNoLongCooldown(t *testing.T) {
	store := &attemptStore{}

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID: beadID,
				Status: StatusNoChanges,
			}, nil
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, StatusNoChanges, out.Report.Status)
	require.NotNil(t, out.NoChanges)
	assert.False(t, out.NoChanges.Satisfied)
	assert.Equal(t, NoChangesActionBadAttemptNoCooldown, out.NoChanges.Action)
	assert.False(t, out.NoChanges.CooldownEligible)
	assert.Equal(t, NoChangesEventUnjustified, out.NoChanges.EventKind)
	assert.Equal(t, NoChangesLabelUnjustified, out.NoChanges.Label)
	assert.Contains(t, out.NoChanges.EventBody, "rationale absent")
	assert.Empty(t, out.Report.RetryAfter)
}

func TestAttempt_ConflictRecovered_ReturnsMergedOutcome(t *testing.T) {
	store := &attemptStore{}
	target := bead.Bead{ID: "ddx-test"}

	out, err := Attempt(context.Background(), store, target.ID, AttemptOpts{
		Bead: target,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:      beadID,
				Status:      StatusLandConflict,
				Detail:      "merge conflict",
				PreserveRef: "refs/ddx/iterations/ddx-test/attempt",
				BaseRev:     "base",
				ResultRev:   "result",
				SessionID:   "session",
			}, nil
		}),
		Store:       store,
		ProjectRoot: t.TempDir(),
		AutoRecover: func(wd, preserveRef string) (string, error) {
			return "merged-tip", nil
		},
		Assignee: "worker",
		Now:      func() time.Time { return time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC) },
		Cooldown: time.Minute,
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeSuccess, out.Disposition)
	assert.Equal(t, StatusSuccess, out.Report.Status)
	assert.Equal(t, "merged-tip", out.Report.ResultRev)
	assert.Equal(t, "merged-tip", store.closedSHA)
	require.Len(t, store.events, 1)
	assert.Equal(t, "land-conflict-auto-recovered", store.events[0].Kind)
}

func TestAttempt_ConflictUnresolvable_ReturnsPark(t *testing.T) {
	store := &attemptStore{}
	target := bead.Bead{ID: "ddx-test"}

	out, err := Attempt(context.Background(), store, target.ID, AttemptOpts{
		Bead: target,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:      beadID,
				Status:      StatusLandConflict,
				Detail:      "merge conflict",
				PreserveRef: "refs/ddx/iterations/ddx-test/attempt",
				BaseRev:     "base",
				ResultRev:   "result",
				SessionID:   "session",
			}, nil
		}),
		Store:       store,
		ProjectRoot: t.TempDir(),
		AutoRecover: func(wd, preserveRef string) (string, error) {
			return "", fmt.Errorf("cannot auto-merge")
		},
		Assignee: "worker",
		Now:      func() time.Time { return time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC) },
		Cooldown: 15 * time.Minute,
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomePark, out.Disposition)
	assert.Equal(t, StatusLandConflictUnresolvable, out.Report.Status)
	assert.Contains(t, out.Report.Detail, "git merge --no-ff refs/ddx/iterations/ddx-test/attempt")
	assert.True(t, store.unclaimed)
	assert.Equal(t, StatusLandConflictUnresolvable, store.cooldownStatus)
	assert.NotEmpty(t, out.Report.RetryAfter)
	require.Len(t, store.events, 1)
	assert.Equal(t, "land-conflict-unresolvable", store.events[0].Kind)
	assert.Contains(t, store.events[0].Body, "rescue_command")
	assert.Contains(t, store.events[0].Body, "git merge --no-ff refs/ddx/iterations/ddx-test/attempt")
}

func TestAttempt_ConflictOperatorRequired_MovesToProposed(t *testing.T) {
	store := &attemptStore{}
	target := bead.Bead{ID: "ddx-test"}

	out, err := Attempt(context.Background(), store, target.ID, AttemptOpts{
		Bead: target,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:      beadID,
				Status:      StatusLandConflict,
				Detail:      "merge conflict",
				PreserveRef: "refs/ddx/iterations/ddx-test/attempt",
				BaseRev:     "base",
				ResultRev:   "result",
				SessionID:   "session",
			}, nil
		}),
		Store:       store,
		ProjectRoot: t.TempDir(),
		AutoRecover: func(wd, preserveRef string) (string, error) {
			return "", fmt.Errorf("cannot auto-merge")
		},
		ConflictResolver: func(ctx context.Context, beadID, preserveRef, projectRoot string) (string, bool, error) {
			return "", true, fmt.Errorf("requires operator judgment")
		},
		Assignee: "worker",
		Now:      func() time.Time { return time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC) },
		Cooldown: 15 * time.Minute,
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomePark, out.Disposition)
	assert.Equal(t, StatusLandConflictOperatorRequired, out.Report.Status)
	assert.Contains(t, out.Report.Detail, "git merge --no-ff refs/ddx/iterations/ddx-test/attempt")
	assert.True(t, store.unclaimed)
	assert.Equal(t, bead.StatusProposed, store.lifecycleStatus)
	assert.Empty(t, store.cooldownStatus, "operator-required conflicts must not rely on cooldown parking")
	require.Len(t, store.events, 1)
	assert.Equal(t, "land-conflict-operator-required", store.events[0].Kind)
	assert.Contains(t, store.events[0].Body, "rescue_command")
	assert.Contains(t, store.events[0].Body, "git merge --no-ff refs/ddx/iterations/ddx-test/attempt")
}

func TestAttempt_DeclinedNeedsDecomposition_ParksWithStructuredEvent(t *testing.T) {
	store := &attemptStore{}
	now := time.Date(2026, 5, 5, 10, 11, 12, 0, time.UTC)
	recommended := []string{"split: route trace", "split: retry policy"}
	rationale := "scope is too large for one pass"

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:                      beadID,
				Status:                      StatusDeclinedNeedsDecomposition,
				Detail:                      "decompose this bead",
				BaseRev:                     "base",
				SessionID:                   "session",
				DecompositionRationale:      rationale,
				DecompositionRecommendation: recommended,
			}, nil
		}),
		Now: func() time.Time { return now },
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomePark, out.Disposition)
	require.NotNil(t, out.Parking)
	assert.True(t, out.Parking.Unclaim)
	assert.True(t, out.Parking.RunPostAttemptTriage)
	require.NotNil(t, out.Parking.Event)
	assert.Equal(t, "decomposition-recommendation", out.Parking.Event.Kind)
	assert.True(t, out.Parking.RetryAfter.IsZero(), "declined_needs_decomposition must not park with cooldown")
	assert.Empty(t, out.Report.RetryAfter, "declined_needs_decomposition must not set work-retry-after")
	require.Len(t, store.events, 0)

	var body struct {
		Rationale           string   `json:"rationale"`
		RecommendedSubbeads []string `json:"recommended_subbeads"`
	}
	require.NoError(t, json.Unmarshal([]byte(out.Parking.Event.Body), &body))
	assert.Equal(t, rationale, body.Rationale)
	assert.Equal(t, recommended, body.RecommendedSubbeads)
}

func TestDeclinedNeedsDecomposition_NoCooldown(t *testing.T) {
	store := &attemptStore{}
	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{BeadID: beadID, Status: StatusDeclinedNeedsDecomposition}, nil
		}),
	})
	require.NoError(t, err)
	assert.Empty(t, out.Report.RetryAfter, "declined_needs_decomposition must not set work-retry-after")
	require.NotNil(t, out.Parking)
	assert.True(t, out.Parking.RetryAfter.IsZero(), "declined_needs_decomposition must not park with a time-based cooldown")
	assert.Empty(t, store.cooldownStatus, "declined_needs_decomposition must not call SetExecutionCooldown")
}

func TestDeclinedNeedsDecomposition_NotEligible(t *testing.T) {
	store := &attemptStore{}
	_, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{BeadID: beadID, Status: StatusDeclinedNeedsDecomposition}, nil
		}),
	})
	require.NoError(t, err)
	assert.Equal(t, false, store.mutatedBead.Extra[bead.ExtraExecutionElig], "execution-eligible must be set to false")
	assert.Equal(t, bead.StatusOpen, store.lifecycleStatus, "status must transition to open")
}

func TestDeclinedNeedsDecomposition_NotInReady(t *testing.T) {
	dir := t.TempDir()
	s := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, s.Init())

	b := &bead.Bead{Title: "too-big bead", IssueType: "task", Priority: 1}
	require.NoError(t, s.Create(b))

	adapter := &beadStoreAdapter{s: s}
	_, err := Attempt(context.Background(), adapter, b.ID, AttemptOpts{
		Store: adapter,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{BeadID: beadID, Status: StatusDeclinedNeedsDecomposition}, nil
		}),
	})
	require.NoError(t, err)

	ready, readyErr := s.Ready()
	require.NoError(t, readyErr)
	for _, rb := range ready {
		assert.NotEqual(t, b.ID, rb.ID, "bead should be excluded from Ready() due to execution-eligible=false, not a cooldown")
	}
}

func TestAttempt_PushFailed_NoPark_NoRetryAfter(t *testing.T) {
	store := &attemptStore{}

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID: beadID,
				Status: StatusPushFailed,
				Detail: PushFailedReasonPrefix + " remote rejected",
			}, nil
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Nil(t, out.Parking)
	assert.Empty(t, out.Report.RetryAfter)
	assert.Empty(t, store.cooldownStatus)
	require.Empty(t, store.events)
}

func TestAttempt_PushFailed_TransportUnavailable_NoCooldown(t *testing.T) {
	store := &attemptStore{}

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID: beadID,
				Status: StatusPushFailed,
				Detail: PushFailedReasonPrefix + " transport unavailable",
			}, nil
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Nil(t, out.Parking)
	assert.Empty(t, out.Report.RetryAfter)
	assert.Equal(t, StatusPushFailed, out.Report.Status)
	assert.Nil(t, out.NoChanges)
	assert.Empty(t, store.cooldownStatus)
}

func TestPushFailed_NoCooldown(t *testing.T) {
	store := &attemptStore{}

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID: beadID,
				Status: StatusPushFailed,
				Detail: PushFailedReasonPrefix + " pre-receive hook declined",
			}, nil
		}),
		Now: func() time.Time { return time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC) },
	})
	require.NoError(t, err)

	// push_failed must not park — the outcome is reported so the execute loop
	// unclaims the bead, making it immediately re-claimable.
	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Nil(t, out.Parking, "push_failed must not carry parking instructions")
	assert.Empty(t, out.Report.RetryAfter, "push_failed must not set work-retry-after")
	assert.Empty(t, store.cooldownStatus, "push_failed must not call SetExecutionCooldown")
}

func TestAttempt_PushConflict_ParksWithCooldown(t *testing.T) {
	store := &attemptStore{}
	now := time.Date(2026, 5, 5, 10, 11, 12, 0, time.UTC)
	detail := PushConflictReasonPrefix + " auto-recovery retry push: CONFLICT (content)"

	out, err := Attempt(context.Background(), store, "ddx-test", AttemptOpts{
		Store: store,
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID:    beadID,
				Status:    StatusPushConflict,
				Detail:    detail,
				BaseRev:   "base",
				ResultRev: "result",
				SessionID: "session",
			}, nil
		}),
		Now: func() time.Time { return now },
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomePark, out.Disposition)
	require.NotNil(t, out.Parking)
	assert.True(t, out.Parking.Unclaim)
	assert.True(t, out.Parking.RunPostAttemptTriage)
	require.NotNil(t, out.Parking.Event)
	assert.Equal(t, "push-conflict", out.Parking.Event.Kind)
	assert.Equal(t, now.Add(maxAttemptCooldown), out.Parking.RetryAfter)
	assert.Equal(t, now.Add(maxAttemptCooldown).Format(time.RFC3339), out.Report.RetryAfter)
	require.Empty(t, store.events)
	assert.Contains(t, out.Parking.Event.Body, detail)
}

func TestAttempt_RateLimit_Retry_HonorsRetryAfter(t *testing.T) {
	var sleepCalls []time.Duration
	calls := 0

	out, err := Attempt(context.Background(), nil, "ddx-test", AttemptOpts{
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			calls++
			if calls == 1 {
				return Report{
					BeadID: beadID,
					Status: StatusExecutionFailed,
					Error:  "cancelled: auth/rate-limit detected",
					Stderr: "HTTP 429\nRetry-After: 7\n",
					Detail: "rate limit",
				}, nil
			}
			return Report{BeadID: beadID, Status: StatusSuccess}, nil
		}),
		RateLimitBudget:     5 * time.Minute,
		RateLimitPerWaitCap: 60 * time.Second,
		RateLimitSleep: func(_ context.Context, d time.Duration) error {
			sleepCalls = append(sleepCalls, d)
			return nil
		},
		RateLimitNow: func() time.Time {
			return time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
		},
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, StatusSuccess, out.Report.Status)
	assert.Equal(t, 2, calls)
	require.Len(t, sleepCalls, 1)
	assert.Equal(t, 7*time.Second, sleepCalls[0])
}

func TestAttempt_RateLimit_Retry_UsesExponentialBackoff(t *testing.T) {
	var sleepCalls []time.Duration
	calls := 0

	out, err := Attempt(context.Background(), nil, "ddx-test", AttemptOpts{
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			calls++
			if calls < 3 {
				return Report{
					BeadID: beadID,
					Status: StatusExecutionFailed,
					Error:  "rate limit exceeded",
					Detail: "rate limit",
				}, nil
			}
			return Report{BeadID: beadID, Status: StatusSuccess}, nil
		}),
		RateLimitBudget:     5 * time.Minute,
		RateLimitPerWaitCap: 60 * time.Second,
		RateLimitSleep: func(_ context.Context, d time.Duration) error {
			sleepCalls = append(sleepCalls, d)
			return nil
		},
	})
	require.NoError(t, err)

	assert.Equal(t, OutcomeReported, out.Disposition)
	assert.Equal(t, StatusSuccess, out.Report.Status)
	assert.Equal(t, 3, calls)
	require.Equal(t, []time.Duration{1 * time.Second, 5 * time.Second}, sleepCalls)
}

func TestAttempt_RateLimit_WiresEvaluateRateLimitWait(t *testing.T) {
	evaluatorCalls := 0
	out, err := Attempt(context.Background(), nil, "ddx-test", AttemptOpts{
		Executor: ExecutorFunc(func(ctx context.Context, beadID string) (Report, error) {
			return Report{
				BeadID: beadID,
				Status: StatusExecutionFailed,
				Error:  "429 rate limit exceeded",
			}, nil
		}),
		RateLimitWaitEvaluator: func(retryAfter time.Duration, attempt int, elapsed, budget, perWaitCap time.Duration) RateLimitWaitDecision {
			evaluatorCalls++
			return RateLimitWaitDecision{ShouldRetry: false, Reason: RateLimitBudgetExhaustedReason}
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, evaluatorCalls)
	assert.Equal(t, RateLimitBudgetExhaustedReason, out.Report.Error)
}

type attemptStore struct {
	events              []bead.BeadEvent
	closedSHA           string
	unclaimed           bool
	cooldownStatus      string
	lifecycleStatus     string
	mutatedBead         bead.Bead
	noChangesCountCalls int
}

func (s *attemptStore) AppendEvent(beadID string, ev bead.BeadEvent) error {
	s.events = append(s.events, ev)
	return nil
}

func (s *attemptStore) CloseWithEvidence(beadID, sessionID, sha string) error {
	s.closedSHA = sha
	return nil
}

func (s *attemptStore) Unclaim(beadID string) error {
	s.unclaimed = true
	return nil
}

func (s *attemptStore) SetExecutionCooldown(beadID string, until time.Time, status, detail, baseRev string) error {
	s.cooldownStatus = status
	return nil
}

func (s *attemptStore) UpdateWithLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error {
	s.lifecycleStatus = status
	b := &bead.Bead{ID: id, Status: bead.StatusOpen}
	if mutate != nil {
		if err := mutate(b); err != nil {
			return err
		}
	}
	s.mutatedBead = *b
	return nil
}

func (s *attemptStore) IncrNoChangesCount(beadID string) (int, error) {
	s.noChangesCountCalls++
	return s.noChangesCountCalls, nil
}

// beadStoreAdapter wraps *bead.Store to satisfy the try.Store interface,
// delegating UpdateWithLifecycleStatus to the real store and stubbing unused methods.
type beadStoreAdapter struct {
	s *bead.Store
}

func (a *beadStoreAdapter) AppendEvent(_ string, _ bead.BeadEvent) error { return nil }
func (a *beadStoreAdapter) CloseWithEvidence(_, _, _ string) error       { return nil }
func (a *beadStoreAdapter) Unclaim(_ string) error                       { return nil }
func (a *beadStoreAdapter) SetExecutionCooldown(_ string, _ time.Time, _, _, _ string) error {
	return nil
}
func (a *beadStoreAdapter) IncrNoChangesCount(_ string) (int, error) { return 0, nil }
func (a *beadStoreAdapter) UpdateWithLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error {
	return a.s.UpdateWithLifecycleStatus(id, status, opts, mutate)
}

package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/triage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsLockContentionError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"Unable to create '.git/index.lock': File exists.", true},
		{"fatal: another git process seems to be running in this repository", true},
		{".git/index.lock: file exists", true},
		{"staging-tracker lock held by another process", true},
		{"tracker lock held: try again later", true},
		{"build failed: undefined: foo.Bar", false},
		{"401 unauthorized", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.msg, func(t *testing.T) {
			if got := IsLockContentionError(tc.msg); got != tc.want {
				t.Fatalf("IsLockContentionError(%q)=%v want %v", tc.msg, got, tc.want)
			}
		})
	}
}

func TestClassifyAttemptForTriage(t *testing.T) {
	cases := []struct {
		name string
		rep  ExecuteBeadReport
		want triage.FailureMode
	}{
		{"review_block",
			ExecuteBeadReport{Status: ExecuteBeadStatusReviewBlock},
			triage.FailureModeReviewBlock},
		{"no_changes",
			ExecuteBeadReport{Status: ExecuteBeadStatusNoChanges},
			triage.FailureModeNoChanges},
		{"execution_failed_lock",
			ExecuteBeadReport{Status: ExecuteBeadStatusExecutionFailed,
				Detail: "Unable to create '.git/index.lock'"},
			triage.FailureModeLockContention},
		{"execution_failed_other",
			ExecuteBeadReport{Status: ExecuteBeadStatusExecutionFailed,
				Detail: "agent crashed"},
			triage.FailureModeExecutionFailed},
		{"success",
			ExecuteBeadReport{Status: ExecuteBeadStatusSuccess},
			triage.FailureMode("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyAttemptForTriage(tc.rep)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestRationaleCitesAC(t *testing.T) {
	ac := "1. The retry-with-backoff dispatcher must escalate after exhaustion. " +
		"2. The TriagePolicy ladder must advance per attempt."
	cases := []struct {
		name      string
		rationale string
		want      bool
	}{
		{"empty", "", false},
		{"vague", "nothing to do here", false},
		{"cites two AC tokens",
			"the dispatcher already has retry-with-backoff and the triagepolicy ladder is in place; see TestX",
			true},
		{"cites SHA",
			"already done in 4fde2b25, see commit log", true},
		{"cites Go test name",
			"see TestRetryWithBackoff in dispatcher_test.go", true},
		{"only one AC token",
			"backoff was implemented", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RationaleCitesAC(tc.rationale, ac)
			if got != tc.want {
				t.Fatalf("RationaleCitesAC(%q)=%v want %v", tc.rationale, got, tc.want)
			}
		})
	}
}

// TestRunWithLockBackoffSucceedsOnRetry — AC#4: lock-error retry success.
// First attempt fails with .git/index.lock; the dispatcher waits the first
// backoff slot and the second attempt succeeds. The schedule's later slots
// are not consumed.
func TestRunWithLockBackoffSucceedsOnRetry(t *testing.T) {
	calls := 0
	attempt := func(ctx context.Context) (ExecuteBeadReport, error) {
		calls++
		if calls == 1 {
			return ExecuteBeadReport{
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: "Unable to create '.git/index.lock': File exists.",
			}, nil
		}
		return ExecuteBeadReport{Status: ExecuteBeadStatusSuccess, ResultRev: "abc"}, nil
	}
	var slept []time.Duration
	sleep := func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	backoff := []time.Duration{30 * time.Second, 90 * time.Second, 300 * time.Second}

	res := RunWithLockBackoff(context.Background(), attempt, backoff, sleep)

	assert.Equal(t, 2, calls, "second attempt must succeed")
	assert.Equal(t, 1, res.Retries, "exactly one retry consumed")
	assert.False(t, res.Exhausted)
	assert.Equal(t, ExecuteBeadStatusSuccess, res.Report.Status)
	assert.Equal(t, []time.Duration{30 * time.Second}, slept,
		"only the first backoff slot was consumed")
}

// TestRunWithLockBackoffExhausts — AC#4: lock-error backoff exhaustion.
// Three retry slots, four lock-contention attempts in total → exhausted.
// All three backoff slots are slept in order before final escalation.
func TestRunWithLockBackoffExhausts(t *testing.T) {
	attempt := func(ctx context.Context) (ExecuteBeadReport, error) {
		return ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed,
			Detail: "Unable to create '.git/index.lock': File exists.",
		}, nil
	}
	var slept []time.Duration
	sleep := func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	backoff := []time.Duration{30 * time.Second, 90 * time.Second, 300 * time.Second}

	res := RunWithLockBackoff(context.Background(), attempt, backoff, sleep)

	assert.Equal(t, 3, res.Retries, "three retries against the schedule")
	assert.True(t, res.Exhausted, "schedule must be exhausted")
	assert.Equal(t, backoff, slept, "all three backoff slots consumed in order")
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, res.Report.Status)
}

// TestRunWithLockBackoffPropagatesNonLockError — non-lock errors must NOT
// trigger retry. The dispatcher must return immediately with the first
// attempt's outcome.
func TestRunWithLockBackoffPropagatesNonLockError(t *testing.T) {
	calls := 0
	attempt := func(ctx context.Context) (ExecuteBeadReport, error) {
		calls++
		return ExecuteBeadReport{
			Status: ExecuteBeadStatusExecutionFailed,
			Detail: "401 unauthorized",
		}, nil
	}
	res := RunWithLockBackoff(context.Background(), attempt, LockContentionBackoff,
		func(_ context.Context, _ time.Duration) error { return nil })
	assert.Equal(t, 1, calls)
	assert.Equal(t, 0, res.Retries)
	assert.False(t, res.Exhausted)
	assert.Empty(t, res.SleepDurations)
}

// TestRunWithLockBackoffRetriesOnAttemptError — when the AttemptFunc itself
// returns an error whose text matches the lock pattern, the dispatcher
// should retry too. This covers callers who surface lock contention as a
// returned error rather than a populated report.
func TestRunWithLockBackoffRetriesOnAttemptError(t *testing.T) {
	calls := 0
	attempt := func(ctx context.Context) (ExecuteBeadReport, error) {
		calls++
		if calls == 1 {
			return ExecuteBeadReport{}, errors.New("staging-tracker lock held: retry")
		}
		return ExecuteBeadReport{Status: ExecuteBeadStatusSuccess}, nil
	}
	res := RunWithLockBackoff(context.Background(), attempt,
		[]time.Duration{1 * time.Millisecond},
		func(_ context.Context, _ time.Duration) error { return nil })
	assert.Equal(t, 2, calls)
	assert.Equal(t, 1, res.Retries)
	assert.False(t, res.Exhausted)
}

// TestDispatchExecutionFailedEscalates — AC#4: execution_failed escalation.
// First failure → re_attempt_with_context. Second → escalate_tier. Third
// → needs_human. Each call records a triage-decision event so future Decide()
// calls see the prior history.
func TestDispatchExecutionFailedEscalates(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	parent := &bead.Bead{Title: "Failing thing", Acceptance: "AC text"}
	require.NoError(t, store.Create(parent))

	policy := triage.DefaultPolicy()
	now := func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC) }

	dispatch := func() triage.Action {
		b, err := store.Get(parent.ID)
		require.NoError(t, err)
		out, err := DispatchPostAttempt(store, policy, DispatchInput{
			Bead: *b,
			Mode: triage.FailureModeExecutionFailed,
			Now:  now,
		})
		require.NoError(t, err)
		return out.Action
	}

	wantLadder := []triage.Action{
		triage.ActionReAttemptWithContext,
		triage.ActionEscalateTier,
		triage.ActionNeedsHuman,
		triage.ActionNeedsHuman, // clamps to last rung
	}
	for i, want := range wantLadder {
		got := dispatch()
		if got != want {
			t.Fatalf("attempt %d: got %q want %q", i+1, got, want)
		}
	}

	// The ladder advances were materialised as triage-decision events.
	events, err := store.Events(parent.ID)
	require.NoError(t, err)
	var decisions []bead.BeadEvent
	for _, ev := range events {
		if ev.Kind == "triage-decision" {
			decisions = append(decisions, ev)
		}
	}
	assert.Len(t, decisions, len(wantLadder))

	// HistoryFromEvents recovers the modes in order.
	hist := HistoryFromEvents(events)
	for _, m := range hist {
		assert.Equal(t, triage.FailureModeExecutionFailed, m)
	}
}

// TestDispatchNoChangesFilesFollowup — AC#4: no_changes follow-up.
// When the worker rationale does not cite any AC items, the policy returns
// file_followup on the second occurrence; the dispatcher creates a child
// clarification bead, links the parent, and closes the parent.
func TestDispatchNoChangesFilesFollowup(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	parent := &bead.Bead{
		Title:      "Implement the gizmo",
		Acceptance: "1. The frobnicator must validate inputs. 2. Add a regression test for invalid power.",
	}
	require.NoError(t, store.Create(parent))

	policy := triage.DefaultPolicy()
	now := func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC) }

	// First no_changes attempt: rationale is weak. Default policy ladder rung 1
	// is re_attempt_with_context — no follow-up is filed yet, but a triage
	// decision is recorded so the next call sees the advance.
	pCopy, _ := store.Get(parent.ID)
	out, err := DispatchPostAttempt(store, policy, DispatchInput{
		Bead:      *pCopy,
		Mode:      triage.FailureModeNoChanges,
		Rationale: "nothing left to do here",
		Now:       now,
	})
	require.NoError(t, err)
	assert.Equal(t, triage.ActionReAttemptWithContext, out.Action)
	assert.Empty(t, out.FollowupID)
	assert.False(t, out.ParentClosed)

	// Second no_changes: ladder advances to file_followup. Dispatcher files a
	// clarification child and closes the parent.
	pCopy, _ = store.Get(parent.ID)
	out, err = DispatchPostAttempt(store, policy, DispatchInput{
		Bead:      *pCopy,
		Mode:      triage.FailureModeNoChanges,
		Rationale: "nothing to do, really",
		SessionID: "sess-1",
		BaseRev:   "abc1234",
		Now:       now,
	})
	require.NoError(t, err)
	assert.Equal(t, triage.ActionFileFollowup, out.Action)
	require.NotEmpty(t, out.FollowupID, "follow-up bead must be created")
	assert.True(t, out.ParentClosed, "parent must be closed")

	// Parent is closed.
	got, err := store.Get(parent.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	// Child has parent link + clarification labels + back-reference in Extra.
	child, err := store.Get(out.FollowupID)
	require.NoError(t, err)
	assert.Equal(t, parent.ID, child.Parent)
	assert.True(t, HasBeadLabel(child.Labels, "kind:clarification"))
	assert.Equal(t, parent.ID, child.Extra["triage_followup_for"])
	assert.Equal(t, "no_changes_weak_rationale", child.Extra["triage_followup_reason"])
	assert.Contains(t, child.Description, "nothing to do, really")
	assert.Contains(t, child.Acceptance, parent.ID)

	// triage-followup-filed event references the child.
	events, err := store.Events(parent.ID)
	require.NoError(t, err)
	var followupEv *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "triage-followup-filed" {
			followupEv = &events[i]
		}
	}
	require.NotNil(t, followupEv, "triage-followup-filed event missing")
	assert.Contains(t, followupEv.Body, out.FollowupID)
}

// TestDispatchNoChangesStrongRationaleStaysOnLadder — when the rationale
// cites the AC, this signals the bead is genuinely satisfied; the existing
// adjudicateNoChanges path closes it as already_satisfied. The dispatcher
// must NOT file a clarification follow-up in that case. We verify by
// ensuring callers can keep the dispatcher off the path: the dispatcher only
// fires when the caller has decided weak rationale → triage. When
// RationaleCitesAC returns true, the caller should skip DispatchPostAttempt
// for no_changes entirely. This unit test just confirms RationaleCitesAC
// returns true on a strong rationale containing an AC token.
func TestDispatchNoChangesStrongRationaleSkipsTriage(t *testing.T) {
	ac := "1. The retry-with-backoff dispatcher must escalate after exhaustion."
	rationale := "Already implemented: see retry-with-backoff in TestRunWithLockBackoffExhausts; commit 4fde2b25."
	if !RationaleCitesAC(rationale, ac) {
		t.Fatalf("strong rationale should be recognised as citing AC")
	}
}

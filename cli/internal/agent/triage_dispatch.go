package agent

// Triage dispatch wires post-attempt failure modes to the standalone
// triage.TriagePolicy. It covers three integration paths described by the
// parent bead's AC:
//
//   - lock_contention: classify staging-tracker / .git/index.lock errors
//     and retry the operation with a 3-step backoff before escalating.
//   - execution_failed: consult the policy, which advances escalate_tier →
//     needs_human as attempts accumulate.
//   - no_changes: when the worker's rationale does not cite the bead's AC,
//     file a clarification follow-up bead and close the original.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/triage"
)

// LockContentionBackoff is the default 3-step backoff schedule applied when
// an attempt fails with a lock-contention signal. Total worst-case wait per
// attempt = 30s + 90s + 300s = 7m. Tests override this by passing their own
// schedule into RunWithLockBackoff.
var LockContentionBackoff = []time.Duration{
	30 * time.Second,
	90 * time.Second,
	300 * time.Second,
}

// IsLockContentionError reports whether errMsg names a transient lock-acquire
// failure that the dispatcher should retry. Patterns cover both git's index
// lock and ddx's own staging/tracker locks.
func IsLockContentionError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return containsAny(lower,
		".git/index.lock",
		"unable to create '.git/index.lock'",
		"another git process seems to be running",
		"index.lock: file exists",
		"staging-tracker lock",
		"tracker lock held",
		"tracker is locked",
	)
}

// ClassifyAttemptForTriage maps an ExecuteBeadReport to the triage.FailureMode
// that the dispatcher should consult. Returns "" when the report does not
// belong to any of the modes this bead wires (success, push_failed, etc.).
func ClassifyAttemptForTriage(report ExecuteBeadReport) triage.FailureMode {
	switch report.Status {
	case ExecuteBeadStatusReviewBlock:
		return triage.FailureModeReviewBlock
	case ExecuteBeadStatusNoChanges:
		return triage.FailureModeNoChanges
	case ExecuteBeadStatusExecutionFailed:
		if IsLockContentionError(report.Detail) {
			return triage.FailureModeLockContention
		}
		return triage.FailureModeExecutionFailed
	}
	return ""
}

// RationaleCitesAC returns true when a no-changes rationale references the
// bead's acceptance criteria with enough specificity to treat the bead as
// already satisfied. The check passes when:
//   - rationaleIsSpecific (already used by adjudicateNoChanges) holds, OR
//   - the rationale shares at least 2 distinct content tokens (lower-cased
//     alphanumeric, length ≥ 6) with the acceptance text.
//
// A vague rationale ("nothing to do", "already done") fails both checks and
// is treated as a clarification request, prompting a follow-up bead.
func RationaleCitesAC(rationale, acceptance string) bool {
	if strings.TrimSpace(rationale) == "" {
		return false
	}
	if rationaleIsSpecific(rationale) {
		return true
	}
	rl := strings.ToLower(rationale)
	matches := 0
	seen := map[string]bool{}
	for _, t := range tokenizeRationale(acceptance) {
		if seen[t] {
			continue
		}
		seen[t] = true
		if strings.Contains(rl, t) {
			matches++
			if matches >= 2 {
				return true
			}
		}
	}
	return false
}

// tokenizeRationale extracts lower-cased alphanumeric tokens of length ≥ 6
// for the AC-overlap check. The threshold is higher than the AC-coverage
// metric in triage.go because a rationale citing AC items typically reuses
// a noun or verb from the AC ("test", "rate", "limit" are too generic).
func tokenizeRationale(s string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			if cur.Len() >= 6 {
				tokens = append(tokens, cur.String())
			}
			cur.Reset()
		}
	}
	if cur.Len() >= 6 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// AttemptFunc runs one execute-bead attempt and returns its report.
type AttemptFunc func(ctx context.Context) (ExecuteBeadReport, error)

// SleepFunc abstracts time.Sleep so tests can inject a fake clock.
type SleepFunc func(ctx context.Context, d time.Duration) error

// DefaultSleep is a context-aware time.Sleep used as the default SleepFunc.
func DefaultSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// LockBackoffResult reports the outcome of RunWithLockBackoff.
type LockBackoffResult struct {
	// Report is the final attempt's report (success or last failed retry).
	Report ExecuteBeadReport
	// Err is the final attempt's error, if any.
	Err error
	// Retries is the number of retry attempts performed *after* the initial
	// run. 0 means the first run did not see a lock-contention error.
	Retries int
	// Exhausted is true when every retry slot in the backoff schedule was
	// consumed and the final attempt still reported lock_contention. The
	// caller should escalate via the triage policy.
	Exhausted bool
	// SleepDurations records each backoff that was waited, in order. Tests
	// assert on this to verify the schedule was honoured.
	SleepDurations []time.Duration
}

// RunWithLockBackoff invokes attempt and, when it returns a lock-contention
// failure, retries up to len(backoff) times with the matching backoff between
// attempts. Returns as soon as an attempt produces any non-lock-contention
// outcome (success or other failure). When every retry is consumed and the
// final attempt is still lock_contention, Exhausted is true.
func RunWithLockBackoff(ctx context.Context, attempt AttemptFunc, backoff []time.Duration, sleep SleepFunc) LockBackoffResult {
	if sleep == nil {
		sleep = DefaultSleep
	}
	res := LockBackoffResult{}
	report, err := attempt(ctx)
	res.Report = report
	res.Err = err
	for i, d := range backoff {
		if !attemptIsLockContention(res.Report, res.Err) {
			return res
		}
		if serr := sleep(ctx, d); serr != nil {
			// Ctx cancelled mid-backoff: surface ctx error, exhaust flag stays
			// false because we did not finish the schedule.
			res.Err = serr
			return res
		}
		res.SleepDurations = append(res.SleepDurations, d)
		report, err = attempt(ctx)
		res.Report = report
		res.Err = err
		res.Retries = i + 1
	}
	if attemptIsLockContention(res.Report, res.Err) {
		res.Exhausted = true
	}
	return res
}

func attemptIsLockContention(report ExecuteBeadReport, err error) bool {
	if err != nil && IsLockContentionError(err.Error()) {
		return true
	}
	if report.Status == ExecuteBeadStatusExecutionFailed && IsLockContentionError(report.Detail) {
		return true
	}
	return false
}

// TriageDispatchStore is the subset of bead.Store used by the dispatcher.
// *bead.Store satisfies it (compile-time check below).
type TriageDispatchStore interface {
	Create(b *bead.Bead) error
	Update(id string, mutate func(*bead.Bead)) error
	AppendEvent(id string, event bead.BeadEvent) error
	Events(id string) ([]bead.BeadEvent, error)
	CloseWithEvidence(id, sessionID, commitSHA string) error
}

// HistoryFromEvents extracts the ordered list of prior triage failure modes
// for a bead from its event log. The dispatcher records each decision as a
// `triage-decision` event whose body carries `mode=<failure_mode>`. Older
// decisions appear first.
func HistoryFromEvents(events []bead.BeadEvent) []triage.FailureMode {
	var hist []triage.FailureMode
	for _, ev := range events {
		if ev.Kind != "triage-decision" {
			continue
		}
		mode := extractTriageEventField(ev.Body, "mode")
		if mode == "" {
			continue
		}
		hist = append(hist, triage.FailureMode(mode))
	}
	return hist
}

func extractTriageEventField(body, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// DispatchInput carries the per-attempt context for a triage decision.
type DispatchInput struct {
	Bead      bead.Bead
	Mode      triage.FailureMode
	Rationale string // for no_changes
	SessionID string // for closing the parent on file_followup
	BaseRev   string
	Now       func() time.Time
}

// DispatchOutcome is the result of DispatchPostAttempt. Action is what the
// triage policy returned; FollowupID is set when ActionFileFollowup created
// a child bead. ParentClosed is true when the original bead was closed as
// part of the followup-file action.
type DispatchOutcome struct {
	Action       triage.Action
	FollowupID   string
	ParentClosed bool
}

// DispatchPostAttempt is the central entry point invoked by the loop after
// an attempt classifies into a triage-tracked failure mode. It records a
// `triage-decision` event with the chosen action and, when the action calls
// for it, files a clarification follow-up bead and closes the parent.
//
// Lock-contention retry is NOT performed here — callers wrap the executor
// with RunWithLockBackoff before invoking DispatchPostAttempt. By the time
// this function runs the in-pass retry schedule is already exhausted.
func DispatchPostAttempt(store TriageDispatchStore, policy triage.TriagePolicy, in DispatchInput) (DispatchOutcome, error) {
	if in.Now == nil {
		in.Now = time.Now
	}
	events, err := store.Events(in.Bead.ID)
	if err != nil {
		return DispatchOutcome{}, fmt.Errorf("triage dispatch: read events: %w", err)
	}
	history := HistoryFromEvents(events)
	action := policy.Decide(in.Bead.ID, in.Mode, history)

	out := DispatchOutcome{Action: action}

	// Record the triage decision event before any further side effects so
	// future Decide() calls see the advance even if the followup-file path
	// fails partway through.
	body, _ := json.Marshal(map[string]any{
		"mode":      string(in.Mode),
		"action":    string(action),
		"history":   history,
		"rationale": in.Rationale,
	})
	bodyStr := fmt.Sprintf("mode=%s\naction=%s\n\n%s", in.Mode, action, string(body))
	if err := store.AppendEvent(in.Bead.ID, bead.BeadEvent{
		Kind:      "triage-decision",
		Summary:   fmt.Sprintf("%s → %s", in.Mode, action),
		Body:      bodyStr,
		Actor:     "ddx",
		Source:    "ddx triage dispatch",
		CreatedAt: in.Now().UTC(),
	}); err != nil {
		return out, fmt.Errorf("triage dispatch: append event: %w", err)
	}

	if action == triage.ActionFileFollowup && in.Mode == triage.FailureModeNoChanges {
		childID, ferr := fileNoChangesFollowup(store, in)
		if ferr != nil {
			return out, ferr
		}
		out.FollowupID = childID
		// Close the parent: the no_changes outcome means the agent claims
		// nothing to do, but the rationale is too weak to call the AC met.
		// The follow-up bead carries the clarification work; this one is
		// closed without a commit (BaseRev is the closing SHA).
		closingSHA := in.BaseRev
		if err := store.CloseWithEvidence(in.Bead.ID, in.SessionID, closingSHA); err != nil {
			return out, fmt.Errorf("triage dispatch: close parent: %w", err)
		}
		out.ParentClosed = true
	}
	return out, nil
}

// fileNoChangesFollowup creates a child clarification bead. The child
// inherits the parent's spec-id and labels, and its description quotes the
// weak rationale and the parent AC so a future attempt has enough context
// to either redo the work or confirm satisfaction.
func fileNoChangesFollowup(store TriageDispatchStore, in DispatchInput) (string, error) {
	parent := in.Bead
	rationale := strings.TrimSpace(in.Rationale)
	if rationale == "" {
		rationale = "(agent provided no rationale)"
	}

	desc := fmt.Sprintf(
		"Clarify acceptance criteria for parent bead %s.\n\n"+
			"The agent reported no_changes with a rationale that does not cite the parent's acceptance criteria. "+
			"The original bead has been closed; this follow-up captures the unresolved question so a human or "+
			"a stronger model can either confirm satisfaction with concrete evidence or do the work.\n\n"+
			"Worker rationale:\n%s\n\n"+
			"Parent acceptance criteria:\n%s",
		parent.ID, rationale, strings.TrimSpace(parent.Acceptance))

	acceptance := fmt.Sprintf(
		"1. Confirm whether each AC item from %s is genuinely satisfied. "+
			"For each item, either cite a commit SHA / test name proving it, or do the work and commit it. "+
			"2. If satisfied, close with evidence; if not, the implementation lands here.",
		parent.ID)

	labels := append([]string{}, parent.Labels...)
	if !HasBeadLabel(labels, "kind:clarification") {
		labels = append(labels, "kind:clarification")
	}

	extra := map[string]any{
		"triage_followup_for":     parent.ID,
		"triage_followup_reason":  "no_changes_weak_rationale",
		"triage_parent_rationale": rationale,
	}
	if parent.Extra != nil {
		if specID, ok := parent.Extra["spec-id"]; ok && specID != "" {
			extra["spec-id"] = specID
		}
	}

	child := &bead.Bead{
		Title:       "Clarify: " + parent.Title,
		Description: desc,
		Acceptance:  acceptance,
		Labels:      labels,
		Parent:      parent.ID,
		Extra:       extra,
	}
	if err := store.Create(child); err != nil {
		return "", fmt.Errorf("triage dispatch: file followup bead: %w", err)
	}

	// Cross-link the parent so the closure carries a pointer to the
	// followup bead's id. AppendEvent on the parent is best-effort; failure
	// here does not abort the dispatch because the child is already filed.
	body, _ := json.Marshal(map[string]any{
		"followup_id": child.ID,
		"reason":      "no_changes_weak_rationale",
		"rationale":   rationale,
	})
	_ = store.AppendEvent(parent.ID, bead.BeadEvent{
		Kind:      "triage-followup-filed",
		Summary:   fmt.Sprintf("filed clarification follow-up %s", child.ID),
		Body:      string(body),
		Actor:     "ddx",
		Source:    "ddx triage dispatch",
		CreatedAt: in.Now().UTC(),
	})
	return child.ID, nil
}

// Compile-time check: bead.Store satisfies TriageDispatchStore.
var _ TriageDispatchStore = (*bead.Store)(nil)

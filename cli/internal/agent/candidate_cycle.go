package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// CandidateResult is the outcome of a single ImplementationPass inside a live
// worktree. The coordinator owns the worktree lifetime; callers must not remove
// WorktreePath directly.
type CandidateResult struct {
	// Report is the full execute-bead report from the implementation pass.
	Report ExecuteBeadReport
	// WorktreePath is the live worktree path, valid until the coordinator
	// disposes it after reaching terminal disposition.
	WorktreePath string
	// CycleIndex is the zero-based repair-cycle index for this candidate.
	CycleIndex int
}

// ImplementationPass runs the agent in an isolated worktree and returns a
// CandidateResult. It does not own terminal cleanup — the AttemptCycleCoordinator
// is responsible for worktree removal after disposition.
type ImplementationPass interface {
	Execute(ctx context.Context, beadID string) (CandidateResult, error)
}

// CandidateRefStore pins and unpins candidate git refs while the coordinator
// evaluates them. Refs survive executor cleanup so the landing gate can check
// out the candidate commit without racing concurrent garbage collection.
//
// Ref naming convention: refs/ddx/iterations/<attemptID>/<cycleIndex>
type CandidateRefStore interface {
	PinCandidateRef(projectRoot, attemptID string, cycleIndex int, rev string) (ref string, err error)
	UnpinCandidateRef(projectRoot, ref string) error
}

// CandidateCheckResult carries the outcome of a pre-land quality check.
type CandidateCheckResult struct {
	Passed    bool
	Detail    string
	Artifacts []string
}

// CandidateCheckRunner runs pre-land quality checks against a candidate.
type CandidateCheckRunner interface {
	RunChecks(ctx context.Context, projectRoot string, candidate CandidateResult) (CandidateCheckResult, error)
}

// CandidateReviewResult carries a reviewer verdict for a committed candidate.
type CandidateReviewResult struct {
	// Verdict is one of "APPROVE", "REQUEST_CHANGES", or "BLOCK".
	Verdict        string
	Rationale      string
	PerAC          []ReviewAC
	Findings       []Finding
	Classification string
}

// CandidateReviewer runs a read-only review pass against a committed candidate
// inside the still-live worktree. Reserved for future candidate-cycle review
// implementation (non-scope for FEAT-010 initial pass).
type CandidateReviewer interface {
	Review(ctx context.Context, projectRoot string, candidate CandidateResult) (CandidateReviewResult, error)
}

// RepairPass attempts to repair a candidate in the still-live worktree based on
// review findings, returning an updated CandidateResult. Reserved for future
// repair-loop implementation (non-scope for FEAT-010 initial pass).
type RepairPass interface {
	Repair(ctx context.Context, candidate CandidateResult, findings string) (CandidateResult, error)
}

// CandidateLander lands an approved candidate to the main branch.
type CandidateLander interface {
	Land(ctx context.Context, candidate CandidateResult) (ExecuteBeadReport, error)
}

// candidateIterationRef returns the ref name for a pinned candidate cycle.
// Format: refs/ddx/iterations/<attemptID>/<cycleIndex>
func candidateIterationRef(attemptID string, cycleIndex int) string {
	return fmt.Sprintf("refs/ddx/iterations/%s/%d", attemptID, cycleIndex)
}

// GitCandidateRefStore is the production CandidateRefStore implementation.
// It pins and unpins candidate refs in the project root via git update-ref so
// they remain accessible after the temporary implementation worktree is removed.
type GitCandidateRefStore struct{}

func (s *GitCandidateRefStore) PinCandidateRef(projectRoot, attemptID string, cycleIndex int, rev string) (string, error) {
	ref := candidateIterationRef(attemptID, cycleIndex)
	out, err := internalgit.Command(context.Background(), projectRoot, "update-ref", ref, rev).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pin candidate ref %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return ref, nil
}

func (s *GitCandidateRefStore) UnpinCandidateRef(projectRoot, ref string) error {
	out, err := internalgit.Command(context.Background(), projectRoot, "update-ref", "-d", ref).CombinedOutput()
	if err != nil {
		return fmt.Errorf("unpin candidate ref %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// CandidateCycleEventBody is the structured payload of a candidate_cycle_pinned bead event.
type CandidateCycleEventBody struct {
	CandidateRef string `json:"candidate_ref"`
	CycleIndex   int    `json:"cycle_index"`
	AttemptID    string `json:"attempt_id"`
	BaseRev      string `json:"base_rev"`
	ResultRev    string `json:"result_rev"`
}

const candidateChecksFailedEventKind = "candidate-checks-failed"
const candidateReviewClassifiedEventKind = "candidate-review-classified"

// CandidateChecksFailedEventBody is the structured payload of a
// candidate-checks-failed bead event.
type CandidateChecksFailedEventBody struct {
	CandidateRef string   `json:"candidate_ref,omitempty"`
	CycleIndex   int      `json:"cycle_index"`
	AttemptID    string   `json:"attempt_id,omitempty"`
	BaseRev      string   `json:"base_rev,omitempty"`
	ResultRev    string   `json:"result_rev,omitempty"`
	Detail       string   `json:"detail,omitempty"`
	Artifacts    []string `json:"artifacts,omitempty"`
}

// CandidateReviewClassifiedEventBody is the structured payload of a
// candidate-review-classified bead event.
type CandidateReviewClassifiedEventBody struct {
	CandidateRef   string `json:"candidate_ref,omitempty"`
	CycleIndex     int    `json:"cycle_index"`
	AttemptID      string `json:"attempt_id,omitempty"`
	BaseRev        string `json:"base_rev,omitempty"`
	ResultRev      string `json:"result_rev,omitempty"`
	Verdict        string `json:"verdict,omitempty"`
	Classification string `json:"classification,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// ShouldRetainCandidateRef returns true when the attempt outcome requires
// keeping the candidate ref for operator inspection and recovery.
// Only a successfully landed (merged) candidate allows the temporary ref to be
// cleaned up; preserved, conflicted, parked, and manual candidates retain it.
func ShouldRetainCandidateRef(status string) bool {
	return status != ExecuteBeadStatusSuccess
}

// AttemptCycleResult is the outcome of AttemptCycleCoordinator.Run.
type AttemptCycleResult struct {
	// Report is the final execute-bead report after all cycle phases complete.
	Report ExecuteBeadReport
	// Landed is true when the candidate was merged to the main branch.
	Landed bool
}

// AttemptCycleCoordinator owns the full live-worktree lifecycle for a single
// implementation attempt: implementation → optional checks → optional review →
// optional repair → land. It ensures terminal disposition is reached before
// the coordinator exits, regardless of which phase terminates the cycle.
//
// Pass and Lander are required. Nil optional fields (Checks, Reviewer, Repair,
// RefStore) skip their respective phases.
//
// Repair loops are reserved for future implementation. When Reviewer is non-nil
// the coordinator runs one pre-land candidate review before landing.
type AttemptCycleCoordinator struct {
	Pass        ImplementationPass
	Checks      CandidateCheckRunner // nil → skip pre-land checks
	Reviewer    CandidateReviewer    // nil → skip candidate review (future)
	Repair      RepairPass           // nil → skip repair loops (future)
	Lander      CandidateLander
	RefStore    CandidateRefStore // nil → no ref pinning
	ProjectRoot string            // required when RefStore non-nil
	BeadEvents  BeadEventAppender // nil → no candidate-cycle event emission
}

// Run executes one attempt cycle: implementation → checks → land. Non-success
// statuses (no_changes, execution_failed, etc.) return without calling the
// lander.
// The caller is responsible for worktree cleanup after Run returns.
//
// When RefStore and ProjectRoot are set, Run pins a candidate ref under
// refs/ddx/iterations/<attempt-id>/<cycle-index> before checks and review.
// After a successful land the temporary ref is cleaned up; for preserved,
// conflicted, parked, or otherwise non-landed outcomes the ref is retained so
// operators can inspect and recover the candidate.
func (c *AttemptCycleCoordinator) Run(ctx context.Context, beadID string) (AttemptCycleResult, error) {
	candidate, err := c.Pass.Execute(ctx, beadID)
	if err != nil {
		return AttemptCycleResult{Report: candidate.Report}, err
	}

	if candidate.Report.Status != ExecuteBeadStatusSuccess {
		return AttemptCycleResult{Report: candidate.Report}, nil
	}

	// Pin candidate ref in project root before checks and read-only review so
	// the commit remains accessible even after the temp worktree is removed.
	if c.RefStore != nil && c.ProjectRoot != "" {
		ref, pinErr := c.RefStore.PinCandidateRef(
			c.ProjectRoot,
			candidate.Report.AttemptID,
			candidate.CycleIndex,
			candidate.Report.ResultRev,
		)
		if pinErr == nil {
			candidate.Report.CandidateRef = ref
			candidate.Report.CycleIndex = candidate.CycleIndex
			if c.BeadEvents != nil {
				body, _ := json.Marshal(CandidateCycleEventBody{
					CandidateRef: ref,
					CycleIndex:   candidate.CycleIndex,
					AttemptID:    candidate.Report.AttemptID,
					BaseRev:      candidate.Report.BaseRev,
					ResultRev:    candidate.Report.ResultRev,
				})
				_ = c.BeadEvents.AppendEvent(beadID, bead.BeadEvent{
					Kind: "candidate_cycle_pinned",
					Body: string(body),
				})
			}
		}
	}

	if c.Checks != nil {
		checksResult, checksErr := c.Checks.RunChecks(ctx, c.ProjectRoot, candidate)
		if checksErr != nil {
			checksResult.Detail = checksErr.Error()
		}
		if checksErr != nil || !checksResult.Passed {
			report := candidate.Report
			if checksErr != nil {
				report.Status = ExecuteBeadStatusExecutionFailed
			} else {
				report.Status = ExecuteBeadStatusPostRunCheckFailed
			}
			report.OutcomeReason = candidateChecksFailedEventKind
			report.Detail = candidateChecksFailedDetail(checksResult.Detail)
			if checksErr != nil && report.Error == "" {
				report.Error = checksErr.Error()
			}
			c.appendCandidateChecksFailedEvent(beadID, report, checksResult)
			return AttemptCycleResult{Report: report}, nil
		}
	}

	if c.Reviewer != nil {
		reviewResult, reviewErr := c.Reviewer.Review(ctx, c.ProjectRoot, candidate)
		if reviewErr != nil {
			report := candidate.Report
			report.Status = ExecuteBeadStatusReviewMalfunction
			report.Detail = "pre-land review: " + reviewErr.Error()
			report.ReviewVerdict = strings.TrimSpace(reviewResult.Verdict)
			report.ReviewRationale = strings.TrimSpace(reviewResult.Rationale)
			c.appendCandidateReviewClassifiedEvent(beadID, report, reviewResult, ReviewFindingClassMalfunction, reviewErr.Error())
			return AttemptCycleResult{Report: report}, nil
		}
		verdict := Verdict(strings.TrimSpace(reviewResult.Verdict))
		switch verdict {
		case VerdictApprove:
			candidate.Report.ReviewVerdict = string(VerdictApprove)
			candidate.Report.ReviewRationale = strings.TrimSpace(reviewResult.Rationale)
		case VerdictRequestChanges:
			classification := c.classifyCandidateReview(reviewResult)
			report := candidate.Report
			report.Status = ExecuteBeadStatusReviewRequestChanges
			report.Detail = "pre-land review: REQUEST_CHANGES"
			report.ReviewVerdict = string(VerdictRequestChanges)
			report.ReviewRationale = strings.TrimSpace(reviewResult.Rationale)
			c.appendCandidateReviewClassifiedEvent(beadID, report, reviewResult, classification.Class, classification.Reason)
			return AttemptCycleResult{Report: report}, nil
		case VerdictBlock:
			classification := c.classifyCandidateReview(reviewResult)
			report := candidate.Report
			report.Status = ExecuteBeadStatusReviewBlock
			report.Detail = "pre-land review: BLOCK"
			report.ReviewVerdict = string(VerdictBlock)
			report.ReviewRationale = strings.TrimSpace(reviewResult.Rationale)
			if classification.Class == ReviewFindingClassMalfunction {
				report.Status = ExecuteBeadStatusReviewMalfunction
				report.Detail = "pre-land review: " + ReviewFindingClassMalfunction
			}
			c.appendCandidateReviewClassifiedEvent(beadID, report, reviewResult, classification.Class, classification.Reason)
			return AttemptCycleResult{Report: report}, nil
		default:
			report := candidate.Report
			report.Status = ExecuteBeadStatusReviewMalfunction
			report.Detail = "pre-land review: malformed verdict"
			report.ReviewVerdict = strings.TrimSpace(reviewResult.Verdict)
			report.ReviewRationale = strings.TrimSpace(reviewResult.Rationale)
			c.appendCandidateReviewClassifiedEvent(beadID, report, reviewResult, ReviewFindingClassMalfunction, "malformed review verdict")
			return AttemptCycleResult{Report: report}, nil
		}
	}

	landed, err := c.Lander.Land(ctx, candidate)
	if err != nil {
		return AttemptCycleResult{Report: candidate.Report}, err
	}

	// Propagate candidate metadata to the landed report when the lander does
	// not set them (e.g. in tests that return candidate.Report directly).
	if landed.CandidateRef == "" && candidate.Report.CandidateRef != "" {
		landed.CandidateRef = candidate.Report.CandidateRef
		landed.CycleIndex = candidate.Report.CycleIndex
	}

	result := AttemptCycleResult{Report: landed, Landed: true}

	// Clean up the candidate ref after successful landing. For any non-success
	// outcome (preserved, conflicted, parked, manual) the ref is retained so
	// operators can inspect the candidate commit after the worktree is removed.
	if c.RefStore != nil && c.ProjectRoot != "" && landed.CandidateRef != "" {
		if !ShouldRetainCandidateRef(landed.Status) {
			_ = c.RefStore.UnpinCandidateRef(c.ProjectRoot, landed.CandidateRef)
		}
	}

	return result, nil
}

func candidateChecksFailedDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return candidateChecksFailedEventKind
	}
	return candidateChecksFailedEventKind + ": " + detail
}

func (c *AttemptCycleCoordinator) appendCandidateChecksFailedEvent(beadID string, report ExecuteBeadReport, checksResult CandidateCheckResult) {
	if c.BeadEvents == nil {
		return
	}
	body, _ := json.Marshal(CandidateChecksFailedEventBody{
		CandidateRef: report.CandidateRef,
		CycleIndex:   report.CycleIndex,
		AttemptID:    report.AttemptID,
		BaseRev:      report.BaseRev,
		ResultRev:    report.ResultRev,
		Detail:       strings.TrimSpace(checksResult.Detail),
		Artifacts:    append([]string(nil), checksResult.Artifacts...),
	})
	_ = c.BeadEvents.AppendEvent(beadID, bead.BeadEvent{
		Kind:    candidateChecksFailedEventKind,
		Summary: report.Detail,
		Body:    string(body),
	})
}

func (c *AttemptCycleCoordinator) classifyCandidateReview(review CandidateReviewResult) ReviewFindingClassification {
	if review.Classification != "" {
		return reviewClassification(review.Classification, nil, "", review.Classification == ReviewFindingClassFixableGap)
	}
	return ClassifyReviewFindings(&ReviewResult{
		Verdict:   Verdict(strings.TrimSpace(review.Verdict)),
		Rationale: strings.TrimSpace(review.Rationale),
		PerAC:     append([]ReviewAC(nil), review.PerAC...),
		Findings:  append([]Finding(nil), review.Findings...),
	})
}

func (c *AttemptCycleCoordinator) appendCandidateReviewClassifiedEvent(beadID string, report ExecuteBeadReport, review CandidateReviewResult, class, reason string) {
	if c.BeadEvents == nil {
		return
	}
	body, _ := json.Marshal(CandidateReviewClassifiedEventBody{
		CandidateRef:   report.CandidateRef,
		CycleIndex:     report.CycleIndex,
		AttemptID:      report.AttemptID,
		BaseRev:        report.BaseRev,
		ResultRev:      report.ResultRev,
		Verdict:        strings.TrimSpace(review.Verdict),
		Classification: strings.TrimSpace(class),
		Reason:         strings.TrimSpace(reason),
	})
	_ = c.BeadEvents.AppendEvent(beadID, bead.BeadEvent{
		Kind:    candidateReviewClassifiedEventKind,
		Summary: strings.TrimSpace(class),
		Body:    string(body),
	})
}

// MarkWorktreeActiveCycle sets the ActiveCandidateCycle flag in the worktree's
// cleanup metadata. The cleanup manager preserves any worktree with this flag
// set so it is never deleted mid-cycle before the coordinator disposes it.
func MarkWorktreeActiveCycle(wtPath string) error {
	meta, err := ReadExecutionCleanupMetadata(wtPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("mark active cycle: read metadata: %w", err)
	}
	meta.ActiveCandidateCycle = true
	return WriteExecutionCleanupMetadata(wtPath, meta)
}

// ClearWorktreeActiveCycle clears the ActiveCandidateCycle flag in the
// worktree's cleanup metadata after the coordinator reaches terminal disposition.
func ClearWorktreeActiveCycle(wtPath string) error {
	meta, err := ReadExecutionCleanupMetadata(wtPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("clear active cycle: read metadata: %w", err)
	}
	meta.ActiveCandidateCycle = false
	return WriteExecutionCleanupMetadata(wtPath, meta)
}

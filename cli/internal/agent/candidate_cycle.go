package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
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

// CandidateCycleState is the shared on-disk snapshot of the candidate-cycle
// phase currently owning a live attempt worktree.
type CandidateCycleState struct {
	Active       bool
	Phase        string
	CandidateRef string
	CandidateRev string
	CycleIndex   int
	ReviewActive bool
	RepairActive bool
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
	Verdict              string
	Rationale            string
	PerAC                []ReviewAC
	Findings             []Finding
	VerificationCommands []string
	Classification       string
	ReviewGroupID        string
	ReviewerIndices      []int
	ReviewerVerdicts     []string
	ReviewerRoute        ExecutionCycleRouteFacts
}

// CandidateReviewer runs a read-only review pass against a committed candidate
// inside the still-live worktree. Reserved for future candidate-cycle review
// implementation (non-scope for FEAT-010 initial pass).
type CandidateReviewer interface {
	Review(ctx context.Context, projectRoot string, candidate CandidateResult) (CandidateReviewResult, error)
}

// RepairPass attempts to repair a candidate in the still-live worktree based on
// the repair prompt, returning an updated CandidateResult with an append-only
// repair commit.
type RepairPass interface {
	Repair(ctx context.Context, candidate CandidateResult, prompt string) (CandidateResult, error)
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
const repairCycleStartedEventKind = "repair-cycle-started"
const candidateReviewProviderEmptyRetryLimit = 2

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

// RepairCycleStartedEventBody is the structured payload of a
// repair-cycle-started bead event.
type RepairCycleStartedEventBody struct {
	AttemptID          string `json:"attempt_id,omitempty"`
	BaseRev            string `json:"base_rev,omitempty"`
	FailedCandidateRev string `json:"failed_candidate_rev,omitempty"`
	RepairCycleIndex   int    `json:"repair_cycle_index"`
	Classification     string `json:"classification,omitempty"`
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
// Pass is required. Lander is optional: nil finalizes the worker-side
// candidate cycle without advancing the target branch. Nil optional fields
// (Checks, Reviewer, Repair, RefStore) skip their respective phases.
//
// Repair loops are reserved for future implementation. When Reviewer is non-nil
// the coordinator runs one pre-land candidate review before landing.
type AttemptCycleCoordinator struct {
	Pass                   ImplementationPass
	Checks                 CandidateCheckRunner // nil → skip pre-land checks
	Reviewer               CandidateReviewer    // nil → skip candidate review (future)
	NoReview               bool                 // true → skip review with durable no-review reason
	Repair                 RepairPass           // nil → skip repair loops (future)
	Lander                 CandidateLander
	RefStore               CandidateRefStore                     // nil → no ref pinning
	ProjectRoot            string                                // required when RefStore non-nil
	BeadEvents             BeadEventAppender                     // nil → no candidate-cycle event emission
	ValidateCandidate      func(candidate CandidateResult) error // nil → no candidate validation
	ImportCandidate        func(candidate CandidateResult) error // nil → candidate is already project-reachable
	ReleaseCandidateImport func(candidate CandidateResult) error // nil → no transient import ref
	// OriginalTask is the immutable bead task supplied to every repair operation.
	// CandidateDiff supplies the bounded base..candidate git diff. Production
	// wires boundedCandidateRepairDiff; nil is retained for seam-only tests that
	// do not have a real repository.
	OriginalTask  string
	CandidateDiff func(candidate CandidateResult) (string, error)
	// RepairCaps is the implementer-role evidence budget used for both the
	// candidate diff and the final repair prompt. RepairCapsConfigured preserves
	// an explicitly configured all-zero budget; unset seam-only tests use the
	// binary defaults.
	RepairCaps           evidence.Caps
	RepairCapsConfigured bool
	// RepairMaxCycles caps append-only repair attempts after review_fixable_gap.
	// Values <=0 default to one repair cycle.
	RepairMaxCycles int
}

// Run executes one attempt cycle: implementation → checks → optional review →
// optional repair → optional land. Non-success statuses (no_changes,
// execution_failed, etc.) return without calling the lander.
// The caller is responsible for worktree cleanup after Run returns.
//
// When RefStore and ProjectRoot are set, Run pins a candidate ref under
// refs/ddx/iterations/<attempt-id>/<cycle-index> before checks and review.
// After a successful land the temporary ref is cleaned up; for preserved,
// conflicted, parked, or otherwise non-landed outcomes the ref is retained so
// operators can inspect and recover the candidate. When Lander is nil the
// coordinator finalizes the worker-side candidate state and retains the ref.
func (c *AttemptCycleCoordinator) Run(ctx context.Context, beadID string) (AttemptCycleResult, error) {
	candidate, err := c.Pass.Execute(ctx, beadID)
	if err != nil {
		return AttemptCycleResult{Report: candidate.Report}, err
	}

	if candidate.Report.Status != ExecuteBeadStatusSuccess {
		return AttemptCycleResult{Report: candidate.Report}, nil
	}

	repairCycles := 0
	reviewRetryCount := 0
	candidatePrepared := false
	cycleTrace := append([]ExecutionCycleTrace(nil), candidate.Report.CycleTrace...)
	recordCycle := func(report *ExecuteBeadReport, review *CandidateReviewResult, finalDecision string) {
		entry := executionCycleTraceFor(candidate, review, finalDecision)
		cycleTrace = appendExecutionCycleTrace(cycleTrace, entry)
		if report != nil {
			report.CycleTrace = append([]ExecutionCycleTrace(nil), cycleTrace...)
		}
	}
	var pinnedRefs []string
	var cycleReview *CandidateReviewResult
	cleanupPinnedRefs := func() {
		if c.RefStore == nil || c.ProjectRoot == "" {
			return
		}
		for _, ref := range pinnedRefs {
			_ = c.RefStore.UnpinCandidateRef(c.ProjectRoot, ref)
		}
	}
	failCandidate := func(prefix string, failure error, reason string) (AttemptCycleResult, error) {
		report := candidate.Report
		report.CandidateRef = ""
		report.Status = ExecuteBeadStatusExecutionFailed
		report.Detail = prefix + ": " + failure.Error()
		report.Error = failure.Error()
		report.OutcomeReason = reason
		recordCycle(&report, cycleReview, report.Status)
		cleanupPinnedRefs()
		return AttemptCycleResult{Report: report}, fmt.Errorf("%s cycle %d: %w", prefix, candidate.CycleIndex, failure)
	}
	for {
		if !candidatePrepared {
			// Validate every candidate revision, including append-only repair output,
			// before any project-root ref, event, check, review, or landing side
			// effect makes that revision durable or externally visible.
			if c.ValidateCandidate != nil {
				if validateErr := c.ValidateCandidate(candidate); validateErr != nil {
					return failCandidate("validating candidate", validateErr, FailureModeAttemptIntegrity)
				}
			}
			// Clone backends import the validated object through a short-lived ref.
			// Linked and in-tree backends no-op. No project candidate ref is created
			// until import succeeds.
			if c.ImportCandidate != nil {
				if importErr := c.ImportCandidate(candidate); importErr != nil {
					if c.ReleaseCandidateImport != nil {
						importErr = errors.Join(importErr, c.ReleaseCandidateImport(candidate))
					}
					return failCandidate("importing candidate", importErr, FailureModeLandRetry)
				}
			}
			c.recordCandidateCycleState(candidate, CandidateCycleState{
				Active:       true,
				Phase:        "candidate",
				CandidateRev: candidate.Report.ResultRev,
				CycleIndex:   candidate.CycleIndex,
			})
			if pinErr := c.pinCandidateRef(beadID, &candidate); pinErr != nil {
				if c.ReleaseCandidateImport != nil {
					pinErr = errors.Join(pinErr, c.ReleaseCandidateImport(candidate))
				}
				return failCandidate("pinning candidate", pinErr, FailureModeLandRetry)
			}
			pinnedRefs = appendUniqueString(pinnedRefs, candidate.Report.CandidateRef)
			if c.ReleaseCandidateImport != nil {
				if releaseErr := c.ReleaseCandidateImport(candidate); releaseErr != nil {
					return failCandidate("releasing candidate import", releaseErr, FailureModeLandRetry)
				}
			}
			c.recordCandidateCycleState(candidate, CandidateCycleState{
				Active:       true,
				Phase:        "candidate_pinned",
				CandidateRef: candidate.Report.CandidateRef,
				CandidateRev: candidate.Report.ResultRev,
				CycleIndex:   candidate.CycleIndex,
			})

			if c.Checks != nil {
				c.recordCandidateCycleState(candidate, CandidateCycleState{
					Active:       true,
					Phase:        "checks",
					CandidateRef: candidate.Report.CandidateRef,
					CandidateRev: candidate.Report.ResultRev,
					CycleIndex:   candidate.CycleIndex,
				})
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
					// A check-runner error is infrastructure/setup failure, not a
					// candidate defect an implementer can safely repair.
					if checksErr != nil || c.Repair == nil {
						recordCycle(&report, nil, report.Status)
						return AttemptCycleResult{Report: report}, nil
					}
					if repairCycles >= c.maxRepairCycles() {
						report.Status = ExecuteBeadStatusRepairCycleExhausted
						report.OutcomeReason = ExecuteBeadStatusRepairCycleExhausted
						report.Detail = "pre-land repair: " + ExecuteBeadStatusRepairCycleExhausted
						recordCycle(&report, nil, report.Status)
						return AttemptCycleResult{Report: report}, nil
					}
					recordCycle(&candidate.Report, nil, ExecuteBeadStatusPostRunCheckFailed)
					prompt, promptErr := c.buildRepairPrompt(beadID, candidate, CandidateReviewResult{}, repairCycles+1, checksResult.Detail)
					if promptErr != nil {
						report.Status = ExecuteBeadStatusExecutionFailed
						report.Detail = "pre-land repair prompt: " + promptErr.Error()
						report.Error = promptErr.Error()
						recordCycle(&report, nil, report.Status)
						return AttemptCycleResult{Report: report}, nil
					}
					c.appendRepairCycleStartedEvent(beadID, candidate, repairCycles+1, candidateChecksFailedEventKind)
					c.recordCandidateCycleState(candidate, CandidateCycleState{
						Active:       true,
						Phase:        "repair",
						CandidateRef: candidate.Report.CandidateRef,
						CandidateRev: candidate.Report.ResultRev,
						CycleIndex:   candidate.CycleIndex,
						RepairActive: true,
					})
					repaired, repairErr := c.Repair.Repair(ctx, candidate, prompt)
					if repairErr != nil {
						report.Status = ExecuteBeadStatusExecutionFailed
						report.Detail = "pre-land repair: " + repairErr.Error()
						report.Error = repairErr.Error()
						recordCycle(&report, nil, report.Status)
						return AttemptCycleResult{Report: report}, nil
					}
					repairCycles++
					candidate = normalizeRepairedCandidate(candidate, repaired)
					if candidate.Report.Status != ExecuteBeadStatusSuccess {
						recordCycle(&candidate.Report, nil, candidate.Report.Status)
						return AttemptCycleResult{Report: candidate.Report}, nil
					}
					cycleReview = nil
					candidatePrepared = false
					continue
				}
			}
			candidatePrepared = true
		}

		if c.NoReview || c.Reviewer == nil {
			if c.NoReview {
				candidate.Report.ReviewSkipReason = "--no-review"
			}
			break
		}

		c.recordCandidateCycleState(candidate, CandidateCycleState{
			Active:       true,
			Phase:        "review",
			CandidateRef: candidate.Report.CandidateRef,
			CandidateRev: candidate.Report.ResultRev,
			CycleIndex:   candidate.CycleIndex,
			ReviewActive: true,
		})
		reviewResult, reviewErr := c.Reviewer.Review(ctx, c.ProjectRoot, candidate)
		cycleReview = &reviewResult
		if reviewErr != nil {
			if candidateReviewErrorClass(reviewErr) == evidence.OutcomeReviewProviderEmpty {
				reviewRoute := candidateReviewRouteLabel(reviewResult, candidate)
				c.appendCandidateReviewRetryEvent(beadID, candidate, reviewRoute, reviewRetryCount+1)
				if reviewRetryCount < candidateReviewProviderEmptyRetryLimit {
					reviewRetryCount++
					continue
				}
				report := candidate.Report
				report.EscalationCount = reviewRetryCount
				report.Status = ExecuteBeadStatusReviewMalfunction
				report.Detail = fmt.Sprintf("pre-land review: %s after retries=%d on review route %s", evidence.OutcomeReviewProviderEmpty, reviewRetryCount, reviewRoute)
				report.Error = reviewErr.Error()
				recordCycle(&report, cycleReview, report.Status)
				return AttemptCycleResult{Report: report}, nil
			}

			report := candidate.Report
			report.EscalationCount = reviewRetryCount
			report.Status = ExecuteBeadStatusReviewMalfunction
			report.Detail = "pre-land review: " + reviewErr.Error()
			report.ReviewVerdict = strings.TrimSpace(reviewResult.Verdict)
			report.ReviewRationale = strings.TrimSpace(reviewResult.Rationale)
			c.appendCandidateReviewClassifiedEvent(beadID, report, reviewResult, ReviewFindingClassMalfunction, reviewErr.Error())
			recordCycle(&report, cycleReview, report.Status)
			return AttemptCycleResult{Report: report}, nil
		}

		candidate.Report.EscalationCount = reviewRetryCount
		verdict := Verdict(strings.TrimSpace(reviewResult.Verdict))
		switch verdict {
		case VerdictApprove:
			candidate.Report.ReviewVerdict = string(VerdictApprove)
			candidate.Report.ReviewRationale = strings.TrimSpace(reviewResult.Rationale)
		case VerdictRequestChanges, VerdictBlock:
			classification := c.classifyCandidateReview(reviewResult)
			report := candidate.Report
			report.ReviewVerdict = string(verdict)
			report.ReviewRationale = strings.TrimSpace(reviewResult.Rationale)
			if verdict == VerdictRequestChanges {
				report.Status = ExecuteBeadStatusReviewRequestChanges
				report.Detail = "pre-land review: REQUEST_CHANGES"
			} else {
				report.Status = ExecuteBeadStatusReviewBlock
				report.Detail = "pre-land review: BLOCK"
			}
			if verdict == VerdictBlock && classification.Class == ReviewFindingClassMalfunction {
				report.Status = ExecuteBeadStatusReviewMalfunction
				report.Detail = "pre-land review: " + ReviewFindingClassMalfunction
			}
			c.appendCandidateReviewClassifiedEvent(beadID, report, reviewResult, classification.Class, classification.Reason)
			if !classification.AutomatedRepairEligible || c.Repair == nil {
				recordCycle(&report, cycleReview, report.Status)
				return AttemptCycleResult{Report: report}, nil
			}
			if repairCycles >= c.maxRepairCycles() {
				report.Status = ExecuteBeadStatusRepairCycleExhausted
				report.OutcomeReason = ExecuteBeadStatusRepairCycleExhausted
				report.Detail = "pre-land repair: " + ExecuteBeadStatusRepairCycleExhausted
				recordCycle(&report, cycleReview, report.Status)
				return AttemptCycleResult{Report: report}, nil
			}
			recordCycle(&candidate.Report, cycleReview, ExecuteBeadStatusReviewFixableGap)
			prompt, promptErr := c.buildRepairPrompt(beadID, candidate, reviewResult, repairCycles+1, reviewResult.Rationale)
			if promptErr != nil {
				report.Status = ExecuteBeadStatusExecutionFailed
				report.Detail = "pre-land repair prompt: " + promptErr.Error()
				report.Error = promptErr.Error()
				recordCycle(&report, cycleReview, report.Status)
				return AttemptCycleResult{Report: report}, nil
			}
			c.appendRepairCycleStartedEvent(beadID, candidate, repairCycles+1, classification.Class)
			c.recordCandidateCycleState(candidate, CandidateCycleState{
				Active:       true,
				Phase:        "repair",
				CandidateRef: candidate.Report.CandidateRef,
				CandidateRev: candidate.Report.ResultRev,
				CycleIndex:   candidate.CycleIndex,
				RepairActive: true,
			})
			repaired, repairErr := c.Repair.Repair(ctx, candidate, prompt)
			if repairErr != nil {
				report.Status = ExecuteBeadStatusExecutionFailed
				report.Detail = "pre-land repair: " + repairErr.Error()
				report.Error = repairErr.Error()
				recordCycle(&report, cycleReview, report.Status)
				return AttemptCycleResult{Report: report}, nil
			}
			repairCycles++
			candidate = normalizeRepairedCandidate(candidate, repaired)
			if candidate.Report.Status != ExecuteBeadStatusSuccess {
				recordCycle(&candidate.Report, cycleReview, candidate.Report.Status)
				return AttemptCycleResult{Report: candidate.Report}, nil
			}
			candidatePrepared = false
			continue
		default:
			report := candidate.Report
			report.Status = ExecuteBeadStatusReviewMalfunction
			report.Detail = "pre-land review: malformed verdict"
			report.ReviewVerdict = strings.TrimSpace(reviewResult.Verdict)
			report.ReviewRationale = strings.TrimSpace(reviewResult.Rationale)
			c.appendCandidateReviewClassifiedEvent(beadID, report, reviewResult, ReviewFindingClassMalfunction, "malformed review verdict")
			return AttemptCycleResult{Report: report}, nil
		}
		break
	}

	if c.Lander == nil {
		recordCycle(&candidate.Report, cycleReview, candidate.Report.Status)
		return AttemptCycleResult{Report: candidate.Report}, nil
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
	if landed.EscalationCount == 0 && candidate.Report.EscalationCount != 0 {
		landed.EscalationCount = candidate.Report.EscalationCount
	}
	recordCycle(&landed, cycleReview, landed.Status)

	result := AttemptCycleResult{Report: landed, Landed: true}

	// Clean up the candidate ref after successful landing. For any non-success
	// outcome (preserved, conflicted, parked, manual) the ref is retained so
	// operators can inspect the candidate commit after the worktree is removed.
	if c.RefStore != nil && c.ProjectRoot != "" && landed.CandidateRef != "" {
		if !ShouldRetainCandidateRef(landed.Status) {
			for _, ref := range pinnedRefs {
				_ = c.RefStore.UnpinCandidateRef(c.ProjectRoot, ref)
			}
		}
	}

	return result, nil
}

func (c *AttemptCycleCoordinator) recordCandidateCycleState(candidate CandidateResult, state CandidateCycleState) {
	if candidate.WorktreePath == "" {
		return
	}
	_ = WriteWorktreeCandidateCycleState(c.ProjectRoot, candidate.WorktreePath, candidate.Report.AttemptID, state)
}

func candidateChecksFailedDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return candidateChecksFailedEventKind
	}
	return candidateChecksFailedEventKind + ": " + detail
}

func appendUniqueString(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func (c *AttemptCycleCoordinator) pinCandidateRef(beadID string, candidate *CandidateResult) error {
	if candidate == nil || c.RefStore == nil || c.ProjectRoot == "" {
		return nil
	}
	if candidate.Report.CandidateRef != "" {
		return nil
	}
	ref, pinErr := c.RefStore.PinCandidateRef(
		c.ProjectRoot,
		candidate.Report.AttemptID,
		candidate.CycleIndex,
		candidate.Report.ResultRev,
	)
	if pinErr != nil {
		return pinErr
	}
	candidate.Report.CandidateRef = ref
	candidate.Report.CycleIndex = candidate.CycleIndex
	if c.BeadEvents == nil {
		return nil
	}
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
	return nil
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

func (c *AttemptCycleCoordinator) maxRepairCycles() int {
	if c.RepairMaxCycles <= 0 {
		return 1
	}
	return c.RepairMaxCycles
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

func (c *AttemptCycleCoordinator) buildRepairPrompt(beadID string, candidate CandidateResult, review CandidateReviewResult, repairCycleIndex int, failureOutput string) (string, error) {
	var diff string
	if c.CandidateDiff != nil {
		var err error
		diff, err = c.CandidateDiff(candidate)
		if err != nil {
			return "", err
		}
	}
	prompt := BuildRepairPrompt(repairPromptInput(beadID, candidate, review, repairCycleIndex, c.OriginalTask, diff, failureOutput))
	if err := validateInlinePromptCap("implementer repair prompt", prompt, c.effectiveRepairCaps().MaxPromptBytes); err != nil {
		return "", err
	}
	return prompt, nil
}

func boundedCandidateRepairDiff(candidate CandidateResult) (string, error) {
	return boundedCandidateRepairDiffWithCaps(candidate, evidence.DefaultCaps())
}

func boundedCandidateRepairDiffWithCaps(candidate CandidateResult, caps evidence.Caps) (string, error) {
	if strings.TrimSpace(candidate.WorktreePath) == "" || strings.TrimSpace(candidate.Report.BaseRev) == "" || strings.TrimSpace(candidate.Report.ResultRev) == "" {
		return "", fmt.Errorf("repair diff requires worktree_path, base_rev, and result_rev")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := append([]string{"diff", "--no-color", candidate.Report.BaseRev, candidate.Report.ResultRev, "--", "."}, EvidenceReviewExcludePathspecs()...)
	out, err := internalgit.Command(ctx, candidate.WorktreePath, args...).Output()
	if err != nil {
		return "", fmt.Errorf("repair diff %s..%s: %w", candidate.Report.BaseRev, candidate.Report.ResultRev, err)
	}
	clamped, _ := evidence.ClampDiff(string(out), caps.MaxDiffBytes)
	return clamped, nil
}

func (c *AttemptCycleCoordinator) effectiveRepairCaps() evidence.Caps {
	if c.RepairCapsConfigured || c.RepairCaps != (evidence.Caps{}) {
		return c.RepairCaps
	}
	return evidence.DefaultCaps()
}

func repairPromptInput(beadID string, candidate CandidateResult, review CandidateReviewResult, repairCycleIndex int, originalTask, currentDiff, failureOutput string) RepairPromptInput {
	return RepairPromptInput{
		BeadID:               beadID,
		BaseRev:              candidate.Report.BaseRev,
		FailedCandidateRev:   candidate.Report.ResultRev,
		CycleIndex:           repairCycleIndex,
		OriginalTask:         originalTask,
		CurrentDiff:          currentDiff,
		FailureOutput:        failureOutput,
		ReviewRationale:      strings.TrimSpace(review.Rationale),
		PerAC:                append([]ReviewAC(nil), review.PerAC...),
		Findings:             append([]Finding(nil), review.Findings...),
		VerificationCommands: append([]string(nil), review.VerificationCommands...),
	}
}

func normalizeRepairedCandidate(previous, repaired CandidateResult) CandidateResult {
	if repaired.WorktreePath == "" {
		repaired.WorktreePath = previous.WorktreePath
	}
	if repaired.CycleIndex <= previous.CycleIndex {
		repaired.CycleIndex = previous.CycleIndex + 1
	}
	if repaired.Report.BeadID == "" {
		repaired.Report.BeadID = previous.Report.BeadID
	}
	if repaired.Report.AttemptID == "" {
		repaired.Report.AttemptID = previous.Report.AttemptID
	}
	if repaired.Report.BaseRev == "" {
		repaired.Report.BaseRev = previous.Report.BaseRev
	}
	if repaired.Report.ResultRev != "" && repaired.Report.ResultRev != previous.Report.ResultRev {
		// Local-only evidence means the final candidate revision is also the
		// implementation revision. A repaired append-only commit must replace,
		// not trail behind, the provisional implementation SHA.
		repaired.Report.ImplementationRev = repaired.Report.ResultRev
	} else if repaired.Report.ImplementationRev == "" {
		repaired.Report.ImplementationRev = previous.Report.ImplementationRev
	}
	if repaired.Report.Status == "" {
		repaired.Report.Status = ExecuteBeadStatusSuccess
	}
	if repaired.Report.CycleIndex == 0 && repaired.CycleIndex != 0 {
		repaired.Report.CycleIndex = repaired.CycleIndex
	}
	if len(repaired.Report.CycleTrace) == 0 && len(previous.Report.CycleTrace) > 0 {
		repaired.Report.CycleTrace = append([]ExecutionCycleTrace(nil), previous.Report.CycleTrace...)
	}
	repaired.Report.CandidateRef = ""
	repaired.Report.ReviewVerdict = ""
	repaired.Report.ReviewRationale = ""
	return repaired
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

func (c *AttemptCycleCoordinator) appendCandidateReviewRetryEvent(beadID string, candidate CandidateResult, reviewRoute string, attemptCount int) {
	if c.BeadEvents == nil {
		return
	}
	reason := "review route " + strings.TrimSpace(reviewRoute)
	actor := strings.TrimSpace(candidate.Report.WorkerID)
	if actor == "" {
		actor = "ddx work"
	}
	if store, ok := c.BeadEvents.(ExecuteBeadLoopStore); ok {
		appendReviewGateError(store, beadID, actor, time.Now().UTC(), candidate.Report.ResultRev, evidence.OutcomeReviewProviderEmpty, reason)
		return
	}
	body := ReviewErrorEventBody(evidence.OutcomeReviewProviderEmpty, attemptCount, candidate.Report.ResultRev, reason)
	_ = c.BeadEvents.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "review-error",
		Summary:   evidence.OutcomeReviewProviderEmpty,
		Body:      body,
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	})
}

func candidateReviewErrorClass(reviewErr error) string {
	if reviewErr == nil {
		return ""
	}
	errText := reviewErr.Error()
	switch {
	case strings.Contains(errText, evidence.OutcomeReviewProviderEmpty):
		return evidence.OutcomeReviewProviderEmpty
	case strings.Contains(errText, evidence.OutcomeReviewUnparseable):
		return evidence.OutcomeReviewUnparseable
	case strings.Contains(errText, evidence.OutcomeReviewTransport):
		return evidence.OutcomeReviewTransport
	default:
		return ""
	}
}

func candidateReviewRouteLabel(review CandidateReviewResult, candidate CandidateResult) string {
	harness := strings.TrimSpace(review.ReviewerRoute.Harness)
	if harness == "" {
		harness = strings.TrimSpace(candidate.Report.Harness)
	}
	model := strings.TrimSpace(review.ReviewerRoute.Model)
	if model == "" {
		model = strings.TrimSpace(candidate.Report.Model)
	}
	provider := strings.TrimSpace(review.ReviewerRoute.Provider)
	if provider == "" {
		provider = strings.TrimSpace(candidate.Report.Provider)
	}
	parts := make([]string, 0, 3)
	if harness != "" {
		parts = append(parts, "harness="+harness)
	}
	if model != "" {
		parts = append(parts, "model="+model)
	}
	if provider != "" {
		parts = append(parts, "provider="+provider)
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, " ")
}

func (c *AttemptCycleCoordinator) appendRepairCycleStartedEvent(beadID string, candidate CandidateResult, repairCycleIndex int, class string) {
	if c.BeadEvents == nil {
		return
	}
	body, _ := json.Marshal(RepairCycleStartedEventBody{
		AttemptID:          candidate.Report.AttemptID,
		BaseRev:            candidate.Report.BaseRev,
		FailedCandidateRev: candidate.Report.ResultRev,
		RepairCycleIndex:   repairCycleIndex,
		Classification:     class,
	})
	_ = c.BeadEvents.AppendEvent(beadID, bead.BeadEvent{
		Kind:    repairCycleStartedEventKind,
		Summary: class,
		Body:    string(body),
	})
}

func appendExecutionCycleTrace(trace []ExecutionCycleTrace, entry ExecutionCycleTrace) []ExecutionCycleTrace {
	return append(trace, entry)
}

func executionCycleTraceFor(candidate CandidateResult, review *CandidateReviewResult, finalDecision string) ExecutionCycleTrace {
	reviewPresent := review != nil
	reviewVerdict := ""
	if review != nil {
		reviewVerdict = review.Verdict
	}
	audit := executionDecisionAuditForReport(candidate.Report, finalDecision, reviewPresent, reviewVerdict)
	entry := ExecutionCycleTrace{
		CycleIndex: candidate.CycleIndex,
		AttemptID:  candidate.Report.AttemptID,
		ResultRev:  candidate.Report.ResultRev,
		ImplementerRoute: ExecutionCycleRouteFacts{
			Harness:     candidate.Report.Harness,
			Provider:    candidate.Report.Provider,
			Model:       candidate.Report.Model,
			ActualPower: candidate.Report.ActualPower,
		},
		RequestedRoute: audit.RequestedRoute,
		ActualRoute:    audit.ActualRoute,
		FinalDecision:  finalDecision,
	}
	applyDecisionAuditToTrace(&entry, audit)
	if review == nil {
		return entry
	}
	entry.ReviewGroupID = strings.TrimSpace(review.ReviewGroupID)
	entry.ReviewerIndices = append([]int(nil), review.ReviewerIndices...)
	entry.ReviewVerdicts = append([]string(nil), review.ReviewerVerdicts...)
	entry.ReviewResult = ExecutionCycleReviewResult{
		Verdict:        strings.TrimSpace(review.Verdict),
		Rationale:      strings.TrimSpace(review.Rationale),
		Classification: strings.TrimSpace(review.Classification),
		PerAC:          append([]ReviewAC(nil), review.PerAC...),
		Findings:       append([]Finding(nil), review.Findings...),
	}
	entry.ReviewerRoute = review.ReviewerRoute
	return entry
}

// WriteWorktreeCandidateCycleState updates cleanup metadata and, when a
// matching run-state exists, the per-attempt run-state snapshot for a live
// candidate-cycle worktree.
func WriteWorktreeCandidateCycleState(projectRoot, wtPath, attemptID string, state CandidateCycleState) error {
	meta, err := ReadExecutionCleanupMetadata(wtPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("write candidate cycle state: read metadata: %w", err)
	}
	meta.WorktreePath = firstNonEmpty(meta.WorktreePath, wtPath)
	meta.ProjectRoot = firstNonEmpty(meta.ProjectRoot, projectRoot)
	meta.AttemptID = firstNonEmpty(meta.AttemptID, attemptID)
	applyCandidateCycleStateToMetadata(&meta, state)
	if err := WriteExecutionCleanupMetadata(wtPath, meta); err != nil {
		return err
	}
	if projectRoot == "" || meta.AttemptID == "" {
		return nil
	}
	states, err := ReadRunStates(projectRoot)
	if err != nil {
		return nil
	}
	for i := range states {
		current := states[i]
		if current.AttemptID != meta.AttemptID {
			continue
		}
		applyCandidateCycleStateToRunState(&current, state)
		return WriteRunState(projectRoot, current)
	}
	return nil
}

// MarkWorktreeActiveCycle sets the ActiveCandidateCycle flag in the worktree's
// cleanup metadata. The cleanup manager preserves any worktree with this flag
// set so it is never deleted mid-cycle before the coordinator disposes it.
func MarkWorktreeActiveCycle(wtPath string) error {
	return WriteWorktreeCandidateCycleState("", wtPath, "", CandidateCycleState{
		Active: true,
		Phase:  "active",
	})
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
	meta.CandidateCyclePhase = ""
	meta.ReviewActive = false
	meta.RepairActive = false
	return WriteExecutionCleanupMetadata(wtPath, meta)
}

func applyCandidateCycleStateToMetadata(meta *ExecutionCleanupMetadata, state CandidateCycleState) {
	meta.ActiveCandidateCycle = state.Active
	meta.CandidateCyclePhase = state.Phase
	meta.CandidateRef = state.CandidateRef
	meta.CandidateRev = state.CandidateRev
	meta.CycleIndex = state.CycleIndex
	meta.ReviewActive = state.ReviewActive
	meta.RepairActive = state.RepairActive
}

func applyCandidateCycleStateToRunState(runState *RunState, state CandidateCycleState) {
	runState.CandidateCyclePhase = state.Phase
	runState.CandidateRef = state.CandidateRef
	runState.CandidateRev = state.CandidateRev
	runState.CycleIndex = state.CycleIndex
	runState.ReviewActive = state.ReviewActive
	runState.RepairActive = state.RepairActive
}

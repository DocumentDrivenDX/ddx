package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	Passed bool
	Detail string
}

// CandidateCheckRunner runs pre-land quality checks against a candidate.
type CandidateCheckRunner interface {
	RunChecks(ctx context.Context, projectRoot string, candidate CandidateResult) (CandidateCheckResult, error)
}

// CandidateReviewResult carries a reviewer verdict for a committed candidate.
type CandidateReviewResult struct {
	// Verdict is one of "APPROVE", "REQUEST_CHANGES", or "BLOCK".
	Verdict   string
	Rationale string
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
// Review and repair loops are reserved for future implementation. When Reviewer
// and Repair are non-nil the coordinator will run review/repair cycles before
// landing. Today they are accepted in the struct but not invoked (non-scope
// per FEAT-010 candidate-cycle spec).
type AttemptCycleCoordinator struct {
	Pass     ImplementationPass
	Checks   CandidateCheckRunner // nil → skip pre-land checks
	Reviewer CandidateReviewer    // nil → skip candidate review (future)
	Repair   RepairPass           // nil → skip repair loops (future)
	Lander   CandidateLander
	RefStore CandidateRefStore // nil → no ref pinning
}

// Run executes one attempt cycle: implementation → land. Non-success statuses
// (no_changes, execution_failed, etc.) return without calling the lander.
// The caller is responsible for worktree cleanup after Run returns.
func (c *AttemptCycleCoordinator) Run(ctx context.Context, beadID string) (AttemptCycleResult, error) {
	candidate, err := c.Pass.Execute(ctx, beadID)
	if err != nil {
		return AttemptCycleResult{Report: candidate.Report}, err
	}

	if candidate.Report.Status != ExecuteBeadStatusSuccess {
		return AttemptCycleResult{Report: candidate.Report}, nil
	}

	landed, err := c.Lander.Land(ctx, candidate)
	if err != nil {
		return AttemptCycleResult{Report: candidate.Report}, err
	}
	return AttemptCycleResult{Report: landed, Landed: true}, nil
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

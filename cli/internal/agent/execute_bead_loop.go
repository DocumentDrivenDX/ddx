package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

type ExecuteBeadReport struct {
	BeadID      string `json:"bead_id"`
	Status      string `json:"status"`
	Detail      string `json:"detail,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	BaseRev     string `json:"base_rev,omitempty"`
	ResultRev   string `json:"result_rev,omitempty"`
	PreserveRef string `json:"preserve_ref,omitempty"`
}

type ExecuteBeadExecutor interface {
	Execute(ctx context.Context, beadID string) (ExecuteBeadReport, error)
}

type ExecuteBeadExecutorFunc func(ctx context.Context, beadID string) (ExecuteBeadReport, error)

func (f ExecuteBeadExecutorFunc) Execute(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
	return f(ctx, beadID)
}

type ExecuteBeadLoopStore interface {
	ReadyExecution() ([]bead.Bead, error)
	Claim(id, assignee string) error
	Unclaim(id string) error
	CloseWithEvidence(id, sessionID, commitSHA string) error
	AppendEvent(id string, event bead.BeadEvent) error
}

type ExecuteBeadLoopOptions struct {
	Assignee     string
	Once         bool
	PollInterval time.Duration
	Log          io.Writer
}

type ExecuteBeadLoopResult struct {
	Attempts          int                 `json:"attempts"`
	Successes         int                 `json:"successes"`
	Failures          int                 `json:"failures"`
	NoReadyWork       bool                `json:"no_ready_work,omitempty"`
	LastSuccessAt     time.Time           `json:"last_success_at,omitempty"`
	LastFailureStatus string              `json:"last_failure_status,omitempty"`
	Results           []ExecuteBeadReport `json:"results,omitempty"`
}

// ExecuteBeadWorker drains the current single-project execution-ready queue.
// It intentionally does not retry a failed/conflicted bead again in the same
// process run; a later operator-driven invocation can create the next attempt.
type ExecuteBeadWorker struct {
	Store    ExecuteBeadLoopStore
	Executor ExecuteBeadExecutor
	Now      func() time.Time
}

func (w *ExecuteBeadWorker) Run(ctx context.Context, opts ExecuteBeadLoopOptions) (*ExecuteBeadLoopResult, error) {
	if w.Store == nil {
		return nil, fmt.Errorf("execute-bead loop: store is required")
	}
	if w.Executor == nil {
		return nil, fmt.Errorf("execute-bead loop: executor is required")
	}

	now := w.Now
	if now == nil {
		now = time.Now
	}
	assignee := opts.Assignee
	if assignee == "" {
		assignee = "ddx"
	}

	result := &ExecuteBeadLoopResult{}
	attempted := make(map[string]struct{})

	for {
		candidate, ok, err := w.nextCandidate(attempted)
		if err != nil {
			return result, err
		}
		if !ok {
			if result.Attempts == 0 {
				result.NoReadyWork = true
			}
			if opts.PollInterval <= 0 {
				return result, nil
			}
			if err := sleepWithContext(ctx, opts.PollInterval); err != nil {
				return result, err
			}
			continue
		}

		attempted[candidate.ID] = struct{}{}
		if err := w.Store.Claim(candidate.ID, assignee); err != nil {
			continue
		}

		report, err := w.Executor.Execute(ctx, candidate.ID)
		if err != nil {
			report = ExecuteBeadReport{
				BeadID: candidate.ID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: err.Error(),
			}
		}
		if report.BeadID == "" {
			report.BeadID = candidate.ID
		}
		if report.Status == "" {
			report.Status = ExecuteBeadStatusExecutionFailed
		}
		if report.Detail == "" {
			report.Detail = ExecuteBeadStatusDetail(report.Status, "", "")
		}

		result.Attempts++
		result.Results = append(result.Results, report)

		if report.Status == ExecuteBeadStatusSuccess {
			if err := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.ResultRev); err != nil {
				return result, err
			}
			result.Successes++
			result.LastSuccessAt = now().UTC()
		} else {
			if err := w.Store.Unclaim(candidate.ID); err != nil {
				return result, err
			}
			result.Failures++
			result.LastFailureStatus = report.Status
		}

		if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
			return result, err
		}

		if opts.Log != nil {
			_, _ = fmt.Fprintf(opts.Log, "%s %s\n", report.BeadID, report.Status)
		}

		if opts.Once {
			return result, nil
		}
	}
}

func (w *ExecuteBeadWorker) nextCandidate(attempted map[string]struct{}) (bead.Bead, bool, error) {
	ready, err := w.Store.ReadyExecution()
	if err != nil {
		return bead.Bead{}, false, err
	}
	for _, candidate := range ready {
		if _, seen := attempted[candidate.ID]; seen {
			continue
		}
		return candidate, true, nil
	}
	return bead.Bead{}, false, nil
}

func executeBeadLoopEvent(report ExecuteBeadReport, actor string, createdAt time.Time) bead.BeadEvent {
	parts := []string{}
	if report.Detail != "" {
		parts = append(parts, report.Detail)
	}
	if report.PreserveRef != "" {
		parts = append(parts, fmt.Sprintf("preserve_ref=%s", report.PreserveRef))
	}
	if report.ResultRev != "" {
		parts = append(parts, fmt.Sprintf("result_rev=%s", report.ResultRev))
	}
	if report.BaseRev != "" {
		parts = append(parts, fmt.Sprintf("base_rev=%s", report.BaseRev))
	}

	return bead.BeadEvent{
		Kind:      "execute-bead",
		Summary:   report.Status,
		Body:      strings.Join(parts, "\n"),
		Actor:     actor,
		Source:    "ddx agent execute-loop",
		CreatedAt: createdAt,
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
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

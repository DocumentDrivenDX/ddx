package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

type ExecuteBeadReport struct {
	BeadID      string `json:"bead_id"`
	AttemptID   string `json:"attempt_id,omitempty"`
	WorkerID    string `json:"worker_id,omitempty"`
	Harness     string `json:"harness,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	Status      string `json:"status"`
	Detail      string `json:"detail,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	BaseRev     string `json:"base_rev,omitempty"`
	ResultRev   string `json:"result_rev,omitempty"`
	PreserveRef string `json:"preserve_ref,omitempty"`
	RetryAfter  string `json:"retry_after,omitempty"`
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
	Heartbeat(id string) error
	CloseWithEvidence(id, sessionID, commitSHA string) error
	AppendEvent(id string, event bead.BeadEvent) error
	SetExecutionCooldown(id string, until time.Time, status, detail string) error
	IncrNoChangesCount(id string) (int, error)
}

type ExecuteBeadLoopOptions struct {
	Assignee                string
	Once                    bool
	PollInterval            time.Duration
	NoProgressCooldown      time.Duration
	MaxNoChangesBeforeClose int
	// HeartbeatInterval, if > 0, overrides bead.HeartbeatInterval for this
	// worker's claim heartbeat loop. Tests use this to shorten the tick.
	HeartbeatInterval time.Duration
	Log               io.Writer
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
	noProgressCooldown := opts.NoProgressCooldown
	if noProgressCooldown <= 0 {
		noProgressCooldown = 6 * time.Hour
	}
	maxNoChangesBeforeClose := opts.MaxNoChangesBeforeClose
	if maxNoChangesBeforeClose <= 0 {
		maxNoChangesBeforeClose = 3
	}
	heartbeatInterval := opts.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = bead.HeartbeatInterval
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

		hbCtx, hbCancel := context.WithCancel(ctx)
		var hbWG sync.WaitGroup
		hbWG.Add(1)
		go func(beadID string) {
			defer hbWG.Done()
			ticker := time.NewTicker(heartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-hbCtx.Done():
					return
				case <-ticker.C:
					_ = w.Store.Heartbeat(beadID)
				}
			}
		}(candidate.ID)

		report, err := w.Executor.Execute(ctx, candidate.ID)
		hbCancel()
		hbWG.Wait()
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
			if report.Status == ExecuteBeadStatusNoChanges {
				count, cerr := w.Store.IncrNoChangesCount(candidate.ID)
				if cerr != nil {
					return result, cerr
				}
				if count >= maxNoChangesBeforeClose {
					// Bead has returned no_changes on every attempt up to the
					// threshold. Treat it as already-satisfied: close with the
					// accumulated no-changes evidence so the queue can drain.
					if cerr := w.Store.CloseWithEvidence(candidate.ID, report.SessionID, report.BaseRev); cerr != nil {
						return result, cerr
					}
					report.Status = ExecuteBeadStatusAlreadySatisfied
					result.Successes++
					result.LastSuccessAt = now().UTC()
				} else {
					if shouldSuppressNoProgress(report) {
						retryAfter := now().UTC().Add(noProgressCooldown)
						if cerr := w.Store.SetExecutionCooldown(candidate.ID, retryAfter, report.Status, report.Detail); cerr != nil {
							return result, cerr
						}
						report.RetryAfter = retryAfter.Format(time.RFC3339)
					}
					result.Failures++
					result.LastFailureStatus = report.Status
				}
			} else {
				if shouldSuppressNoProgress(report) {
					retryAfter := now().UTC().Add(noProgressCooldown)
					if err := w.Store.SetExecutionCooldown(candidate.ID, retryAfter, report.Status, report.Detail); err != nil {
						return result, err
					}
					report.RetryAfter = retryAfter.Format(time.RFC3339)
				}
				result.Failures++
				result.LastFailureStatus = report.Status
			}
		}

		result.Results = append(result.Results, report)

		if err := w.Store.AppendEvent(candidate.ID, executeBeadLoopEvent(report, assignee, now().UTC())); err != nil {
			return result, err
		}

		if opts.Log != nil {
			resultStr := report.Status
			if report.ResultRev != "" {
				shortRev := report.ResultRev
				if len(shortRev) > 8 {
					shortRev = shortRev[:8]
				}
				resultStr = fmt.Sprintf("%s (%s)", report.Status, shortRev)
			}
			if report.Detail != "" && report.Detail != report.Status {
				resultStr = fmt.Sprintf("%s: %s", resultStr, report.Detail)
			}
			_, _ = fmt.Fprintf(opts.Log, "✓ %s → %s\n", candidate.ID, resultStr)
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
	if report.RetryAfter != "" {
		parts = append(parts, fmt.Sprintf("retry_after=%s", report.RetryAfter))
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

func shouldSuppressNoProgress(report ExecuteBeadReport) bool {
	if report.BaseRev == "" || report.ResultRev == "" {
		return false
	}
	return report.BaseRev == report.ResultRev
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

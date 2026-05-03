package graphql

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ExecLogProvider fetches execution run logs by run ID. The workingDir
// argument identifies the project root the lookup should be scoped to;
// LAYER 2 of the GraphQL multi-project fix (ddx-055e8d32) threads this
// per-request value via context so the singleton resolver can serve any
// registered project. Implementations should return a non-nil error when the
// run is not yet available so the resolver can retry.
type ExecLogProvider interface {
	GetExecLog(workingDir, runID string) (stdout, stderr string, err error)
}

// CoordinatorMetricsSnap is a point-in-time snapshot of land-coordinator
// metrics. Defined here so the server package can satisfy the
// CoordinatorMetricsProvider interface without an import cycle.
type CoordinatorMetricsSnap struct {
	Landed          int64
	Preserved       int64
	Failed          int64
	PushFailed      int64
	TotalDurationMS int64
	TotalCommits    int64
}

// CoordinatorMetricsProvider can fetch a coordinator metrics snapshot for a
// given project root. Returns nil if no coordinator has been started for that
// project yet.
type CoordinatorMetricsProvider interface {
	GetCoordinatorMetrics(projectRoot string) *CoordinatorMetricsSnap
}

// ExecutionEvidence is the resolver for the executionEvidence subscription.
// It polls ExecLogProvider until the run log is available, then streams each
// stdout and stderr line as a separate ExecutionEvent and closes the channel.
func (r *subscriptionResolver) ExecutionEvidence(ctx context.Context, runID string) (<-chan *ExecutionEvent, error) {
	if r.ExecLogs == nil {
		return nil, fmt.Errorf("subscription not available: execution log provider not configured")
	}

	out := make(chan *ExecutionEvent, 32)
	go func() {
		defer close(out)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			stdout, stderr, err := r.ExecLogs.GetExecLog(r.workingDir(ctx), runID)
			if err == nil {
				now := time.Now().UTC().Format(time.RFC3339)
				seq := 0
				emit := func(stream, line string) bool {
					evt := &ExecutionEvent{
						EventID:   fmt.Sprintf("%s-%d", runID, seq),
						RunID:     runID,
						Stream:    stream,
						Line:      line,
						Timestamp: now,
					}
					seq++
					select {
					case out <- evt:
						return true
					case <-ctx.Done():
						return false
					}
				}
				for _, line := range splitLines(stdout) {
					if !emit("stdout", line) {
						return
					}
				}
				for _, line := range splitLines(stderr) {
					if !emit("stderr", line) {
						return
					}
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return out, nil
}

// CoordinatorMetrics is the resolver for the coordinatorMetrics subscription.
// It polls CoordinatorMetricsProvider once per MetricsPollInterval (default 1s)
// and emits a CoordinatorMetricsUpdate whenever the snapshot for projectRoot
// changes.
func (r *subscriptionResolver) CoordinatorMetrics(ctx context.Context, projectRoot string) (<-chan *CoordinatorMetricsUpdate, error) {
	if r.CoordMetrics == nil {
		return nil, fmt.Errorf("subscription not available: coordinator metrics provider not configured")
	}

	interval := r.MetricsPollInterval
	if interval <= 0 {
		interval = time.Second
	}

	out := make(chan *CoordinatorMetricsUpdate, 16)
	go func() {
		defer close(out)
		var last CoordinatorMetricsSnap
		hasPrev := false
		seq := 0
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				snap := r.CoordMetrics.GetCoordinatorMetrics(projectRoot)
				if snap == nil {
					continue
				}
				if hasPrev && *snap == last {
					continue
				}
				hasPrev = true
				last = *snap
				seq++
				ts := time.Now().UTC().Format(time.RFC3339)
				update := &CoordinatorMetricsUpdate{
					UpdateID:        fmt.Sprintf("%s-update-%d", projectRoot, seq),
					ProjectRoot:     projectRoot,
					Timestamp:       ts,
					Landed:          int(snap.Landed),
					Preserved:       int(snap.Preserved),
					Failed:          int(snap.Failed),
					PushFailed:      int(snap.PushFailed),
					TotalDurationMs: int(snap.TotalDurationMS),
					TotalCommits:    int(snap.TotalCommits),
				}
				select {
				case out <- update:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// splitLines splits s by newlines, returning nil for empty strings.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

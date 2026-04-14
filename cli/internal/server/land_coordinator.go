package server

// land_coordinator.go — per-project land coordinator goroutine.
//
// The land coordinator is the server-side implementation of the human-PR
// landing model. For each project, a single goroutine owns all writes to the
// project's target refs. Workers (goroutines running ExecuteBead) submit
// their completed results to the coordinator via a channel; the coordinator
// drains the channel in FIFO order and invokes agent.Land() for each
// submission.
//
// Contract:
//   - Exactly one coordinator goroutine per projectRoot. Lazily created on
//     first submission and cached in WorkerManager so subsequent runWorker
//     invocations reuse it.
//   - Submissions block the submitter until the coordinator returns a
//     LandResult. This is a channel-based future/promise pattern.
//   - The coordinator does NOT share state with other projects'
//     coordinators. Full isolation per projectRoot.
//   - The coordinator goroutine survives individual runWorker invocations.
//     It exits only when the WorkerManager is stopped (not implemented
//     here — coordinators live for the lifetime of the server process).
//
// Why: see ddx-8746d8a6 / ddx-e14efc58 / ddx-6aa50e57.

import (
	"fmt"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// LandOutcomeSummary is one entry in the coordinator's last-N submissions log.
type LandOutcomeSummary struct {
	TS          time.Time `json:"ts"`
	BeadID      string    `json:"bead_id,omitempty"`
	AttemptID   string    `json:"attempt_id,omitempty"`
	Outcome     string    `json:"outcome"` // landed | preserved | failed | push_failed
	DurationMS  int64     `json:"duration_ms"`
	RebaseCount int       `json:"rebase_count"` // 0 on fast-forward, N on rebase path
}

// CoordinatorMetrics holds all in-memory metrics for one LandCoordinator.
// All fields are protected by the owning LandCoordinator's metricsMu.
type CoordinatorMetrics struct {
	// Outcome counters
	Landed     int64 `json:"landed"`
	Preserved  int64 `json:"preserved"`
	Failed     int64 `json:"failed"`
	PushFailed int64 `json:"push_failed"`
	// Total duration in milliseconds (sum, for computing avg)
	TotalDurationMS int64 `json:"total_duration_ms"`
	// Total rebase commit count (sum)
	TotalRebaseCommits int64 `json:"total_rebase_commits"`
	// PreservedRatio is (preserved+failed) / total over the last window
	PreservedRatio float64 `json:"preserved_ratio"`
	// LastSubmissions holds up to the last 10 outcomes
	LastSubmissions []LandOutcomeSummary `json:"last_submissions,omitempty"`
}

// LandSubmission is one worker-to-coordinator submission. Submissions block
// on replyCh until the coordinator has processed them and pushed the
// LandResult back.
type LandSubmission struct {
	Request agent.LandRequest
	replyCh chan landReply
}

type landReply struct {
	result *agent.LandResult
	err    error
}

// LandCoordinator owns a single goroutine that serializes agent.Land() calls
// for one projectRoot. Create via newLandCoordinator; lifetime is tied to
// the owning WorkerManager.
type LandCoordinator struct {
	projectRoot string
	gitOps      agent.LandingGitOps
	queue       chan *LandSubmission
	done        chan struct{}

	metricsMu sync.Mutex
	metrics   CoordinatorMetrics
}

// NewLocalLandCoordinator returns a process-local LandCoordinator for use
// by CLI commands such as `ddx agent execute-loop --local`. It has the same
// single-writer semantics as the server-hosted coordinator but its lifetime
// is tied to the CLI invocation. Stop() should be called on function exit.
func NewLocalLandCoordinator(projectRoot string, gitOps agent.LandingGitOps) *LandCoordinator {
	return newLandCoordinator(projectRoot, gitOps)
}

func newLandCoordinator(projectRoot string, gitOps agent.LandingGitOps) *LandCoordinator {
	if gitOps == nil {
		gitOps = agent.RealLandingGitOps{}
	}
	c := &LandCoordinator{
		projectRoot: projectRoot,
		gitOps:      gitOps,
		// Buffered so a single-worker happy path does not block on a
		// channel handoff. Additional submissions queue up naturally.
		queue: make(chan *LandSubmission, 32),
		done:  make(chan struct{}),
	}
	go c.run()
	return c
}

// Submit sends req to the coordinator and blocks until the LandResult is
// available. Safe to call concurrently from any number of goroutines — the
// coordinator processes submissions in FIFO order.
func (c *LandCoordinator) Submit(req agent.LandRequest) (*agent.LandResult, error) {
	sub := &LandSubmission{
		Request: req,
		replyCh: make(chan landReply, 1),
	}
	select {
	case c.queue <- sub:
	case <-c.done:
		return nil, fmt.Errorf("land coordinator for %s has stopped", c.projectRoot)
	}
	reply := <-sub.replyCh
	return reply.result, reply.err
}

// Stop closes the submission queue and signals the coordinator goroutine to
// exit after draining any in-flight submissions. Intended for test cleanup
// and process shutdown; not currently called from the server HTTP path.
func (c *LandCoordinator) Stop() {
	select {
	case <-c.done:
		return
	default:
	}
	close(c.done)
	close(c.queue)
}

// run is the coordinator goroutine. It drains queue in FIFO order, calling
// agent.Land() for each submission. One submission at a time — this is the
// single-writer guarantee for target-ref writes on projectRoot.
func (c *LandCoordinator) run() {
	for sub := range c.queue {
		start := time.Now()
		result, err := agent.Land(c.projectRoot, sub.Request, c.gitOps)
		elapsed := time.Since(start)
		c.recordMetrics(sub.Request, result, err, elapsed)
		sub.replyCh <- landReply{result: result, err: err}
	}
}

// recordMetrics updates the in-memory metrics after a Land() call.
func (c *LandCoordinator) recordMetrics(req agent.LandRequest, result *agent.LandResult, err error, elapsed time.Duration) {
	const maxLastSubmissions = 10

	outcome := "failed"
	rebaseCount := 0
	if err == nil && result != nil {
		switch result.Status {
		case "landed":
			if result.PushFailed {
				outcome = "push_failed"
			} else {
				outcome = "landed"
			}
			rebaseCount = result.RebaseCommitCount
		case "preserved":
			outcome = "preserved"
			rebaseCount = result.RebaseCommitCount
		default:
			outcome = "failed"
		}
	}

	elapsedMS := elapsed.Milliseconds()

	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	switch outcome {
	case "landed":
		c.metrics.Landed++
	case "preserved":
		c.metrics.Preserved++
	case "failed":
		c.metrics.Failed++
	case "push_failed":
		c.metrics.PushFailed++
		c.metrics.Landed++ // push_failed still landed locally
	}
	c.metrics.TotalDurationMS += elapsedMS
	c.metrics.TotalRebaseCommits += int64(rebaseCount)

	total := c.metrics.Landed + c.metrics.Preserved + c.metrics.Failed
	if total > 0 {
		c.metrics.PreservedRatio = float64(c.metrics.Preserved+c.metrics.Failed) / float64(total)
	}

	entry := LandOutcomeSummary{
		TS:          time.Now().UTC(),
		BeadID:      req.BeadID,
		AttemptID:   req.AttemptID,
		Outcome:     outcome,
		DurationMS:  elapsedMS,
		RebaseCount: rebaseCount,
	}
	c.metrics.LastSubmissions = append(c.metrics.LastSubmissions, entry)
	if len(c.metrics.LastSubmissions) > maxLastSubmissions {
		c.metrics.LastSubmissions = c.metrics.LastSubmissions[len(c.metrics.LastSubmissions)-maxLastSubmissions:]
	}
}

// Metrics returns a snapshot of the coordinator's current metrics.
// Safe to call concurrently.
func (c *LandCoordinator) Metrics() CoordinatorMetrics {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()
	// Copy slice to avoid data races on the caller side
	m := c.metrics
	if len(c.metrics.LastSubmissions) > 0 {
		m.LastSubmissions = make([]LandOutcomeSummary, len(c.metrics.LastSubmissions))
		copy(m.LastSubmissions, c.metrics.LastSubmissions)
	}
	return m
}

// coordinatorRegistry is the WorkerManager's per-project coordinator cache.
// It is lazily populated on first Submit call for a given projectRoot and
// persists for the lifetime of the process.
type coordinatorRegistry struct {
	mu           sync.Mutex
	coordinators map[string]*LandCoordinator
	// gitOpsOverride, when non-nil, is injected into each newly created
	// coordinator instead of RealLandingGitOps. Tests use this.
	gitOpsOverride agent.LandingGitOps
}

func newCoordinatorRegistry() *coordinatorRegistry {
	return &coordinatorRegistry{
		coordinators: map[string]*LandCoordinator{},
	}
}

// Get returns the coordinator for projectRoot, creating one on first access.
// Safe to call concurrently.
func (r *coordinatorRegistry) Get(projectRoot string) *LandCoordinator {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.coordinators[projectRoot]; ok {
		return c
	}
	c := newLandCoordinator(projectRoot, r.gitOpsOverride)
	r.coordinators[projectRoot] = c
	return c
}

// StopAll stops every coordinator in the registry. For test cleanup.
func (r *coordinatorRegistry) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.coordinators {
		c.Stop()
	}
	r.coordinators = map[string]*LandCoordinator{}
}

// CoordinatorMetricsEntry is one project's coordinator metrics plus its
// project root, for the /api/agent/coordinators endpoint.
type CoordinatorMetricsEntry struct {
	ProjectRoot string             `json:"project_root"`
	Metrics     CoordinatorMetrics `json:"metrics"`
}

// MetricsFor returns a pointer to a metrics snapshot for the given projectRoot
// if a coordinator has already been created for it, or nil otherwise. This
// does NOT create a coordinator as a side effect.
func (r *coordinatorRegistry) MetricsFor(projectRoot string) *CoordinatorMetrics {
	r.mu.Lock()
	c, ok := r.coordinators[projectRoot]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	m := c.Metrics()
	return &m
}

// AllMetrics returns a snapshot of metrics for every coordinator in the registry.
func (r *coordinatorRegistry) AllMetrics() []CoordinatorMetricsEntry {
	r.mu.Lock()
	roots := make([]string, 0, len(r.coordinators))
	coords := make(map[string]*LandCoordinator, len(r.coordinators))
	for root, c := range r.coordinators {
		roots = append(roots, root)
		coords[root] = c
	}
	r.mu.Unlock()

	out := make([]CoordinatorMetricsEntry, 0, len(roots))
	for _, root := range roots {
		out = append(out, CoordinatorMetricsEntry{
			ProjectRoot: root,
			Metrics:     coords[root].Metrics(),
		})
	}
	return out
}

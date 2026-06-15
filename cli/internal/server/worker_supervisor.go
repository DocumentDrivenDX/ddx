package server

import (
	"errors"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
)

// Worker state labels. running/stopping are non-terminal; the rest are terminal.
const (
	workerStateRunning  = "running"
	workerStateStopping = "stopping"
	workerStateStopped  = "stopped"
)

// isTerminalWorkerState reports whether a worker state is a final resting state
// (the worker is no longer doing work). "stopping" is excluded — it is a
// transient state on the way to "stopped".
func isTerminalWorkerState(state string) bool {
	switch state {
	case "exited", "failed", "reaped", workerStateStopped:
		return true
	}
	return false
}

// workerController is the subset of *WorkerManager the supervisor drives.
// Defined as an interface so reconcile logic can be tested against a fake
// controller without spawning real worker goroutines. *WorkerManager
// implements it.
type workerController interface {
	StartExecuteLoop(spec ExecuteLoopWorkerSpec) (WorkerRecord, error)
	Stop(id string) error
	List() ([]WorkerRecord, error)
	// HasLiveWorker reports whether the controller currently holds a live
	// in-memory handle (goroutine) for id. False for disk-only records left
	// behind by a previous server run.
	HasLiveWorker(id string) bool
}

var _ workerController = (*WorkerManager)(nil)

// managedWorker is the supervisor's in-memory record of a worker it started.
type managedWorker struct {
	spec      ExecuteLoopWorkerSpec
	startedAt time.Time
}

// ReconcileResult records the actions a single Reconcile pass took. It is
// returned for observability and asserted in tests.
type ReconcileResult struct {
	Started        []string `json:"started,omitempty"`
	Restarted      []string `json:"restarted,omitempty"`
	Stopped        []string `json:"stopped,omitempty"`
	StaleMarked    []string `json:"stale_marked,omitempty"`
	RestartSkipped []string `json:"restart_skipped,omitempty"`
	Errors         []string `json:"errors,omitempty"`
}

// WorkerSupervisor keeps the number of running server-managed workers for a
// project equal to the desired_count persisted in .ddx/workers/desired.json.
// It starts missing workers, stops excess workers, marks stale disk records
// stopped, and restarts unexpectedly-exited workers subject to the restart
// policy.
type WorkerSupervisor struct {
	projectRoot string
	ctrl        workerController

	// RestartPauseCheck, when non-nil, gates restarts on an external blocker
	// such as a dirty project root or an operator-attention state. When it
	// returns paused=true the restart pass is skipped with the given reason.
	RestartPauseCheck func() (paused bool, reason string)

	// clock is overridable in tests; nil uses time.Now().UTC().
	clock func() time.Time

	mu sync.Mutex
	// managed holds the workers this supervisor started in this process.
	managed map[string]managedWorker
	// pendingRestart counts crashed managed workers awaiting restart. It
	// survives between reconcile passes so a restart held back by backoff or
	// max-restarts is retried later instead of falling through to
	// unconditional initial provisioning.
	pendingRestart int
	// restarts records restart timestamps for max_restarts_per_hour accounting.
	restarts []time.Time
	// backoffUntil is the earliest time the next restart may occur.
	backoffUntil time.Time
}

// NewWorkerSupervisor builds a supervisor for projectRoot driving ctrl.
func NewWorkerSupervisor(projectRoot string, ctrl workerController) *WorkerSupervisor {
	return &WorkerSupervisor{
		projectRoot: projectRoot,
		ctrl:        ctrl,
		managed:     map[string]managedWorker{},
	}
}

func (s *WorkerSupervisor) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now().UTC()
}

// loadDesired reads the desired-state file, treating an absent file as
// "manage nothing" (desired_count 0).
func (s *WorkerSupervisor) loadDesired() (*WorkerDesiredState, error) {
	state, err := LoadWorkerDesiredState(s.projectRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &WorkerDesiredState{Version: WorkerDesiredStateVersion}, nil
		}
		return nil, err
	}
	return state, nil
}

// Reconcile brings the actual set of server-managed workers in line with the
// persisted desired state: it marks stale disk records stopped, restarts
// crashed managed workers (policy permitting), starts missing workers, and
// stops the newest excess workers.
func (s *WorkerSupervisor) Reconcile() (ReconcileResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result ReconcileResult

	desired, err := s.loadDesired()
	if err != nil {
		return result, err
	}
	recs, err := s.ctrl.List()
	if err != nil {
		return result, err
	}
	byID := make(map[string]WorkerRecord, len(recs))
	for _, r := range recs {
		byID[r.ID] = r
	}
	now := s.now()

	// Stale pass: a running/stopping disk record we do not manage and that has
	// no live in-memory worker is reconciled to stopped. It is never adopted.
	for _, r := range recs {
		if _, ok := s.managed[r.ID]; ok {
			continue
		}
		if (r.State == workerStateRunning || r.State == workerStateStopping) && !s.ctrl.HasLiveWorker(r.ID) {
			if stopErr := s.ctrl.Stop(r.ID); stopErr == nil {
				result.StaleMarked = append(result.StaleMarked, r.ID)
			} else {
				result.Errors = append(result.Errors, stopErr.Error())
			}
		}
	}

	// Classify managed workers into healthy-running and crashed.
	running := 0
	for id := range s.managed {
		r, ok := byID[id]
		switch {
		case !ok:
			// The record vanished entirely — treat as an unexpected exit.
			delete(s.managed, id)
			s.pendingRestart++
		case isTerminalWorkerState(r.State):
			delete(s.managed, id)
			if r.State == workerStateStopped {
				// Operator-initiated stop — not a restart candidate.
				continue
			}
			s.pendingRestart++
		case !s.ctrl.HasLiveWorker(id):
			// Disk says running but the goroutine is gone — crashed.
			delete(s.managed, id)
			s.pendingRestart++
		default:
			running++
		}
	}

	// Restart pass: replace crashed managed workers, gated by restart policy.
	for s.pendingRestart > 0 && running < desired.DesiredCount {
		ok, reason := s.restartAllowed(desired.Restart, now)
		if !ok {
			result.RestartSkipped = append(result.RestartSkipped, reason)
			break
		}
		rec, startErr := s.startWorker(desired)
		if startErr != nil {
			result.Errors = append(result.Errors, startErr.Error())
			break
		}
		s.restarts = append(s.restarts, now)
		s.backoffUntil = now.Add(s.backoffDuration(desired.Restart))
		s.pendingRestart--
		running++
		result.Restarted = append(result.Restarted, rec.ID)
	}

	// Initial provisioning: only when no crashed slot is being held back by
	// restart policy. These are first-time starts, not restarts, so they are
	// not policy-gated.
	if s.pendingRestart == 0 {
		for running < desired.DesiredCount {
			rec, startErr := s.startWorker(desired)
			if startErr != nil {
				result.Errors = append(result.Errors, startErr.Error())
				break
			}
			running++
			result.Started = append(result.Started, rec.ID)
		}
	}

	// Excess: stop the newest managed running workers down to desired_count.
	if running > desired.DesiredCount {
		excess := running - desired.DesiredCount
		for _, id := range s.newestManagedIDs(excess) {
			if stopErr := s.ctrl.Stop(id); stopErr == nil {
				delete(s.managed, id)
				result.Stopped = append(result.Stopped, id)
			} else {
				result.Errors = append(result.Errors, stopErr.Error())
			}
		}
	}

	return result, nil
}

// startWorker expands the default spec and starts a worker through the
// controller, recording it as managed.
func (s *WorkerSupervisor) startWorker(desired *WorkerDesiredState) (WorkerRecord, error) {
	spec := desired.DefaultSpec.toExecuteLoopSpec(s.projectRoot)
	rec, err := s.ctrl.StartExecuteLoop(spec)
	if err != nil {
		return WorkerRecord{}, err
	}
	s.managed[rec.ID] = managedWorker{spec: spec, startedAt: rec.StartedAt}
	return rec, nil
}

// restartAllowed reports whether a crashed managed worker may be restarted
// now under policy, returning a human-readable reason when it may not.
func (s *WorkerSupervisor) restartAllowed(policy WorkerRestartPolicy, now time.Time) (bool, string) {
	if !policy.Enabled {
		return false, "restart disabled"
	}
	if s.RestartPauseCheck != nil {
		if paused, reason := s.RestartPauseCheck(); paused {
			if reason == "" {
				reason = "restart paused"
			}
			return false, reason
		}
	}
	if now.Before(s.backoffUntil) {
		return false, "within backoff window"
	}
	if policy.MaxRestartsPerHour > 0 && s.restartsInLastHour(now) >= policy.MaxRestartsPerHour {
		return false, "max restarts per hour reached"
	}
	return true, ""
}

func (s *WorkerSupervisor) restartsInLastHour(now time.Time) int {
	cutoff := now.Add(-time.Hour)
	n := 0
	for _, t := range s.restarts {
		if t.After(cutoff) {
			n++
		}
	}
	return n
}

func (s *WorkerSupervisor) backoffDuration(policy WorkerRestartPolicy) time.Duration {
	if policy.Backoff == "" {
		return 0
	}
	d, err := time.ParseDuration(policy.Backoff)
	if err != nil {
		return 0
	}
	return d
}

// newestManagedIDs returns up to n managed worker IDs ordered newest-first by
// start time.
func (s *WorkerSupervisor) newestManagedIDs(n int) []string {
	ids := make([]string, 0, len(s.managed))
	for id := range s.managed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return s.managed[ids[i]].startedAt.After(s.managed[ids[j]].startedAt)
	})
	if n > len(ids) {
		n = len(ids)
	}
	return ids[:n]
}

// toExecuteLoopSpec expands a persisted default spec into the ExecuteLoopSpec
// the worker manager consumes, defaulting Mode to watch for a long-running
// drain worker.
func (d WorkerDefaultSpec) toExecuteLoopSpec(projectRoot string) ExecuteLoopWorkerSpec {
	spec := executeloop.ExecuteLoopSpec{
		ProjectRoot: projectRoot,
		Harness:     d.Harness,
		Model:       d.Model,
		Profile:     d.Profile,
		Provider:    d.Provider,
		LabelFilter: d.LabelFilter,
		Mode:        executeloop.Mode(d.Mode),
	}
	if d.IdleInterval != "" {
		if dur, err := time.ParseDuration(d.IdleInterval); err == nil {
			spec.IdleInterval = executeloop.Duration{Duration: dur}
		}
	}
	if spec.Mode == "" {
		spec.Mode = executeloop.ModeWatch
	}
	return spec
}

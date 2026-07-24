package work

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

// LivenessReporter observes worker liveness ticks. WithHeartbeat invokes
// OnTick after each TouchClaimHeartbeat so the caller can mirror the same
// signal to a worker-status sidecar and to the optional server probe
// without rewriting the bead tracker.
type LivenessReporter interface {
	OnTick(now time.Time)
}

// SidecarLivenessReporter is the production LivenessReporter. Each tick it
// (a) writes a status.json sidecar under .ddx/workers/<worker-id>/ via the
// workerstatus package, and (b) emits a worker.heartbeat envelope to sink
// (typically the loop EventSink, which is teed to the workerprobe). The
// sidecar write is best-effort and never fails the loop; the sink emit is
// best-effort and never blocks the heartbeat goroutine.
type SidecarLivenessReporter struct {
	mu          sync.Mutex
	projectRoot string
	rec         workerstatus.LivenessRecord
	sink        io.Writer
	sessionID   string
	childProbe  func(route, harness, phase string) []workerstatus.ProviderChild
}

// NewSidecarLivenessReporter constructs a SidecarLivenessReporter for the
// given worker. projectRoot must be the project's git root (where .ddx/
// lives). sink may be nil for callers that don't want event mirroring.
func NewSidecarLivenessReporter(projectRoot, workerID, sessionID string, sink io.Writer) *SidecarLivenessReporter {
	return &SidecarLivenessReporter{
		projectRoot: projectRoot,
		sink:        sink,
		sessionID:   sessionID,
		rec: workerstatus.LivenessRecord{
			WorkerID:    workerID,
			ProjectRoot: projectRoot,
			PID:         os.Getpid(),
			StartedAt:   time.Now().UTC(),
		},
	}
}

// SetAttempt records the in-flight attempt identity. Called once per claim
// before WithHeartbeat starts so the first tick records the right bead.
func (r *SidecarLivenessReporter) SetAttempt(beadID, attemptID, phase, route, harness, model, profile string, childPID int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.rec.CurrentBead = beadID
	r.rec.AttemptID = attemptID
	r.rec.Phase = phase
	r.rec.Message = ""
	r.rec.Route = route
	r.rec.Harness = harness
	r.rec.Model = model
	r.rec.Profile = profile
	r.rec.ChildPID = childPID
	r.mu.Unlock()
}

// SetCandidateResolving publishes candidate-scoped resolving liveness without
// creating an implementation attempt identity. Used while preclaim
// decomposition runs: operators see the candidate bead ID and phase=resolving
// (plus provider-child observations via the child probe) before any
// implementation attempt_id is assigned.
func (r *SidecarLivenessReporter) SetCandidateResolving(beadID, harness, model, profile string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.rec.CurrentBead = beadID
	r.rec.AttemptID = ""
	r.rec.Phase = string(PhaseResolving)
	r.rec.Message = ""
	r.rec.Route = ""
	r.rec.Harness = harness
	r.rec.Model = model
	r.rec.Profile = profile
	r.rec.ChildPID = 0
	r.mu.Unlock()
}

// SetChildProbe installs a best-effort probe invoked on each heartbeat tick.
func (r *SidecarLivenessReporter) SetChildProbe(probe func(route, harness, phase string) []workerstatus.ProviderChild) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.childProbe = probe
	r.mu.Unlock()
}

// SwapChildProbe replaces the installed probe and returns the previous one so
// callers can restore it after a scoped lifecycle (e.g. preclaim decomposition).
func (r *SidecarLivenessReporter) SwapChildProbe(probe func(route, harness, phase string) []workerstatus.ProviderChild) func(route, harness, phase string) []workerstatus.ProviderChild {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	prev := r.childProbe
	r.childProbe = probe
	r.mu.Unlock()
	return prev
}

// SetWorkerState records a non-attempt worker state in the sidecar.
func (r *SidecarLivenessReporter) SetWorkerState(phase, message string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.rec.CurrentBead = ""
	r.rec.AttemptID = ""
	r.rec.Phase = phase
	r.rec.Message = message
	r.rec.Route = ""
	r.rec.Harness = ""
	r.rec.Model = ""
	r.rec.Profile = ""
	r.rec.ChildPID = 0
	r.rec.ProviderChildren = nil
	r.mu.Unlock()
}

// UpdateRoute overwrites the in-flight route fields once fizeau resolves them.
// Guarded on r.rec.CurrentBead == beadID; a mis-matched beadID is a no-op.
// Empty args do not blank existing values.
func (r *SidecarLivenessReporter) UpdateRoute(beadID, harness, model, route string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rec.CurrentBead != beadID {
		return
	}
	if harness != "" {
		r.rec.Harness = harness
	}
	if model != "" {
		r.rec.Model = model
	}
	if route != "" {
		r.rec.Route = route
	}
}

// ClearAttempt drops the in-flight attempt identity once the attempt
// terminates. The worker_id and project_root remain so an idle drain loop
// still presents as the same worker between attempts.
func (r *SidecarLivenessReporter) ClearAttempt() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.rec.CurrentBead = ""
	r.rec.AttemptID = ""
	r.rec.Phase = ""
	r.rec.Message = ""
	r.rec.Route = ""
	r.rec.Harness = ""
	r.rec.Model = ""
	r.rec.Profile = ""
	r.rec.ChildPID = 0
	r.rec.ProviderChildren = nil
	r.mu.Unlock()
}

// OnTick updates the sidecar with the new last_activity_at and mirrors a
// worker.heartbeat envelope to the configured sink (best-effort).
func (r *SidecarLivenessReporter) OnTick(now time.Time) {
	if r == nil {
		return
	}
	r.mu.Lock()
	probe := r.childProbe
	route := r.rec.Route
	harness := r.rec.Harness
	phase := r.rec.Phase
	hasAttempt := r.rec.CurrentBead != ""
	r.mu.Unlock()

	var children []workerstatus.ProviderChild
	if probe != nil && hasAttempt {
		children = probe(route, harness, phase)
	}

	r.mu.Lock()
	r.rec.LastActivityAt = now.UTC()
	r.rec.ProviderChildren = children
	snapshot := r.rec
	r.mu.Unlock()
	_ = workerstatus.WriteLiveness(r.projectRoot, snapshot.WorkerID, snapshot)
	r.emit(snapshot, now)
}

// Snapshot returns a copy of the current sidecar record.
func (r *SidecarLivenessReporter) Snapshot() workerstatus.LivenessRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rec
}

func (r *SidecarLivenessReporter) emit(rec workerstatus.LivenessRecord, now time.Time) {
	if r.sink == nil {
		return
	}
	entry := map[string]any{
		"session_id": r.sessionID,
		"type":       "worker.heartbeat",
		"ts":         now.UTC().Format(time.RFC3339Nano),
		"data": map[string]any{
			"worker_id":        rec.WorkerID,
			"bead_id":          rec.CurrentBead,
			"attempt_id":       rec.AttemptID,
			"phase":            rec.Phase,
			"message":          rec.Message,
			"route":            rec.Route,
			"harness":          rec.Harness,
			"model":            rec.Model,
			"profile":          rec.Profile,
			"last_activity_at": rec.LastActivityAt.Format(time.RFC3339Nano),
		},
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	line = append(line, '\n')
	_, _ = r.sink.Write(line)
}

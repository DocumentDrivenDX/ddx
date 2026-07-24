package agent

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

const (
	providerChildActionTerminated = "terminated"
	reasonSupersededProviderChild = "superseded_provider_child"
	reasonRunningPhaseGuard       = "running_phase_non_route_provider_child"
	reasonAttemptEnded            = "attempt_ended"
	providerChildCleanupArtifact  = "provider-children.json"
)

// runningProviderGuardInterval is how often the running-phase guard re-scans
// the worker's provider-CLI descendants while an attempt is in flight. Provider
// CLIs spawned after route resolution (the dogfooded Codex/Gemini fan-out) are
// reaped within one interval instead of surviving until attempt-end cleanup.
const runningProviderGuardInterval = 2 * time.Second

// routeHarnessDeadGrace is how long the active route's own process may be
// absent from the provider-child scan (after having been observed at least
// once) before SetHarnessDeadWatch declares the harness dead. Comfortably
// above one runningProviderGuardInterval scan so a single missed ps snapshot
// never false-positives, and well under the 30s/60s recovery SLAs a dead
// harness subprocess must meet (ddx-f2b7cf89 AC2/AC3).
const routeHarnessDeadGrace = 8 * time.Second

var providerCLINames = map[string]struct{}{
	"claude":   {},
	"codex":    {},
	"gemini":   {},
	"opencode": {},
	"pi":       {},
}

type providerChildProcess struct {
	PID       int
	PPID      int
	Provider  string
	Command   string
	StartedAt time.Time

	// OwnerProvider is the nearest provider-classified ancestor's name found
	// while walking this process's PPID chain up to (but not including) the
	// worker root, or "" if no such ancestor exists (the process is a direct
	// or sibling child of the worker rather than a descendant of another
	// provider CLI). OwnerProviderPID is that ancestor's PID, or 0.
	//
	// This distinguishes a provider CLI launched by the active route's own
	// harness process (e.g. a Claude fixture a Codex-run test suite spawns)
	// from a provider CLI launched directly by worker lifecycle or
	// model-discovery paths — the former is legitimate work-in-progress under
	// the active route and must survive the running-phase guard; the latter
	// is exactly the leak the guard exists to reap (ddx-44e89575).
	OwnerProviderPID int
	OwnerProvider    string
}

type providerChildReapRecord struct {
	PID        int     `json:"pid"`
	Provider   string  `json:"provider"`
	Command    string  `json:"command,omitempty"`
	AgeSeconds float64 `json:"age_seconds"`
	Action     string  `json:"action"`
	Reason     string  `json:"reason"`
}

type providerChildCleanupReport struct {
	AttemptID   string                       `json:"attempt_id"`
	BeadID      string                       `json:"bead_id"`
	Trigger     string                       `json:"trigger"`
	ActiveRoute string                       `json:"active_route,omitempty"`
	ScannedAt   time.Time                    `json:"scanned_at"`
	Survivors   []workerstatus.ProviderChild `json:"survivors,omitempty"`
	Reaped      []providerChildReapRecord    `json:"reaped,omitempty"`
	ScanErr     string                       `json:"scan_error,omitempty"`
}

var providerChildScanner = func(ctx context.Context, rootPID int, now time.Time) ([]providerChildProcess, error) {
	return scanProviderChildProcessesImpl(ctx, rootPID, now)
}

var terminateProviderChild = terminateProviderChildImpl

func providerForCommand(cmdline string) string {
	parts := strings.Fields(strings.TrimSpace(cmdline))
	if len(parts) == 0 {
		return ""
	}
	base := filepath.Base(parts[0])
	if _, ok := providerCLINames[base]; ok {
		return base
	}
	// Node-wrapper detection: "node /path/to/provider-cli ..." where argv[0]
	// is the Node.js runtime and argv[1] is the provider CLI path
	// (e.g. Gemini installed via npm: "node /home/linuxbrew/.linuxbrew/bin/gemini").
	if base == "node" && len(parts) >= 2 {
		if argBase := filepath.Base(parts[1]); argBase != "" && argBase != "." {
			if _, ok := providerCLINames[argBase]; ok {
				return argBase
			}
		}
	}
	return ""
}

func providerRouteLabel(provider, model string) string {
	route := strings.TrimSpace(provider)
	if route != "" && strings.TrimSpace(model) != "" {
		route += "/" + strings.TrimSpace(model)
	}
	return route
}

func routeOwnsProvider(provider string, routeTokens ...string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return false
	}
	for _, tok := range routeTokens {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			continue
		}
		if tok == p {
			return true
		}
		if seg := strings.SplitN(tok, "/", 2); len(seg) > 0 && seg[0] == p {
			return true
		}
	}
	return false
}

func scanProviderChildrenForStatus(ctx context.Context, rootPID int, routeLabel, harness, phase string, now time.Time) []workerstatus.ProviderChild {
	procs, err := providerChildScanner(ctx, rootPID, now)
	if err != nil || len(procs) == 0 {
		return nil
	}
	activeOwner := activeRouteOwner(routeLabel, harness)
	out := make([]workerstatus.ProviderChild, 0, len(procs))
	for _, proc := range procs {
		out = append(out, providerChildStatus(proc, routeLabel, harness, activeOwner, phase, now))
	}
	return out
}

// providerChildPIDSet records provider-child PIDs observed at a lifecycle
// baseline (for example immediately before preclaim decomposition).
func providerChildPIDSet(children []workerstatus.ProviderChild) map[int]struct{} {
	if len(children) == 0 {
		return nil
	}
	out := make(map[int]struct{}, len(children))
	for _, child := range children {
		if child.PID > 0 {
			out[child.PID] = struct{}{}
		}
	}
	return out
}

// filterProviderChildrenAfterBaseline drops children that were already present
// at the pre-hook baseline. Combined with a rootPID-scoped scanner, this keeps
// preclaim candidate metadata free of processes that predate the hook or
// belong to another worker.
func filterProviderChildrenAfterBaseline(children []workerstatus.ProviderChild, baseline map[int]struct{}) []workerstatus.ProviderChild {
	if len(children) == 0 {
		return nil
	}
	if len(baseline) == 0 {
		out := make([]workerstatus.ProviderChild, len(children))
		copy(out, children)
		return out
	}
	out := make([]workerstatus.ProviderChild, 0, len(children))
	for _, child := range children {
		if _, seen := baseline[child.PID]; seen {
			continue
		}
		out = append(out, child)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// providerChildStatus renders one scanned provider process as a status entry,
// marking it route-owned (RouteOwner set) when its own provider matches the
// active route, harness-owned (RouteOwner + HarnessOwned set) when its
// nearest provider-classified ancestor matches the active route instead, or,
// when the active route is known and neither holds, non-route with an
// operator-facing diagnostic. Non-route children observed without a known
// route are reported plain so an early scan never mislabels a child the
// guard cannot yet judge.
func providerChildStatus(proc providerChildProcess, routeLabel, harness, activeOwner, phase string, now time.Time) workerstatus.ProviderChild {
	child := workerstatus.ProviderChild{
		PID:        proc.PID,
		Provider:   proc.Provider,
		Harness:    proc.Provider,
		Phase:      phase,
		AgeSeconds: childAgeSeconds(proc, now),
	}
	if routeOwnsProvider(proc.Provider, routeLabel, harness) {
		child.RouteOwner = activeOwner
		return child
	}
	if routeOwnsProvider(proc.OwnerProvider, routeLabel, harness) {
		child.RouteOwner = activeOwner
		child.HarnessOwned = true
		return child
	}
	if activeOwner != "" {
		child.NonRoute = true
		child.Diagnostic = nonRouteProviderDiagnostic(proc.Provider, activeOwner)
	}
	return child
}

// activeRouteOwner is the human-facing owner label for the active route:
// the route label when present, otherwise the harness. Empty when neither is
// known (route not yet resolved).
func activeRouteOwner(routeLabel, harness string) string {
	if owner := strings.TrimSpace(routeLabel); owner != "" {
		return owner
	}
	return strings.TrimSpace(harness)
}

func nonRouteProviderDiagnostic(provider, owner string) string {
	if owner == "" {
		return "non-route provider " + provider + " terminated by running-phase guard"
	}
	return "non-route provider " + provider + " terminated by running-phase guard (active route " + owner + ")"
}

// runningProviderChildGuard performs one running-phase sweep of the worker's
// provider-CLI descendants. Children whose provider matches the active
// route/harness are preserved and reported as route-owned; children that are
// instead descendants of the active route's own process (e.g. a provider CLI
// a Codex-run pre-commit/test suite spawns as a fixture) are likewise
// preserved and reported as harness-owned. Every other provider CLI — a
// direct or sibling non-route leak — is terminated by process group (taking
// grandchildren such as Gemini's Node worker with it) and reported as
// non-route with an operator-attention diagnostic. When the active route is
// not yet known (route and harness both empty) the guard observes without
// reaping, so it never quarantines a child it cannot yet attribute. Returns
// the status view and the reap evidence records.
func runningProviderChildGuard(ctx context.Context, rootPID int, routeLabel, harness, phase string, now time.Time) ([]workerstatus.ProviderChild, []providerChildReapRecord) {
	procs, err := providerChildScanner(ctx, rootPID, now)
	if err != nil || len(procs) == 0 {
		return nil, nil
	}
	activeOwner := activeRouteOwner(routeLabel, harness)
	children := make([]workerstatus.ProviderChild, 0, len(procs))
	var reaped []providerChildReapRecord
	for _, proc := range procs {
		child := providerChildStatus(proc, routeLabel, harness, activeOwner, phase, now)
		if child.NonRoute {
			terminateProviderChild(proc.PID)
			reaped = append(reaped, providerChildReapRecord{
				PID:        proc.PID,
				Provider:   proc.Provider,
				Command:    proc.Command,
				AgeSeconds: child.AgeSeconds,
				Action:     providerChildActionTerminated,
				Reason:     reasonRunningPhaseGuard,
			})
		}
		children = append(children, child)
	}
	return children, reaped
}

// runningProviderGuard is the attempt-scoped watchdog that drives
// runningProviderChildGuard on a ticker for the duration of attemptBackend.Run.
// It is owned by ExecuteBeadWithConfig (the shared ddx work / ddx try attempt
// path). The active route is seeded from any pinned harness/model and refreshed
// via UpdateRoute when fizeau resolves the route mid-attempt.
type runningProviderGuard struct {
	projectRoot string
	beadID      string
	attemptID   string
	rootPID     int
	interval    time.Duration

	// harnessDeadGrace and onHarnessDead arm the harness-liveness watchdog
	// (see SetHarnessDeadWatch). Both zero-value (disabled) by default.
	harnessDeadGrace time.Duration
	onHarnessDead    func()

	mu                  sync.Mutex
	routeLabel          string
	harness             string
	reaped              []providerChildReapRecord
	harnessSeen         bool
	harnessMissingSince time.Time
	harnessDeadFired    bool
}

func newRunningProviderGuard(projectRoot, beadID, attemptID string, rootPID int) *runningProviderGuard {
	return &runningProviderGuard{
		projectRoot: projectRoot,
		beadID:      beadID,
		attemptID:   attemptID,
		rootPID:     rootPID,
		interval:    runningProviderGuardInterval,
	}
}

// UpdateRoute records the active route. Non-empty harness/route values overwrite
// the prior values; empty values are ignored so a partial resolution never
// blanks a previously known route.
func (g *runningProviderGuard) UpdateRoute(harness, provider, model string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if h := strings.TrimSpace(harness); h != "" {
		g.harness = h
	}
	if r := providerRouteLabel(provider, model); r != "" {
		g.routeLabel = r
	}
}

func (g *runningProviderGuard) snapshotRoute() (string, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.routeLabel, g.harness
}

// SetHarnessDeadWatch arms the harness-liveness watchdog: once the active
// route/harness's own process has been observed at least once in a scan,
// if it then disappears for >= grace, onDead is invoked exactly once (from
// a new goroutine, so onDead may itself block or call back into the guard
// without deadlocking the ticker). A grace <= 0 or nil onDead disables the
// watchdog (the default).
//
// This closes a gap the generic idle timeout in drainServiceEventsWithRenderer
// cannot: that timeout only fires on total event silence, and a harness
// subprocess dying mid-attempt can be masked indefinitely by unrelated
// provider-CLI noise (e.g. a non-route fallback spawn loop) that keeps
// resetting it. Watching the process tree directly for the specific
// route-owned process disappearing gives a bounded, noise-immune signal
// (ddx-f2b7cf89).
func (g *runningProviderGuard) SetHarnessDeadWatch(grace time.Duration, onDead func()) {
	if g == nil {
		return
	}
	g.harnessDeadGrace = grace
	g.onHarnessDead = onDead
}

// observeHarnessLiveness updates the harness-liveness state from one scan's
// children and fires onHarnessDead (once) if the route-owned process has
// been missing for >= harnessDeadGrace since it was last seen. It is a
// no-op until the route is known and the watchdog has been armed, and it
// never fires before the route-owned process has been observed at least
// once (so attempt startup, before the harness process spawns, is never
// mistaken for a death).
func (g *runningProviderGuard) observeHarnessLiveness(route, harness string, children []workerstatus.ProviderChild, now time.Time) {
	if g == nil || g.harnessDeadGrace <= 0 || g.onHarnessDead == nil {
		return
	}
	if activeRouteOwner(route, harness) == "" {
		return
	}
	seen := false
	for _, c := range children {
		if c.RouteOwner != "" {
			seen = true
			break
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if seen {
		g.harnessSeen = true
		g.harnessMissingSince = time.Time{}
		return
	}
	if !g.harnessSeen || g.harnessDeadFired {
		return
	}
	if g.harnessMissingSince.IsZero() {
		g.harnessMissingSince = now
		return
	}
	if now.Sub(g.harnessMissingSince) >= g.harnessDeadGrace {
		g.harnessDeadFired = true
		onDead := g.onHarnessDead
		go onDead()
	}
}

// Reaped returns a copy of the running-phase reap evidence accumulated so far,
// so attempt-end cleanup can fold it into the final provider-children.json.
func (g *runningProviderGuard) Reaped() []providerChildReapRecord {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]providerChildReapRecord(nil), g.reaped...)
}

// Start launches the watchdog goroutine and returns a stop function. The stop
// function is idempotent and safe to call after ctx is already done.
func (g *runningProviderGuard) Start(ctx context.Context) func() {
	if g == nil {
		return func() {}
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(g.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case t := <-ticker.C:
				g.tick(ctx, t.UTC())
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

func (g *runningProviderGuard) tick(ctx context.Context, now time.Time) {
	route, harness := g.snapshotRoute()
	children, reaped := runningProviderChildGuard(ctx, g.rootPID, route, harness, "running", now)
	g.observeHarnessLiveness(route, harness, children, now)
	if len(reaped) == 0 {
		return
	}
	g.mu.Lock()
	g.reaped = append(g.reaped, reaped...)
	cumulative := append([]providerChildReapRecord(nil), g.reaped...)
	g.mu.Unlock()
	writeProviderChildCleanupArtifact(g.projectRoot, g.attemptID, &providerChildCleanupReport{
		AttemptID:   g.attemptID,
		BeadID:      g.beadID,
		Trigger:     reasonRunningPhaseGuard,
		ActiveRoute: activeRouteOwner(route, harness),
		ScannedAt:   now,
		Reaped:      cumulative,
	})
}

func reapProviderChildren(ctx context.Context, rootPID int, now time.Time, reasonFor func(providerChildProcess) string) ([]providerChildReapRecord, []workerstatus.ProviderChild, error) {
	procs, err := providerChildScanner(ctx, rootPID, now)
	if err != nil {
		return nil, nil, err
	}
	var reaped []providerChildReapRecord
	var survivors []workerstatus.ProviderChild
	for _, proc := range procs {
		reason := reasonFor(proc)
		if reason == "" {
			survivors = append(survivors, workerstatus.ProviderChild{
				PID:        proc.PID,
				Provider:   proc.Provider,
				Harness:    proc.Provider,
				AgeSeconds: childAgeSeconds(proc, now),
			})
			continue
		}
		terminateProviderChild(proc.PID)
		reaped = append(reaped, providerChildReapRecord{
			PID:        proc.PID,
			Provider:   proc.Provider,
			Command:    proc.Command,
			AgeSeconds: childAgeSeconds(proc, now),
			Action:     providerChildActionTerminated,
			Reason:     reason,
		})
	}
	return reaped, survivors, nil
}

func reapSupersededProviderChildren(ctx context.Context, rootPID int, routeLabel, harness string, now time.Time) ([]providerChildReapRecord, []workerstatus.ProviderChild) {
	if strings.TrimSpace(routeLabel) == "" && strings.TrimSpace(harness) == "" {
		return nil, nil
	}
	reaped, survivors, err := reapProviderChildren(ctx, rootPID, now, func(proc providerChildProcess) string {
		if routeOwnsProvider(proc.Provider, routeLabel, harness) {
			return ""
		}
		return reasonSupersededProviderChild
	})
	if err != nil {
		return nil, nil
	}
	return reaped, survivors
}

func reapAllProviderChildren(ctx context.Context, rootPID int, now time.Time) []providerChildReapRecord {
	reaped, _, err := reapProviderChildren(ctx, rootPID, now, func(providerChildProcess) string {
		return reasonAttemptEnded
	})
	if err != nil {
		return nil
	}
	return reaped
}

func childAgeSeconds(proc providerChildProcess, now time.Time) float64 {
	if proc.StartedAt.IsZero() {
		return 0
	}
	age := now.Sub(proc.StartedAt)
	if age < 0 {
		return 0
	}
	return age.Seconds()
}

func writeProviderChildCleanupArtifact(projectRoot, attemptID string, report *providerChildCleanupReport) {
	if report == nil || strings.TrimSpace(projectRoot) == "" || strings.TrimSpace(attemptID) == "" {
		return
	}
	path := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, providerChildCleanupArtifact)
	_ = writeArtifactJSON(path, report)
}

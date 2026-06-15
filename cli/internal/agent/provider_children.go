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
	reasonDefunctProviderChild    = "defunct_provider_child"
	reasonAttemptEnded            = "attempt_ended"
	providerChildCleanupArtifact  = "provider-children.json"
)

// runningProviderGuardInterval is how often the running-phase guard re-scans
// the worker's provider-CLI descendants while an attempt is in flight. Provider
// CLIs spawned after route resolution (the dogfooded Codex/Gemini fan-out) are
// reaped within one interval instead of surviving until attempt-end cleanup.
const runningProviderGuardInterval = 2 * time.Second

var providerCLINames = map[string]struct{}{
	"claude":   {},
	"codex":    {},
	"gemini":   {},
	"opencode": {},
	"pi":       {},
}

var providerRouteAliases = map[string]string{
	"claude-code": "claude",
	"claude-tui":  "claude",
	"gemini-cli":  "gemini",
}

type providerChildProcess struct {
	PID       int
	PPID      int
	Provider  string
	Command   string
	CWD       string
	StartedAt time.Time
	Defunct   bool
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
	if strings.HasPrefix(base, "[") && strings.HasSuffix(base, "]") {
		base = strings.TrimSuffix(strings.TrimPrefix(base, "["), "]")
	}
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
		owner := routeTokenProvider(tok)
		if owner == "" {
			continue
		}
		if owner == p {
			return true
		}
	}
	return false
}

func routeTokenProvider(token string) string {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return ""
	}
	if seg := strings.SplitN(token, "/", 2); len(seg) > 0 {
		token = strings.TrimSpace(seg[0])
	}
	if alias, ok := providerRouteAliases[token]; ok {
		return alias
	}
	return token
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

// providerChildStatus renders one scanned provider process as a status entry,
// marking it route-owned (RouteOwner set) or, when the active route is known
// and the provider does not match it, non-route with an operator-facing
// diagnostic. Non-route children observed without a known route are reported
// plain so an early scan never mislabels a child the guard cannot yet judge.
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
// route/harness are preserved and reported as route-owned; every other provider
// CLI is terminated by process group (taking grandchildren such as Gemini's
// Node worker with it) and reported as non-route with an operator-attention
// diagnostic. When the active route is not yet known (route and harness both
// empty) the guard observes without reaping, so it never quarantines a child it
// cannot yet attribute. Returns the status view and the reap evidence records.
func runningProviderChildGuard(ctx context.Context, rootPID int, scopeDir, routeLabel, harness, phase string, now time.Time) ([]workerstatus.ProviderChild, []providerChildReapRecord) {
	procs, err := providerChildScanner(ctx, rootPID, now)
	if err != nil || len(procs) == 0 {
		return nil, nil
	}
	activeOwner := activeRouteOwner(routeLabel, harness)
	children := make([]workerstatus.ProviderChild, 0, len(procs))
	var reaped []providerChildReapRecord
	for _, proc := range procs {
		ownedByRoot := providerChildOwnedByRoot(proc, rootPID)
		if proc.Defunct && ownedByRoot {
			child := providerChildStatus(proc, routeLabel, harness, activeOwner, phase, now)
			terminateProviderChild(proc.PID)
			child.Phase = "defunct"
			child.Diagnostic = "defunct provider child reaped"
			reaped = append(reaped, providerChildReapRecord{
				PID:        proc.PID,
				Provider:   proc.Provider,
				Command:    proc.Command,
				AgeSeconds: child.AgeSeconds,
				Action:     providerChildActionTerminated,
				Reason:     reasonDefunctProviderChild,
			})
			children = append(children, child)
			continue
		}
		if !providerChildInScope(proc, scopeDir) {
			continue
		}
		child := providerChildStatus(proc, routeLabel, harness, activeOwner, phase, now)
		if !ownedByRoot {
			if child.NonRoute {
				child.Diagnostic = "nested provider child observed under active agent; not terminated"
			}
			children = append(children, child)
			continue
		}
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
	scopeDir    string
	interval    time.Duration

	mu         sync.Mutex
	routeLabel string
	harness    string
	reaped     []providerChildReapRecord
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

func (g *runningProviderGuard) SetScopeDir(scopeDir string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.scopeDir = strings.TrimSpace(scopeDir)
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

func (g *runningProviderGuard) snapshotScopeDir() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.scopeDir
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
	_, reaped := runningProviderChildGuard(ctx, g.rootPID, g.snapshotScopeDir(), route, harness, "running", now)
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

func reapProviderChildren(ctx context.Context, rootPID int, scopeDir string, now time.Time, reasonFor func(providerChildProcess) string) ([]providerChildReapRecord, []workerstatus.ProviderChild, error) {
	procs, err := providerChildScanner(ctx, rootPID, now)
	if err != nil {
		return nil, nil, err
	}
	var reaped []providerChildReapRecord
	var survivors []workerstatus.ProviderChild
	for _, proc := range procs {
		ownedByRoot := providerChildOwnedByRoot(proc, rootPID)
		if proc.Defunct && ownedByRoot {
			terminateProviderChild(proc.PID)
			reaped = append(reaped, providerChildReapRecord{
				PID:        proc.PID,
				Provider:   proc.Provider,
				Command:    proc.Command,
				AgeSeconds: childAgeSeconds(proc, now),
				Action:     providerChildActionTerminated,
				Reason:     reasonDefunctProviderChild,
			})
			continue
		}
		if !providerChildInScope(proc, scopeDir) {
			continue
		}
		if !ownedByRoot {
			survivors = append(survivors, workerstatus.ProviderChild{
				PID:        proc.PID,
				Provider:   proc.Provider,
				Harness:    proc.Provider,
				AgeSeconds: childAgeSeconds(proc, now),
				Diagnostic: "nested provider child observed under active agent; cleanup is root-owned",
			})
			continue
		}
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

func reapSupersededProviderChildren(ctx context.Context, rootPID int, scopeDir, routeLabel, harness string, now time.Time) ([]providerChildReapRecord, []workerstatus.ProviderChild) {
	if strings.TrimSpace(routeLabel) == "" && strings.TrimSpace(harness) == "" {
		return nil, nil
	}
	reaped, survivors, err := reapProviderChildren(ctx, rootPID, scopeDir, now, func(proc providerChildProcess) string {
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

func reapAllProviderChildren(ctx context.Context, rootPID int, scopeDir string, now time.Time) []providerChildReapRecord {
	reaped, _, err := reapProviderChildren(ctx, rootPID, scopeDir, now, func(providerChildProcess) string {
		return reasonAttemptEnded
	})
	if err != nil {
		return nil
	}
	return reaped
}

// ReapRootProviderChildrenInScope terminates provider CLI processes that are
// direct children of rootPID and whose cwd is within scopeDir. Status and model
// inventory surfaces use this after foreground Fizeau probes so server-owned
// probe CLIs do not survive without touching worker-owned agent children.
func ReapRootProviderChildrenInScope(ctx context.Context, rootPID int, scopeDir string) int {
	reaped := reapAllProviderChildren(ctx, rootPID, scopeDir, time.Now().UTC())
	return len(reaped)
}

func ReapDefunctRootProviderChildren(ctx context.Context, rootPID int) int {
	if rootPID <= 0 {
		return 0
	}
	procs, err := providerChildScanner(ctx, rootPID, time.Now().UTC())
	if err != nil {
		return 0
	}
	reaped := 0
	for _, proc := range procs {
		if !proc.Defunct || !providerChildOwnedByRoot(proc, rootPID) {
			continue
		}
		terminateProviderChild(proc.PID)
		reaped++
	}
	return reaped
}

func providerChildOwnedByRoot(proc providerChildProcess, rootPID int) bool {
	if rootPID <= 0 {
		return false
	}
	if proc.PPID == 0 {
		return true
	}
	return proc.PPID == rootPID
}

func providerChildInScope(proc providerChildProcess, scopeDir string) bool {
	scopeDir = strings.TrimSpace(scopeDir)
	if scopeDir == "" {
		return true
	}
	cwd := strings.TrimSpace(proc.CWD)
	if cwd == "" {
		return false
	}
	return isPathWithin(cwd, scopeDir)
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

package agent

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	monitorShellCleanupArtifact  = "monitor-shell-cleanup.json"
	reasonSelfMatchingMonitor    = "self_matching_monitor"
	monitorShellActionTerminated = "terminated"
	monitorShellScanInterval     = 2 * time.Second
)

// shellBinaryNames are the process binaries recognised as monitor shells.
var shellBinaryNames = map[string]struct{}{
	"sh":   {},
	"bash": {},
	"zsh":  {},
	"dash": {},
	"ksh":  {},
}

type monitorShellProcess struct {
	PID       int
	Command   string
	StartedAt time.Time
}

// MonitorShellRecord is a single cleaned-shell entry in the evidence artifact.
type MonitorShellRecord struct {
	PID        int     `json:"pid"`
	Command    string  `json:"command"`
	AgeSeconds float64 `json:"age_seconds"`
	Action     string  `json:"action"`
	Reason     string  `json:"reason"`
	Pattern    string  `json:"pattern,omitempty"`
}

type monitorShellCleanupReport struct {
	AttemptID string               `json:"attempt_id"`
	BeadID    string               `json:"bead_id"`
	ScannedAt time.Time            `json:"scanned_at"`
	Cleaned   []MonitorShellRecord `json:"cleaned"`
}

// monitorShellScanner is the platform-specific scanner; injectable in tests.
var monitorShellScanner = func(ctx context.Context, rootPID int, now time.Time) ([]monitorShellProcess, error) {
	return scanMonitorShellsImpl(ctx, rootPID, now)
}

// terminateMonitorShell terminates a specific monitor shell PID; injectable in tests.
var terminateMonitorShell = terminateMonitorShellImpl

// isSelfMatchingMonitor reports whether cmdline contains a `pgrep -f <pattern>`
// invocation where <pattern> also appears literally in cmdline (causing the
// process to match itself indefinitely). Returns the extracted pattern and true.
func isSelfMatchingMonitor(cmdline string) (string, bool) {
	pattern := extractPgrepPattern(cmdline)
	if pattern == "" {
		return "", false
	}
	// The pattern is extracted from cmdline, so strings.Contains is always true
	// for literal quoted patterns — that IS the self-matching condition: pgrep -f
	// <pattern> will find this shell because its own argv contains <pattern>.
	return pattern, strings.Contains(cmdline, pattern)
}

// extractPgrepPattern finds the first `pgrep -f <pattern>` invocation in
// cmdline and returns the literal pattern string. Returns "" when not found.
func extractPgrepPattern(cmdline string) string {
	idx := strings.Index(cmdline, "pgrep -f")
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(cmdline[idx+len("pgrep -f"):])
	if rest == "" {
		return ""
	}
	// Skip extra single-char flags before the pattern (e.g. -l -n)
	for len(rest) > 1 && rest[0] == '-' && rest[1] != ' ' {
		sp := strings.IndexByte(rest, ' ')
		if sp < 0 {
			return ""
		}
		rest = strings.TrimSpace(rest[sp+1:])
	}
	if rest == "" {
		return ""
	}
	// Quoted pattern
	if rest[0] == '"' || rest[0] == '\'' {
		quote := rest[0]
		end := strings.IndexByte(rest[1:], quote)
		if end >= 0 {
			return rest[1 : end+1]
		}
		return strings.TrimRight(rest[1:], ");|& \t\n")
	}
	// Unquoted: first token, strip shell punctuation including trailing quotes
	if sp := strings.IndexAny(rest, " \t\n);|&'\""); sp >= 0 {
		return rest[:sp]
	}
	return strings.TrimRight(rest, "'\")")
}

// monitorShellGuard is an attempt-scoped watchdog that scans descendant shell
// processes for self-matching pgrep-based monitors on a ticker. Detected shells
// are terminated and recorded in a monitor-shell-cleanup.json evidence artifact.
type monitorShellGuard struct {
	projectRoot string
	beadID      string
	attemptID   string
	rootPID     int
	interval    time.Duration

	mu      sync.Mutex
	cleaned []MonitorShellRecord
}

func newMonitorShellGuard(projectRoot, beadID, attemptID string, rootPID int) *monitorShellGuard {
	return &monitorShellGuard{
		projectRoot: projectRoot,
		beadID:      beadID,
		attemptID:   attemptID,
		rootPID:     rootPID,
		interval:    monitorShellScanInterval,
	}
}

// Start launches the watchdog goroutine and returns an idempotent stop function.
func (g *monitorShellGuard) Start(ctx context.Context) func() {
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

func (g *monitorShellGuard) tick(ctx context.Context, now time.Time) {
	procs, err := monitorShellScanner(ctx, g.rootPID, now)
	if err != nil || len(procs) == 0 {
		return
	}
	var newCleaned []MonitorShellRecord
	for _, proc := range procs {
		pattern, selfMatch := isSelfMatchingMonitor(proc.Command)
		if !selfMatch {
			continue
		}
		terminateMonitorShell(proc.PID)
		newCleaned = append(newCleaned, MonitorShellRecord{
			PID:        proc.PID,
			Command:    proc.Command,
			AgeSeconds: monitorShellAgeSeconds(proc, now),
			Action:     monitorShellActionTerminated,
			Reason:     reasonSelfMatchingMonitor,
			Pattern:    pattern,
		})
	}
	if len(newCleaned) == 0 {
		return
	}
	g.mu.Lock()
	g.cleaned = append(g.cleaned, newCleaned...)
	cumulative := append([]MonitorShellRecord(nil), g.cleaned...)
	g.mu.Unlock()
	writeMonitorShellCleanupArtifact(g.projectRoot, g.attemptID, &monitorShellCleanupReport{
		AttemptID: g.attemptID,
		BeadID:    g.beadID,
		ScannedAt: now,
		Cleaned:   cumulative,
	})
}

// CleanedCount returns the cumulative number of monitor shells terminated by
// this guard since it was started.
func (g *monitorShellGuard) CleanedCount() int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.cleaned)
}

// Cleaned returns a copy of the cleaned-shell evidence records accumulated
// since the guard was started.
func (g *monitorShellGuard) Cleaned() []MonitorShellRecord {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]MonitorShellRecord(nil), g.cleaned...)
}

func monitorShellAgeSeconds(proc monitorShellProcess, now time.Time) float64 {
	if proc.StartedAt.IsZero() {
		return 0
	}
	age := now.Sub(proc.StartedAt)
	if age < 0 {
		return 0
	}
	return age.Seconds()
}

func writeMonitorShellCleanupArtifact(projectRoot, attemptID string, report *monitorShellCleanupReport) {
	if report == nil || strings.TrimSpace(projectRoot) == "" || strings.TrimSpace(attemptID) == "" {
		return
	}
	path := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, monitorShellCleanupArtifact)
	_ = writeArtifactJSON(path, report)
}

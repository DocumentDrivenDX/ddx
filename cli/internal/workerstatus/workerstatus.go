// Package workerstatus reports on live "ddx work" / "ddx try" processes,
// scoped to a specific project root by default.
//
// Operators and agents asking "is the worker still working beads?" must be
// able to answer the question for one project at a time. A global process
// scan ("ps aux | grep ddx work") returns workers from every checkout on the
// machine and produces a misleading "yes, working" signal when the worker
// belongs to a different repository. This package filters live workers by
// the resolved project root so the answer always matches the question.
package workerstatus

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LiveWorker describes one running ddx worker process.
type LiveWorker struct {
	PID               int             `json:"pid"`
	Command           string          `json:"command"`
	StartedAt         time.Time       `json:"started_at"`
	AgeSeconds        float64         `json:"age_seconds"`
	Age               string          `json:"age"`
	ProjectRoot       string          `json:"project_root"`
	BeadID            string          `json:"bead_id,omitempty"`
	AttemptID         string          `json:"attempt_id,omitempty"`
	Phase             string          `json:"phase,omitempty"`
	Message           string          `json:"message,omitempty"`
	ChildPID          int             `json:"child_pid,omitempty"`
	LastActivityAt    time.Time       `json:"last_activity_at,omitempty"`
	ExecutionWorktree string          `json:"execution_worktree,omitempty"`
	Cwd               string          `json:"cwd,omitempty"`
	ProviderChildren  []ProviderChild `json:"provider_children,omitempty"`
}

// ProviderChild describes a provider CLI subprocess observed under a worker.
type ProviderChild struct {
	PID        int     `json:"pid"`
	Provider   string  `json:"provider"`
	Harness    string  `json:"harness,omitempty"`
	RouteOwner string  `json:"route_owner,omitempty"`
	Phase      string  `json:"phase,omitempty"`
	AgeSeconds float64 `json:"age_seconds"`
}

// Scanner discovers live ddx worker processes on the host.
type Scanner interface {
	Scan(ctx context.Context) ([]LiveWorker, error)
}

// New returns the platform-appropriate Scanner. On Linux this reads /proc.
// On other platforms it returns an empty list; the caller's tests and the
// --all-projects escape hatch remain meaningful regardless.
func New() Scanner {
	return newSystemScanner()
}

// beadIDPattern matches ddx-style bead identifiers (prefix + hex, 8+ chars
// total) embedded in argv strings or worktree paths.
var beadIDPattern = regexp.MustCompile(`\bddx-[a-f0-9]{8}\b`)

// executionWorktreePattern matches the conventional execute-bead worktree
// directory name (`.execute-bead-wt-<bead>-<timestamp>-<suffix>`) anywhere
// in a path. The directory name is enough to identify the worktree even when
// the surrounding path is unconventional.
var executionWorktreePattern = regexp.MustCompile(`(?:^|/)(\.execute-bead-wt-[^/]+)`)

// InferBead extracts a bead ID and execution worktree path from the worker's
// command line and current working directory. It returns the longest worktree
// path that contains a `.execute-bead-wt-*` segment (typically the cwd, when
// the inline worker has chdir'd into the isolated worktree), and the first
// bead ID matched in either the command line or the worktree path.
//
// Either return value may be empty when no signal is present, which is the
// expected case for a long-running `ddx work` drain that has not yet started
// a specific bead.
func InferBead(cmdline, cwd string) (beadID, worktree string) {
	worktree = extractWorktreePath(cwd)
	if worktree == "" {
		worktree = extractWorktreePath(cmdline)
	}

	if m := beadIDPattern.FindString(worktree); m != "" {
		beadID = m
	}
	if beadID == "" {
		if m := beadIDPattern.FindString(cmdline); m != "" {
			beadID = m
		}
	}
	if beadID == "" {
		if m := beadIDPattern.FindString(cwd); m != "" {
			beadID = m
		}
	}
	return beadID, worktree
}

func extractWorktreePath(s string) string {
	if s == "" {
		return ""
	}
	if !strings.Contains(s, ".execute-bead-wt-") {
		return ""
	}
	matches := executionWorktreePattern.FindStringIndex(s)
	if matches == nil {
		return ""
	}
	start := matches[0]
	end := matches[1]
	for start > 0 {
		r := s[start-1]
		if r == ' ' || r == '\x00' || r == '\t' {
			break
		}
		start--
	}
	for end < len(s) {
		r := s[end]
		if r == ' ' || r == '\x00' || r == '\t' {
			break
		}
		end++
	}
	return s[start:end]
}

// FilterByProject returns workers whose ProjectRoot resolves to the same path
// as projectRoot. Resolution canonicalises both sides via filepath.Abs and
// filepath.EvalSymlinks so a symlinked checkout matches its real path.
func FilterByProject(workers []LiveWorker, projectRoot string) []LiveWorker {
	target := canonicalPath(projectRoot)
	out := make([]LiveWorker, 0, len(workers))
	for _, w := range workers {
		if SamePath(w.ProjectRoot, target) {
			out = append(out, w)
		}
	}
	return out
}

// SamePath reports whether a and b refer to the same on-disk location after
// canonicalisation. It tolerates empty inputs (returning false) so callers
// can safely compare optional fields without prefiltering.
func SamePath(a, b string) bool {
	ca := canonicalPath(a)
	cb := canonicalPath(b)
	if ca == "" || cb == "" {
		return false
	}
	return ca == cb
}

func canonicalPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// FormatAge renders a duration as a short human-readable string mirroring
// the conventions used by the server-backed worker list (`5s`, `12m`,
// `1h30m`). It is intended for terminal output; JSON output uses
// AgeSeconds as the numeric source of truth.
func FormatAge(d time.Duration) string {
	if d < time.Minute {
		if d < 0 {
			d = 0
		}
		return roundedSeconds(d)
	}
	if d < time.Hour {
		return roundedMinutes(d)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return durationHours(hours)
	}
	return durationHoursMinutes(hours, mins)
}

func roundedSeconds(d time.Duration) string {
	secs := int(d.Seconds())
	return durationSeconds(secs)
}

func roundedMinutes(d time.Duration) string {
	mins := int(d.Minutes())
	return durationMinutes(mins)
}

func durationSeconds(n int) string { return formatUnit(n, "s") }
func durationMinutes(n int) string { return formatUnit(n, "m") }
func durationHours(n int) string   { return formatUnit(n, "h") }
func durationHoursMinutes(h, m int) string {
	return formatUnit(h, "h") + formatUnit(m, "m")
}

func formatUnit(n int, unit string) string {
	if n < 0 {
		n = 0
	}
	return itoa(n) + unit
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	out := string(digits[i:])
	if neg {
		return "-" + out
	}
	return out
}

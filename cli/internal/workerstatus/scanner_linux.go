//go:build linux

package workerstatus

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
)

// newSystemScanner returns a /proc-based scanner.
func newSystemScanner() Scanner {
	return &procScanner{now: time.Now}
}

// procScanner enumerates live `ddx work` and `ddx try` processes by reading
// /proc on Linux. Each candidate's command line, cwd, and start time are
// pulled directly from the kernel, so the scan does not depend on any DDx
// server state.
type procScanner struct {
	now func() time.Time
}

func (s *procScanner) Scan(ctx context.Context) ([]LiveWorker, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}

	workers := make([]LiveWorker, 0)
	bootTime := readBootTime("/proc/stat")
	for _, e := range entries {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		w, ok := s.inspect(pid, bootTime)
		if !ok {
			continue
		}
		workers = append(workers, w)
	}
	return workers, nil
}

func (s *procScanner) inspect(pid int, bootTime time.Time) (LiveWorker, bool) {
	base := filepath.Join("/proc", strconv.Itoa(pid))
	cmdlineRaw, err := os.ReadFile(filepath.Join(base, "cmdline"))
	if err != nil {
		return LiveWorker{}, false
	}
	cmdline := decodeCmdline(cmdlineRaw)
	if !looksLikeDDxWorker(cmdline) {
		return LiveWorker{}, false
	}

	cwd, _ := os.Readlink(filepath.Join(base, "cwd"))

	projectRoot := resolveWorkerProjectRoot(cmdline, cwd)
	beadID, worktree := InferBead(cmdline, cwd)

	started := time.Time{}
	if !bootTime.IsZero() {
		if startClk, ok := readStartTimeClockTicks(filepath.Join(base, "stat")); ok {
			ticks := clockTicksPerSecond()
			seconds := float64(startClk) / ticks
			started = bootTime.Add(time.Duration(seconds * float64(time.Second)))
		}
	}
	now := s.now().UTC()
	if started.IsZero() {
		started = now
	}
	age := now.Sub(started)
	if age < 0 {
		age = 0
	}

	return LiveWorker{
		PID:               pid,
		Command:           cmdline,
		StartedAt:         started.UTC(),
		AgeSeconds:        age.Seconds(),
		Age:               FormatAge(age),
		ProjectRoot:       projectRoot,
		BeadID:            beadID,
		ExecutionWorktree: worktree,
		Cwd:               cwd,
	}, true
}

// decodeCmdline converts /proc/<pid>/cmdline's NUL-separated args into a
// space-separated string suitable for display and substring matching.
func decodeCmdline(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[len(raw)-1] == 0 {
		raw = raw[:len(raw)-1]
	}
	parts := bytes.Split(raw, []byte{0})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, string(p))
	}
	return strings.Join(out, " ")
}

// looksLikeDDxWorker reports whether the argv resembles a `ddx work` or
// `ddx try` invocation. We match against argv[0]'s basename so a binary
// installed at any path (or via `go run ./cli ...`) is still detected.
func looksLikeDDxWorker(cmdline string) bool {
	if cmdline == "" {
		return false
	}
	parts := strings.Fields(cmdline)
	if len(parts) < 2 {
		return false
	}
	bin := filepath.Base(parts[0])
	if bin != "ddx" {
		// Allow `go run ... ddx work` style invocations where the binary
		// path ends in `cmd` or similar but the args still start with the
		// ddx subcommand. We only consider real ddx binaries here.
		return false
	}
	sub := parts[1]
	if sub == "try" {
		return true
	}
	if sub != "work" {
		return false
	}
	if len(parts) >= 3 && isWorkHelperSubcommand(parts[2]) {
		return false
	}
	return true
}

func isWorkHelperSubcommand(arg string) bool {
	switch arg {
	case "analyze", "clear-cooldowns", "focus", "metrics", "plan", "status":
		return true
	default:
		return false
	}
}

// resolveWorkerProjectRoot derives the project root the worker is operating
// on. The order of preference matches how `ddx work` itself resolves --project
// (CLI flag → DDX_PROJECT_ROOT env → cwd's git root). When the cwd points at
// an isolated execute-bead worktree (a sibling of the real checkout), we
// fall back to the surrounding git root rather than the worktree path.
func resolveWorkerProjectRoot(cmdline, cwd string) string {
	if flagVal := parseFlagValue(cmdline, "--project"); flagVal != "" {
		return canonicalPath(flagVal)
	}
	if cwd == "" {
		return ""
	}
	// If cwd is inside an isolated execute-bead worktree the git root will
	// be the worktree itself; that is a per-attempt artefact, not the
	// originating project. Worktree paths conventionally live under either
	// `<project>/.ddx/.execute-bead-wt-*` or under DDX_EXEC_WT_DIR. Caller
	// resolution here only needs to keep workers consistent: we always
	// derive ProjectRoot from cwd's git root via the same helper the worker
	// itself would use.
	root := gitpkg.FindProjectRoot(cwd)
	return canonicalPath(root)
}

// parseFlagValue returns the argument value for a `--flag <value>` or
// `--flag=<value>` form embedded in a space-separated command line. The
// match is anchored at word boundaries so `--projectile` does not satisfy
// `--project`.
func parseFlagValue(cmdline, flag string) string {
	parts := strings.Fields(cmdline)
	for i, p := range parts {
		if p == flag && i+1 < len(parts) {
			return parts[i+1]
		}
		if strings.HasPrefix(p, flag+"=") {
			return strings.TrimPrefix(p, flag+"=")
		}
	}
	return ""
}

func readBootTime(statPath string) time.Time {
	data, err := os.ReadFile(statPath)
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "btime ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sec, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return time.Time{}
		}
		return time.Unix(sec, 0)
	}
	return time.Time{}
}

func readStartTimeClockTicks(statPath string) (int64, bool) {
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, false
	}
	// The 22nd field of /proc/<pid>/stat is starttime (clock ticks since
	// boot). The 2nd field (comm) is wrapped in parentheses and may itself
	// contain spaces, so we scan from the last ')' to skip it cleanly.
	close := strings.LastIndex(string(data), ")")
	if close < 0 {
		return 0, false
	}
	tail := strings.Fields(string(data)[close+1:])
	// After the comm field, the index of starttime is 22 - 2 = 20 (zero-
	// based) in the tail slice (because state is index 0 in the tail).
	const starttimeIndex = 22 - 3
	if starttimeIndex >= len(tail) {
		return 0, false
	}
	v, err := strconv.ParseInt(tail[starttimeIndex], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func clockTicksPerSecond() float64 {
	// Linux exposes CLK_TCK as a sysconf value; the kernel default on
	// every platform DDx supports is 100. Using the constant keeps us
	// free of cgo without changing the calculated age in practice.
	return 100.0
}

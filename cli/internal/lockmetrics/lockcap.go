package lockmetrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
)

// Default hold-time caps. A worker that holds one of the named locks past its
// cap is treated as hung: the lock is force-released and the violation is
// surfaced, rather than letting the indefinite hold cascade into tracker-lock
// timeouts (the failure mode observed 2026-05-17).
const (
	DefaultIndexLockCap   = 10 * time.Second
	DefaultTrackerLockCap = 30 * time.Second
)

// CapFor returns the configured hold-time cap for lockName. The defaults are
// DefaultIndexLockCap for "index.lock" and DefaultTrackerLockCap for
// "tracker.lock", each overridable via the DDX_LOCK_CAP_INDEX_MS /
// DDX_LOCK_CAP_TRACKER_MS environment variables (milliseconds). Any other lock
// name has no cap and returns 0.
func CapFor(lockName string) time.Duration {
	switch lockName {
	case "index.lock":
		return capFromEnv("DDX_LOCK_CAP_INDEX_MS", DefaultIndexLockCap)
	case "tracker.lock":
		return capFromEnv("DDX_LOCK_CAP_TRACKER_MS", DefaultTrackerLockCap)
	default:
		return 0
	}
}

// capFromEnv reads a millisecond cap from the named environment variable,
// falling back to def when the variable is unset, empty, or not a positive
// integer.
func capFromEnv(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms <= 0 {
		return def
	}
	return time.Duration(ms) * time.Millisecond
}

// CapConfig parameterises hold-time cap enforcement for a single instrumented
// lock window. The zero value disables enforcement, so InstrumentCapped then
// behaves exactly like a pure metric wrapper.
type CapConfig struct {
	// Cap is the maximum hold time. Zero or negative disables enforcement.
	Cap time.Duration
	// LockPath is the filesystem path force-released (os.RemoveAll) when the
	// cap is exceeded. Empty skips force-release.
	LockPath string
	// EvidenceDir receives lock-violation.json when the cap is exceeded.
	// Empty skips the evidence record.
	EvidenceDir string
}

// Violation records a lock held past its configured cap. It is written to
// <EvidenceDir>/lock-violation.json so the post-execution reviewer sees the
// over-long hold even though the worker continues.
type Violation struct {
	LockName     string `json:"lock_name"`
	CapMS        int64  `json:"cap_ms"`
	ActualHoldMS int64  `json:"actual_hold_ms"`
	HolderPID    int    `json:"holder_pid"`
	Stack        string `json:"stack"`
}

// InstrumentCapped behaves like Instrument but additionally enforces a
// hold-time cap. It emits an "acquire" event, runs critical, and emits a
// matching "release" event. If cfg.Cap > 0 and the hold exceeds the cap, a
// watchdog fires once: it captures a stack trace, force-releases the lock by
// removing cfg.LockPath, writes <cfg.EvidenceDir>/lock-violation.json, and
// emits an error-severity "violation" event via the metric helper.
//
// The critical section is NOT interrupted — it runs to completion
// (release-only behaviour); the forced release and violation record are the
// loud signal that a hold ran too long. critical's error is returned
// unchanged.
func InstrumentCapped(lockName, operation string, cfg CapConfig, critical func() error) (err error) {
	pid := os.Getpid()
	acquiredAt := time.Now()
	Emit(Event{
		Event:      "acquire",
		LockName:   lockName,
		Operation:  operation,
		HolderPID:  pid,
		AcquiredAt: acquiredAt.UTC().Format(time.RFC3339Nano),
	})

	var timer *time.Timer
	if cfg.Cap > 0 {
		timer = time.AfterFunc(cfg.Cap, func() {
			enforceCapViolation(lockName, operation, pid, acquiredAt, cfg)
		})
	}

	defer func() {
		if timer != nil {
			timer.Stop()
		}
		releasedAt := time.Now()
		Emit(Event{
			Event:      "release",
			LockName:   lockName,
			Operation:  operation,
			HolderPID:  pid,
			AcquiredAt: acquiredAt.UTC().Format(time.RFC3339Nano),
			ReleasedAt: releasedAt.UTC().Format(time.RFC3339Nano),
			DurationMS: releasedAt.Sub(acquiredAt).Milliseconds(),
		})
	}()
	return critical()
}

// enforceCapViolation is the watchdog action run when a hold exceeds its cap:
// force-release the lock, write the evidence record, and emit the error event.
func enforceCapViolation(lockName, operation string, pid int, acquiredAt time.Time, cfg CapConfig) {
	held := time.Since(acquiredAt)
	stack := string(debug.Stack())

	if cfg.LockPath != "" {
		_ = os.RemoveAll(cfg.LockPath)
	}

	if cfg.EvidenceDir != "" {
		_ = writeViolation(cfg.EvidenceDir, Violation{
			LockName:     lockName,
			CapMS:        cfg.Cap.Milliseconds(),
			ActualHoldMS: held.Milliseconds(),
			HolderPID:    pid,
			Stack:        stack,
		})
	}

	Emit(Event{
		Event:      "violation",
		LockName:   lockName,
		Operation:  operation,
		HolderPID:  pid,
		AcquiredAt: acquiredAt.UTC().Format(time.RFC3339Nano),
		Severity:   "error",
		CapMS:      cfg.Cap.Milliseconds(),
		DurationMS: held.Milliseconds(),
	})
}

// writeViolation writes v as lock-violation.json under evidenceDir.
func writeViolation(evidenceDir string, v Violation) error {
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(evidenceDir, "lock-violation.json"), data, 0o644)
}

var (
	capEnfMu       sync.RWMutex
	capEnabled     bool
	capProjectRoot string
	capEvidenceDir string
)

// SetCapEnforcement enables hold-time cap enforcement process-wide for the two
// named locks. projectRoot locates the lock files to force-release;
// evidenceDir (may be "") receives lock-violation.json on a violation. Passing
// an empty projectRoot disables enforcement (the default), leaving Instrument
// a pure metric wrapper. Safe for concurrent use.
func SetCapEnforcement(projectRoot, evidenceDir string) {
	capEnfMu.Lock()
	defer capEnfMu.Unlock()
	capProjectRoot = projectRoot
	capEvidenceDir = evidenceDir
	capEnabled = projectRoot != ""
}

// resolveCapConfig builds the CapConfig that Instrument applies to lockName
// from the process-wide enforcement state. It returns the zero CapConfig
// (no enforcement) when enforcement is disabled or lockName has no cap.
func resolveCapConfig(lockName string) CapConfig {
	capEnfMu.RLock()
	enabled := capEnabled
	root := capProjectRoot
	evidence := capEvidenceDir
	capEnfMu.RUnlock()

	if !enabled {
		return CapConfig{}
	}
	capDur := CapFor(lockName)
	if capDur <= 0 {
		return CapConfig{}
	}
	return CapConfig{
		Cap:         capDur,
		LockPath:    lockPathFor(root, lockName),
		EvidenceDir: evidence,
	}
}

// SharedMainGitLockRoot resolves the DDx state root that should own the
// process-shared main-git lock. Linked worktrees converge on the primary
// workspace when one is available; otherwise the caller falls back to the
// standard project-scoped DDx path.
func SharedMainGitLockRoot(projectRoot string) string {
	if workspace := gitpkg.FindNearestDDxWorkspace(projectRoot); workspace != "" {
		if info, err := os.Stat(filepath.Join(workspace, ddxroot.DirName)); err == nil && info.IsDir() {
			return workspace
		}
	}
	return ddxroot.Path(context.Background(), projectRoot)
}

// SharedTrackerLockPath resolves the process-shared tracker lock used by
// withMainGitLock and cap enforcement.
func SharedTrackerLockPath(projectRoot string) string {
	return filepath.Join(SharedMainGitLockRoot(projectRoot), ddxroot.DirName, ".git-tracker.lock")
}

// lockPathFor resolves the on-disk path of a named lock under projectRoot.
func lockPathFor(projectRoot, lockName string) string {
	switch lockName {
	case "index.lock":
		return filepath.Join(projectRoot, ".git", "index.lock")
	case "tracker.lock":
		return SharedTrackerLockPath(projectRoot)
	default:
		return ""
	}
}

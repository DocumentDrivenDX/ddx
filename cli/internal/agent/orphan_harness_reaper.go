package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

type orphanHarnessProcess struct {
	PID     int
	PPID    int
	Command string
	Cwd     string
}

type orphanHarnessProcessScanner interface {
	Scan(context.Context) ([]orphanHarnessProcess, error)
}

type orphanHarnessLeaseReader interface {
	ClaimLease(id string) (bead.ClaimLeaseRecord, bool, error)
}

type orphanHarnessLeaseReleaser interface {
	Release(id, assignee, targetStatus string) error
}

type orphanHarnessEventAppender interface {
	AppendEvent(id string, event bead.BeadEvent) error
}

// reapOrphanedHarnessChildren kills orphaned harness subprocesses that still
// sit inside this project's execution worktree root and clears the stale lease
// they were holding.
func reapOrphanedHarnessChildren(
	ctx context.Context,
	projectRoot string,
	scanner orphanHarnessProcessScanner,
	leaseReader orphanHarnessLeaseReader,
	releaser orphanHarnessLeaseReleaser,
	appender orphanHarnessEventAppender,
	assignee string,
	log io.Writer,
	emit func(string, map[string]any),
	killGroup func(int) error,
) (int, error) {
	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if scanner == nil || projectRoot == "" {
		return 0, nil
	}
	if killGroup == nil {
		killGroup = func(int) error { return nil }
	}

	processes, err := scanner.Scan(ctx)
	if err != nil {
		return 0, err
	}

	execRoot := canonicalExecRoot(config.ExecutionTempRoot(projectRoot))
	if execRoot == "" {
		return 0, nil
	}

	reaped := 0
	for _, proc := range processes {
		if ctx != nil && ctx.Err() != nil {
			return reaped, ctx.Err()
		}
		if proc.PPID != 1 || proc.PID <= 0 {
			continue
		}
		if !looksLikeHarnessProcess(proc.Command) {
			continue
		}
		beadID, worktree := workerstatus.InferBead(proc.Command, proc.Cwd)
		if beadID == "" || worktree == "" {
			continue
		}
		if !isWithinProjectExecutionRoot(execRoot, worktree) {
			continue
		}
		if leaseReader == nil {
			continue
		}
		lease, found, leaseErr := leaseReader.ClaimLease(beadID)
		if leaseErr != nil || !found || lease.PID <= 0 || processAlive(lease.PID) {
			continue
		}

		diagnosis := fmt.Sprintf(
			"orphaned harness child %d (parent pid 1) in %s; claim owner pid %d is gone",
			proc.PID, worktree, lease.PID,
		)

		if killErr := killGroup(proc.PID); killErr != nil && log != nil {
			_, _ = fmt.Fprintf(log, "startup orphan reaper: failed to kill process group %d for %s: %v\n", proc.PID, beadID, killErr)
		}
		if releaser != nil {
			_ = releaser.Release(beadID, assignee, "")
		}
		if appender != nil {
			body, _ := json.Marshal(map[string]any{
				"reason":             "orphaned_harness_child",
				"bead_id":            beadID,
				"process_pid":        proc.PID,
				"process_parent_pid": proc.PPID,
				"claim_owner_pid":    lease.PID,
				"worktree":           worktree,
				"project_root":       projectRoot,
				"diagnosis":          diagnosis,
			})
			_ = appender.AppendEvent(beadID, bead.BeadEvent{
				Kind:      "operator_attention",
				Summary:   "orphaned_harness_child",
				Body:      string(body),
				Actor:     assignee,
				Source:    "ddx work",
				CreatedAt: time.Now().UTC(),
			})
		}
		if emit != nil {
			emit("loop.operator_attention", map[string]any{
				"reason":             "orphaned_harness_child",
				"bead_id":            beadID,
				"process_pid":        proc.PID,
				"process_parent_pid": proc.PPID,
				"claim_owner_pid":    lease.PID,
				"worktree":           worktree,
				"project_root":       projectRoot,
				"diagnosis":          diagnosis,
			})
		}
		if log != nil {
			_, _ = fmt.Fprintf(log, "startup orphan reaper: reaped orphaned harness child %d for %s\n", proc.PID, beadID)
		}
		reaped++
	}

	return reaped, nil
}

func looksLikeHarnessProcess(cmdline string) bool {
	if strings.TrimSpace(cmdline) == "" {
		return false
	}
	parts := strings.Fields(cmdline)
	if len(parts) == 0 {
		return false
	}
	switch filepath.Base(parts[0]) {
	case "claude", "codex", "gemini", "opencode", "pi":
		return true
	default:
		return false
	}
}

func canonicalExecRoot(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return filepath.Clean(abs)
}

func isWithinProjectExecutionRoot(execRoot, worktree string) bool {
	if execRoot == "" || worktree == "" {
		return false
	}
	abs := canonicalExecRoot(worktree)
	if abs == execRoot {
		return true
	}
	prefix := execRoot + string(filepath.Separator)
	return strings.HasPrefix(abs, prefix)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

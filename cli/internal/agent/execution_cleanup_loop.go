package agent

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

const defaultExecutionCleanupInterval = 10 * time.Minute
const executionCleanupJitterFraction = 0.20

const executionCleanupLockDirName = ".cleanup.lock"

func executionCleanupLockPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".ddx", executionCleanupLockDirName)
}

// runExecutionCleanupPass conservatively runs the shared cleanup manager under
// a project-level lock. When another process already owns the lock, the pass
// is skipped and observed rather than blocking concurrent deletion.
func runExecutionCleanupPass(ctx context.Context, projectRoot string, runner executionCleanupRunner, log io.Writer, emit func(string, map[string]any), reason string) (ExecutionCleanupSummary, bool, error) {
	if runner == nil {
		return ExecutionCleanupSummary{}, false, nil
	}
	if projectRoot == "" {
		return ExecutionCleanupSummary{}, false, fmt.Errorf("execution cleanup: project root is empty")
	}

	release, acquired, err := tryAcquireExecutionCleanupLock(projectRoot)
	if err != nil {
		return ExecutionCleanupSummary{}, false, err
	}
	if !acquired {
		if emit != nil {
			emit("cleanup.skipped", map[string]any{
				"project_root": projectRoot,
				"reason":       reason,
				"lock_path":    executionCleanupLockPath(projectRoot),
			})
		}
		return ExecutionCleanupSummary{}, true, nil
	}
	defer release()

	summary, runErr := runner.Cleanup(ctx)
	meaningful := executionCleanupSummaryMeaningful(summary)

	if emit != nil && meaningful {
		emit("cleanup.pass", map[string]any{
			"project_root":                summary.ProjectRoot,
			"temp_root":                   summary.TempRoot,
			"reason":                      reason,
			"scanned_temp_dirs":           summary.ScannedTempDirs,
			"scanned_evidence_dirs":       summary.ScannedEvidenceDirs,
			"complete_evidence_dirs":      summary.CompleteEvidenceDirs,
			"scanned_scratch_dirs":        summary.ScannedScratchDirs,
			"removed_unregistered":        summary.RemovedUnregisteredTempDirs,
			"removed_registered_worktree": summary.RemovedRegisteredWorktrees,
			"removed_run_state":           summary.RemovedRunStateFiles,
			"removed_scratch_dirs":        summary.RemovedScratchDirs,
			"removed_evidence_dirs":       summary.RemovedEvidenceDirs,
			"removed_agent_logs":          summary.RemovedAgentLogs,
			"removed_worker_dirs":         summary.RemovedWorkerDirs,
			"preserved_scratch_dirs":      summary.PreservedActiveScratchDirs,
			"bytes_reclaimed":             summary.BytesReclaimed,
			"inodes_reclaimed":            summary.InodesReclaimed,
			"scratch_bytes_reclaimed":     summary.ScratchBytesReclaimed,
			"scratch_inodes_reclaimed":    summary.ScratchInodesReclaimed,
			"warnings":                    len(summary.Warnings),
			"issues":                      len(summary.Issues),
		})
	}
	if meaningful && log != nil {
		fmt.Fprintf(log, "cleanup: %s %d temp dir(s), %d worktree(s), %d run-state file(s), %d scratch dir(s), %d evidence dir(s), %d agent log(s), %d worker dir(s), %d byte(s), %d inode(s)\n",
			reason,
			summary.RemovedUnregisteredTempDirs,
			summary.RemovedRegisteredWorktrees,
			summary.RemovedRunStateFiles,
			summary.RemovedScratchDirs,
			summary.RemovedEvidenceDirs,
			summary.RemovedAgentLogs,
			summary.RemovedWorkerDirs,
			summary.BytesReclaimed+summary.ScratchBytesReclaimed,
			summary.InodesReclaimed+summary.ScratchInodesReclaimed,
		)
	}
	return summary, false, runErr
}

func executionCleanupSummaryMeaningful(summary ExecutionCleanupSummary) bool {
	return len(summary.Issues) > 0 ||
		summary.BytesReclaimed > 0 ||
		summary.InodesReclaimed > 0 ||
		summary.ScratchBytesReclaimed > 0 ||
		summary.ScratchInodesReclaimed > 0 ||
		summary.RemovedUnregisteredTempDirs > 0 ||
		summary.RemovedRegisteredWorktrees > 0 ||
		summary.RemovedRunStateFiles > 0 ||
		summary.RemovedScratchDirs > 0 ||
		summary.RemovedEvidenceDirs > 0 ||
		summary.RemovedAgentLogs > 0 ||
		summary.RemovedWorkerDirs > 0 ||
		summary.PreservedActiveScratchDirs > 0
}

func tryAcquireExecutionCleanupLock(projectRoot string) (func(), bool, error) {
	lockDir := executionCleanupLockPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(lockDir), 0o755); err != nil {
		return nil, false, fmt.Errorf("execution cleanup: lock dir: %w", err)
	}
	if err := os.Mkdir(lockDir, 0o755); err != nil {
		if breakStaleExecutionCleanupLock(lockDir) {
			if err := os.Mkdir(lockDir, 0o755); err != nil {
				return nil, false, nil
			}
		} else {
			return nil, false, nil
		}
	}
	_ = os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
	_ = os.WriteFile(filepath.Join(lockDir, "acquired_at"), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
	return func() { _ = os.RemoveAll(lockDir) }, true, nil
}

func breakStaleExecutionCleanupLock(lockDir string) bool {
	pidData, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	if err == nil {
		pid := 0
		if _, scanErr := fmt.Sscanf(string(pidData), "%d", &pid); scanErr == nil && pid > 0 && pid != os.Getpid() {
			if !trackerProcessAlive(pid) {
				_ = os.RemoveAll(lockDir)
				return true
			}
		}
	}

	acquiredData, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	if err == nil {
		acquired, parseErr := time.Parse(time.RFC3339, string(acquiredData))
		if parseErr == nil && time.Since(acquired) > trackerLockStaleAge {
			_ = os.RemoveAll(lockDir)
			return true
		}
	}

	return false
}

func jitteredCleanupDelay(base time.Duration, rng *rand.Rand) time.Duration {
	if base <= 0 {
		return 0
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if executionCleanupJitterFraction <= 0 {
		return base
	}
	jitter := time.Duration(float64(base) * executionCleanupJitterFraction)
	if jitter <= 0 {
		return base
	}
	span := int64(jitter) * 2
	if span <= 0 {
		return base
	}
	offset := time.Duration(rng.Int63n(span+1)) - jitter
	delay := base + offset
	if delay < time.Second {
		delay = time.Second
	}
	return delay
}

func startExecutionCleanupWorker(ctx context.Context, projectRoot string, runner executionCleanupRunner, interval time.Duration, tickCh <-chan time.Time, log io.Writer, emit func(string, map[string]any)) (stop func(runShutdownPass bool)) {
	if runner == nil {
		return func(bool) {}
	}
	if interval <= 0 {
		interval = defaultExecutionCleanupInterval
	}

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var rng *rand.Rand
		if tickCh == nil {
			rng = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		for {
			if tickCh != nil {
				select {
				case <-workerCtx.Done():
					return
				case <-tickCh:
					_, _, _ = runExecutionCleanupPass(workerCtx, projectRoot, runner, log, emit, "periodic")
				}
				continue
			}

			timer := time.NewTimer(jitteredCleanupDelay(interval, rng))
			select {
			case <-workerCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
				_, _, _ = runExecutionCleanupPass(workerCtx, projectRoot, runner, log, emit, "periodic")
			}
		}
	}()

	return func(runShutdownPass bool) {
		cancel()
		<-done
		if runShutdownPass {
			_, _, _ = runExecutionCleanupPass(context.Background(), projectRoot, runner, log, emit, "shutdown")
		}
	}
}

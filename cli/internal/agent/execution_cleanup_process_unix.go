//go:build !windows

package agent

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"
)

func defaultExecutionCleanupProcessScanner() ExecutionCleanupProcessScanner {
	return executionCleanupProcessScannerFunc(func(ctx context.Context) ([]ExecutionCleanupProcessInfo, error) {
		processes, err := scanAttemptProcesses(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]ExecutionCleanupProcessInfo, 0, len(processes))
		for _, proc := range processes {
			out = append(out, ExecutionCleanupProcessInfo(proc))
		}
		return out, nil
	})
}

func defaultExecutionCleanupProcessKiller() ExecutionCleanupProcessKiller {
	return executionCleanupProcessKillerFunc(killExecutionProcessGroup)
}

func killExecutionProcessGroup(ctx context.Context, groupID int) error {
	if groupID <= 0 {
		return nil
	}
	if ownPGID, err := syscall.Getpgid(os.Getpid()); err == nil && ownPGID == groupID {
		return fmt.Errorf("refusing to kill own process group %d", groupID)
	}
	target := -groupID
	if err := syscall.Kill(target, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if syscall.Kill(target, 0) == syscall.ESRCH {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := syscall.Kill(target, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

//go:build windows

package server

import (
	"os"
	"time"
)

// isPIDAlive reports whether a process with the given PID is alive.
// On Windows, os.FindProcess always succeeds, so we conservatively return
// true for any positive PID. Prune falls back to maxAge-based pruning on
// Windows rather than PID liveness.
func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.FindProcess(pid)
	return err == nil
}

// terminateProcessGroup on Windows has no process-group primitive. We call
// os.Process.Kill (maps to TerminateProcess) directly; the grace is observed
// so the API stays uniform across platforms.
func cleanupManagedWorkerProcessTree(pid int, registeredPGIDs []int, grace time.Duration) managedProcessCleanupReport {
	report := managedProcessCleanupReport{
		RootPID:         pid,
		RegisteredPGIDs: uniqueSortedInts(registeredPGIDs),
	}
	targets := make([]int, 0, 1+len(report.RegisteredPGIDs))
	if pid > 0 {
		targets = append(targets, pid)
	}
	targets = append(targets, report.RegisteredPGIDs...)
	targets = uniqueSortedInts(targets)
	if len(targets) == 0 {
		return report
	}
	report.TargetPGIDs = append([]int(nil), targets...)
	for _, target := range targets {
		if signalWindowsProcessOnce(target, os.Interrupt) {
			report.TerminatedPGIDs = append(report.TerminatedPGIDs, target)
		}
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		alive := false
		for _, target := range targets {
			if windowsProcessAlive(target) {
				alive = true
				break
			}
		}
		if !alive {
			return report
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, target := range targets {
		if windowsProcessAlive(target) && killWindowsProcessOnce(target) {
			report.KilledPGIDs = append(report.KilledPGIDs, target)
		}
	}
	return report
}

func terminateProcessGroup(pid int, grace time.Duration) {
	_ = cleanupManagedWorkerProcessTree(pid, nil, grace)
}

func windowsProcessAlive(pid int) bool {
	return isPIDAlive(pid)
}

func signalWindowsProcessOnce(pid int, sig os.Signal) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Signal(sig)
	return true
}

func killWindowsProcessOnce(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Kill()
	return true
}

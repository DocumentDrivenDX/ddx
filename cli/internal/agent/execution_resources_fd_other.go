//go:build !linux

package agent

import "golang.org/x/sys/unix"

// fdDiagnostics captures a snapshot of process fd usage.
type fdDiagnostics struct {
	Count     int
	SoftLimit uint64
	HardLimit uint64
	Sample    []string
}

// collectFDDiagnostics reports RLIMIT_NOFILE values on platforms without a
// /proc/self/fd listing. Count and Sample are left unpopulated.
func collectFDDiagnostics() fdDiagnostics {
	var diag fdDiagnostics
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err == nil {
		diag.SoftLimit = uint64(limit.Cur)
		diag.HardLimit = uint64(limit.Max)
	}
	return diag
}

//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/sys/unix"
)

// fdDiagnosticsSampleSize caps how many open fd targets are attached to a
// preflight failure so the payload stays compact.
const fdDiagnosticsSampleSize = 8

// fdDiagnostics captures a snapshot of process fd usage.
type fdDiagnostics struct {
	Count     int
	SoftLimit uint64
	HardLimit uint64
	Sample    []string
}

func collectFDDiagnostics() fdDiagnostics {
	var diag fdDiagnostics

	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err == nil {
		diag.SoftLimit = uint64(limit.Cur)
		diag.HardLimit = uint64(limit.Max)
	}

	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return diag
	}
	diag.Count = len(entries)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		if len(diag.Sample) >= fdDiagnosticsSampleSize {
			break
		}
		target, readErr := os.Readlink(filepath.Join("/proc/self/fd", name))
		if readErr != nil {
			continue
		}
		diag.Sample = append(diag.Sample, fmt.Sprintf("%s -> %s", name, target))
	}

	return diag
}

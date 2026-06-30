package server

import (
	"fmt"
	"sort"
	"strings"
)

// managedProcessCleanupReport captures the process-group cleanup work performed
// by Stop/reap. It is intentionally compact: callers only need a stable summary
// for lifecycle detail and debug logging.
type managedProcessCleanupReport struct {
	RootPID         int
	RegisteredPGIDs []int
	TargetPGIDs     []int
	TerminatedPGIDs []int
	KilledPGIDs     []int
}

func (r managedProcessCleanupReport) String() string {
	if r.RootPID <= 0 && len(r.TargetPGIDs) == 0 && len(r.TerminatedPGIDs) == 0 && len(r.KilledPGIDs) == 0 {
		return ""
	}
	parts := make([]string, 0, 4)
	if r.RootPID > 0 {
		parts = append(parts, fmt.Sprintf("root=%d", r.RootPID))
	}
	if len(r.RegisteredPGIDs) > 0 {
		parts = append(parts, fmt.Sprintf("registered=%v", r.RegisteredPGIDs))
	}
	if len(r.TargetPGIDs) > 0 {
		parts = append(parts, fmt.Sprintf("targets=%v", r.TargetPGIDs))
	}
	if len(r.TerminatedPGIDs) > 0 {
		parts = append(parts, fmt.Sprintf("sigterm=%v", r.TerminatedPGIDs))
	}
	if len(r.KilledPGIDs) > 0 {
		parts = append(parts, fmt.Sprintf("sigkill=%v", r.KilledPGIDs))
	}
	return strings.Join(parts, " ")
}

func uniqueSortedInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, v := range values {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

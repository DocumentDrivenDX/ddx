package workerstatus

import (
	"fmt"
	"math"
	"runtime"
)

// DefaultLoadPressureThreshold is the maximum accepted five-minute load
// average per logical CPU before a worker should pace new claims.
const DefaultLoadPressureThreshold = 2.5

// LoadPressureSnapshot is a normalized, point-in-time view of host load.
// Unsupported and unavailable snapshots are diagnostic only and must fail
// open at call sites.
type LoadPressureSnapshot struct {
	Load5           float64 `json:"load5"`
	CPUCount        int     `json:"cpu_count"`
	NormalizedRatio float64 `json:"normalized_ratio"`
	Threshold       float64 `json:"threshold"`
	Supported       bool    `json:"supported"`
	Available       bool    `json:"available"`
	Overloaded      bool    `json:"overloaded"`
	Diagnostic      string  `json:"diagnostic,omitempty"`
}

// LoadPressureProbe carries injectable load and CPU sources. Load5 returns
// supported=false on platforms without a host load-average source.
type LoadPressureProbe struct {
	Load5    func() (load5 float64, supported bool, err error)
	CPUCount func() int
}

// Snapshot samples and normalizes five-minute load by logical CPU count.
// A non-positive threshold selects DefaultLoadPressureThreshold.
func (p LoadPressureProbe) Snapshot(threshold float64) LoadPressureSnapshot {
	if threshold <= 0 || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		threshold = DefaultLoadPressureThreshold
	}
	snapshot := LoadPressureSnapshot{Threshold: threshold}
	if p.Load5 == nil {
		snapshot.Diagnostic = "load pressure unavailable: load source is not configured"
		return snapshot
	}

	load5, supported, err := p.Load5()
	snapshot.Supported = supported
	if !supported {
		snapshot.Diagnostic = fmt.Sprintf("load pressure unsupported on %s", runtime.GOOS)
		return snapshot
	}
	if err != nil {
		snapshot.Diagnostic = fmt.Sprintf("load pressure unavailable: %v", err)
		return snapshot
	}
	if load5 < 0 || math.IsNaN(load5) || math.IsInf(load5, 0) {
		snapshot.Diagnostic = fmt.Sprintf("load pressure unavailable: invalid five-minute load %v", load5)
		return snapshot
	}
	if p.CPUCount == nil {
		snapshot.Diagnostic = "load pressure unavailable: CPU source is not configured"
		return snapshot
	}
	cpuCount := p.CPUCount()
	if cpuCount <= 0 {
		snapshot.Diagnostic = fmt.Sprintf("load pressure unavailable: invalid CPU count %d", cpuCount)
		return snapshot
	}

	snapshot.Load5 = load5
	snapshot.CPUCount = cpuCount
	snapshot.NormalizedRatio = load5 / float64(cpuCount)
	snapshot.Available = true
	snapshot.Overloaded = snapshot.NormalizedRatio > threshold
	return snapshot
}

// HostLoadPressureSnapshot samples the current host with platform-specific
// load support and runtime.NumCPU.
func HostLoadPressureSnapshot(threshold float64) LoadPressureSnapshot {
	return (LoadPressureProbe{
		Load5:    systemLoad5,
		CPUCount: runtime.NumCPU,
	}).Snapshot(threshold)
}

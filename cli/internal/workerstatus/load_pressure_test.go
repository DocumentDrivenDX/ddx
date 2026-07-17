package workerstatus

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPressureSnapshot_NormalizesByCPUCount(t *testing.T) {
	tests := []struct {
		name       string
		load5      float64
		supported  bool
		loadErr    error
		cpuCount   int
		available  bool
		overload   bool
		ratio      float64
		diagnostic string
	}{
		{name: "below threshold", load5: 8, supported: true, cpuCount: 4, available: true, ratio: 2},
		{name: "above threshold", load5: 12, supported: true, cpuCount: 4, available: true, overload: true, ratio: 3},
		{name: "unavailable source", supported: true, loadErr: errors.New("loadavg missing"), cpuCount: 4, diagnostic: "load pressure unavailable"},
		{name: "unsupported platform", supported: false, cpuCount: 4, diagnostic: "load pressure unsupported"},
		{name: "invalid CPU count", load5: 8, supported: true, cpuCount: 0, diagnostic: "invalid CPU count"},
		{name: "negative load", load5: -1, supported: true, cpuCount: 4, diagnostic: "invalid five-minute load"},
		{name: "NaN load", load5: math.NaN(), supported: true, cpuCount: 4, diagnostic: "invalid five-minute load"},
		{name: "infinite load", load5: math.Inf(1), supported: true, cpuCount: 4, diagnostic: "invalid five-minute load"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := (LoadPressureProbe{
				Load5: func() (float64, bool, error) {
					return tc.load5, tc.supported, tc.loadErr
				},
				CPUCount: func() int { return tc.cpuCount },
			}).Snapshot(0)

			assert.Equal(t, DefaultLoadPressureThreshold, snapshot.Threshold)
			assert.Equal(t, tc.supported, snapshot.Supported)
			assert.Equal(t, tc.available, snapshot.Available)
			assert.Equal(t, tc.overload, snapshot.Overloaded)
			assert.InDelta(t, tc.ratio, snapshot.NormalizedRatio, 0.0001)
			if tc.available {
				assert.Equal(t, tc.load5, snapshot.Load5)
				assert.Equal(t, tc.cpuCount, snapshot.CPUCount)
				require.Empty(t, snapshot.Diagnostic)
			} else {
				assert.Contains(t, snapshot.Diagnostic, tc.diagnostic)
			}
		})
	}

	for name, threshold := range map[string]float64{
		"NaN threshold":      math.NaN(),
		"infinite threshold": math.Inf(1),
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := (LoadPressureProbe{
				Load5:    func() (float64, bool, error) { return 8, true, nil },
				CPUCount: func() int { return 4 },
			}).Snapshot(threshold)
			assert.Equal(t, DefaultLoadPressureThreshold, snapshot.Threshold)
			assert.True(t, snapshot.Available)
			assert.False(t, snapshot.Overloaded)
		})
	}
}

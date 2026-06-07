package agent

import (
	"context"
	"strings"
	"time"
)

// ServerHealthProbe reports whether the worker's server sidecar is healthy
// enough to resume queue draining. Production callers should return true only
// after the server's /api/health endpoint is healthy and the lightweight smoke
// path succeeds again.
type ServerHealthProbe interface {
	Healthy(ctx context.Context) (bool, error)
}

type serverOutageSample struct {
	beadID string
	at     time.Time
}

type serverOutageTracker struct {
	window        time.Duration
	threshold     int
	probeInterval time.Duration

	active      bool
	reason      string
	nextProbeAt time.Time
	samples     []serverOutageSample
}

func newServerOutageTracker(window time.Duration, threshold int, probeInterval time.Duration) *serverOutageTracker {
	if window <= 0 {
		window = DefaultServerFailureWindow
	}
	if threshold <= 0 {
		threshold = DefaultServerFailureThreshold
	}
	if probeInterval <= 0 {
		probeInterval = DefaultServerHealthProbeInterval
	}
	return &serverOutageTracker{
		window:        window,
		threshold:     threshold,
		probeInterval: probeInterval,
	}
}

func (t *serverOutageTracker) Active() bool {
	if t == nil {
		return false
	}
	return t.active
}

func (t *serverOutageTracker) Activate(now time.Time, reason string) {
	if t == nil {
		return
	}
	t.active = true
	t.reason = reason
	t.samples = nil
	t.nextProbeAt = now.Add(t.probeInterval)
}

func (t *serverOutageTracker) Clear() {
	if t == nil {
		return
	}
	t.active = false
	t.reason = ""
	t.nextProbeAt = time.Time{}
	t.samples = nil
}

func (t *serverOutageTracker) NextProbeAt() time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.nextProbeAt
}

func (t *serverOutageTracker) Reason() string {
	if t == nil {
		return ""
	}
	return t.reason
}

func (t *serverOutageTracker) Record(report ExecuteBeadReport, beadID string, now time.Time) (activated bool, reason string) {
	if t == nil || t.active {
		return false, ""
	}
	if isServerTransportFailureReport(report) {
		t.Activate(now, FailureModeServerUnavailable)
		return true, FailureModeServerUnavailable
	}
	if !isNoViableProviderReport(report) {
		return false, ""
	}
	cutoff := now.Add(-t.window)
	filtered := t.samples[:0]
	distinct := make(map[string]struct{}, len(t.samples)+1)
	for _, sample := range t.samples {
		if sample.at.Before(cutoff) {
			continue
		}
		filtered = append(filtered, sample)
		if sample.beadID != "" {
			distinct[sample.beadID] = struct{}{}
		}
	}
	t.samples = append(filtered, serverOutageSample{beadID: beadID, at: now})
	if beadID != "" {
		distinct[beadID] = struct{}{}
	}
	if len(distinct) < t.threshold {
		return false, ""
	}
	t.Activate(now, FailureModeServerUnavailable)
	return true, FailureModeServerUnavailable
}

func (t *serverOutageTracker) ProbeDue(now time.Time) bool {
	if t == nil || !t.active {
		return false
	}
	return !now.Before(t.nextProbeAt)
}

func (t *serverOutageTracker) MarkProbeAttempt(now time.Time) {
	if t == nil || !t.active {
		return
	}
	t.nextProbeAt = now.Add(t.probeInterval)
}

func isServerTransportFailureReport(report ExecuteBeadReport) bool {
	if strings.TrimSpace(report.Provider) != "" {
		return false
	}
	combined := strings.ToLower(strings.Join([]string{
		report.Detail,
		report.Error,
		report.Stderr,
	}, "\n"))
	return isServerTransportFailureText(combined)
}

func isServerTransportFailureText(lower string) bool {
	if lower == "" {
		return false
	}
	if containsAny(lower,
		"tls handshake error",
		"remote error: tls: bad certificate",
		"server closed idle connection",
	) {
		return true
	}
	return containsAny(lower, "unexpected eof") &&
		containsAny(lower, "server", "api/health", "127.0.0.1", "localhost", "ddx")
}

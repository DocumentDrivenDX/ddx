package agent

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

const (
	// requestTimeoutArtifact is the per-attempt evidence file written when the
	// absolute provider-session wall-clock cap fires.
	requestTimeoutArtifact = "request-timeout.json"
	// reasonRequestTimeout is the durable outcome reason recorded in the
	// request-timeout evidence.
	reasonRequestTimeout = "request_timeout"

	requestTimeoutCleanupReaped     = "reaped"
	requestTimeoutCleanupNoChildren = "no_children"
)

// requestTimeoutEvidence is the durable record written when the absolute
// provider-session wall-clock cap (--request-timeout) fires for an
// execute-bead/work attempt. It names the configured cap, the elapsed
// wall-clock time at expiry, the attempt phase, the reaped provider PID, and
// the process-cleanup result so unattended burn-down failures are auditable
// (ddx-9febbad2).
type requestTimeoutEvidence struct {
	AttemptID                string                    `json:"attempt_id,omitempty"`
	BeadID                   string                    `json:"bead_id,omitempty"`
	Reason                   string                    `json:"reason"`
	Phase                    string                    `json:"phase"`
	ConfiguredTimeout        string                    `json:"configured_timeout"`
	ConfiguredTimeoutSeconds float64                   `json:"configured_timeout_seconds"`
	Elapsed                  string                    `json:"elapsed"`
	ElapsedSeconds           float64                   `json:"elapsed_seconds"`
	ProviderPID              int                       `json:"provider_pid"`
	CleanupResult            string                    `json:"cleanup_result"`
	ScannedAt                time.Time                 `json:"scanned_at"`
	Reaped                   []providerChildReapRecord `json:"reaped,omitempty"`
}

// reapRequestTimeoutAttempt performs the cleanup for an attempt that hit the
// absolute request-timeout cap: it reaps the attempt-scoped provider process
// tree under scopeDir (root-owned provider CLIs and their process groups),
// builds the wall-clock evidence record, and — when attemptID is set — writes
// it under the attempt's evidence bundle. It returns the evidence so callers
// and tests can assert on it directly.
func reapRequestTimeoutAttempt(projectRoot, attemptID, beadID, phase, scopeDir string, rootPID int, configured, elapsed time.Duration, now time.Time) requestTimeoutEvidence {
	reaped := reapAllProviderChildren(context.Background(), rootPID, scopeDir, now)
	ev := requestTimeoutEvidence{
		AttemptID:                attemptID,
		BeadID:                   beadID,
		Reason:                   reasonRequestTimeout,
		Phase:                    phase,
		ConfiguredTimeout:        configured.String(),
		ConfiguredTimeoutSeconds: configured.Seconds(),
		Elapsed:                  elapsed.String(),
		ElapsedSeconds:           elapsed.Seconds(),
		CleanupResult:            requestTimeoutCleanupNoChildren,
		ScannedAt:                now,
		Reaped:                   reaped,
	}
	if len(reaped) > 0 {
		ev.CleanupResult = requestTimeoutCleanupReaped
		ev.ProviderPID = reaped[0].PID
	}
	writeRequestTimeoutEvidence(projectRoot, attemptID, &ev)
	return ev
}

func writeRequestTimeoutEvidence(projectRoot, attemptID string, ev *requestTimeoutEvidence) {
	if ev == nil || strings.TrimSpace(projectRoot) == "" || strings.TrimSpace(attemptID) == "" {
		return
	}
	path := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, requestTimeoutArtifact)
	_ = writeArtifactJSON(path, ev)
}

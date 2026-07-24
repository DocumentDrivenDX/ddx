package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// managedWorkerResultFileName is the structured terminal-outcome file a
// server-managed `ddx work` subprocess writes into its own worker dir. The
// supervising WorkerManager reads it on exit to classify the terminal without
// parsing human-readable logs. See ddx-3d57bc30.
const managedWorkerResultFileName = "result.json"

// ManagedWorkerResult is the structured terminal outcome carried across the
// managed-worker subprocess boundary. A clean (exit 0) subprocess exit is
// otherwise indistinguishable from a real drain, which previously caused
// operator-attention stops (e.g. a dirty project root) to be relaunched in a
// tight loop instead of parking the worker.
type ManagedWorkerResult struct {
	// StopCondition mirrors ExecuteBeadLoopResult.StopCondition (e.g.
	// "drained", "operator_attention", "no_ready_work").
	StopCondition string `json:"stop_condition,omitempty"`
	// OperatorAttention is true when the loop stopped for a project-level
	// operator-attention condition (e.g. uncommitted tracked changes).
	OperatorAttention bool `json:"operator_attention,omitempty"`
	// LastFailureStatus mirrors ExecuteBeadLoopResult.LastFailureStatus so the
	// supervisor can block restarts for terminal contract failures even when a
	// future caller forgets to set OperatorAttention.
	LastFailureStatus string `json:"last_failure_status,omitempty"`
	LastFailureDetail string `json:"last_failure_detail,omitempty"`
	// ResourceExhaustionDiagnosis mirrors ExecuteBeadReport.ResourceExhaustionDiagnosis
	// (e.g. "fd_exhaustion") so status callers can classify the terminal without
	// brittle free-text matching.
	ResourceExhaustionDiagnosis string `json:"resource_exhaustion_diagnosis,omitempty"`
	// ResourceExhaustionRestartable mirrors ExecuteBeadReport.ResourceExhaustionRestartable.
	ResourceExhaustionRestartable bool `json:"resource_exhaustion_restartable,omitempty"`
}

// IsRestartBlocking reports whether this terminal outcome must suppress an
// immediate supervisor relaunch (the worker is parked pending operator action).
func (r ManagedWorkerResult) IsRestartBlocking() bool {
	stop := normalizeManagedWorkerReason(r.StopCondition)
	status := normalizeManagedWorkerReason(r.LastFailureStatus)
	return r.OperatorAttention || stop == "operator_attention" || status == "no_evidence_produced"
}

func normalizeManagedWorkerReason(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "-", "_")
	var out strings.Builder
	for i, r := range v {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(v[i-1])
			if prev != '_' && prev != '-' {
				out.WriteByte('_')
			}
		}
		out.WriteRune(r)
	}
	return strings.ToLower(out.String())
}

// managedWorkerResultDir returns the worker dir that both the subprocess and
// the supervising WorkerManager agree on for a given project root + worker id.
func managedWorkerResultDir(projectRoot, workerID string) string {
	return filepath.Join(ddxroot.JoinProject(projectRoot, "workers"), workerID)
}

// WriteManagedWorkerResult writes res to <workers>/<workerID>/result.json. It
// is called by a server-managed `ddx work` subprocess just before it exits so
// the supervising server can read the structured outcome. The worker dir is
// created by the server before launch, so a missing dir is a genuine error.
func WriteManagedWorkerResult(projectRoot, workerID string, res ManagedWorkerResult) error {
	dir := managedWorkerResultDir(projectRoot, workerID)
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, managedWorkerResultFileName), append(data, '\n'), 0o644)
}

// readManagedWorkerResult reads <dir>/result.json. It returns (nil, false)
// when the file is absent or unreadable so callers fall back to exit-code
// classification.
func readManagedWorkerResult(dir string) (*ManagedWorkerResult, bool) {
	data, err := os.ReadFile(filepath.Join(dir, managedWorkerResultFileName))
	if err != nil {
		return nil, false
	}
	var res ManagedWorkerResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, false
	}
	return &res, true
}

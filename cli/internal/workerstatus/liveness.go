package workerstatus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// LivenessRecord is the worker-side sidecar written under
// .ddx/workers/<worker-id>/status.json to mirror live progress of a
// long-running ddx work / ddx try attempt without rewriting beads.jsonl.
//
// The bead tracker remains the durable claim marker; this sidecar carries
// the high-frequency liveness signal so operators can answer "is the worker
// alive and what is it doing?" without inflating tracker churn.
type LivenessRecord struct {
	WorkerID         string          `json:"worker_id"`
	ProjectRoot      string          `json:"project_root,omitempty"`
	CurrentBead      string          `json:"current_bead,omitempty"`
	AttemptID        string          `json:"attempt_id,omitempty"`
	Phase            string          `json:"phase,omitempty"`
	Message          string          `json:"message,omitempty"`
	Route            string          `json:"route,omitempty"`
	Harness          string          `json:"harness,omitempty"`
	Model            string          `json:"model,omitempty"`
	Profile          string          `json:"profile,omitempty"`
	PID              int             `json:"pid,omitempty"`
	ChildPID         int             `json:"child_pid,omitempty"`
	ProviderChildren []ProviderChild `json:"provider_children,omitempty"`
	StartedAt        time.Time       `json:"started_at,omitempty"`
	LastActivityAt   time.Time       `json:"last_activity_at"`
}

// LivenessTTL is the freshness window for treating a worker sidecar as
// "active". Defaults to 3× the bead tracker heartbeat interval (which is
// 30s) so a sidecar that has not been touched in ~90s is treated as stale.
var LivenessTTL = 90 * time.Second

// LivenessDir returns the worker sidecar directory for a project.
func LivenessDir(projectRoot string) string {
	inTree := ddxroot.InTree(projectRoot)
	if info, err := os.Stat(inTree); err == nil && info.IsDir() {
		return filepath.Join(inTree, "workers")
	}
	return ddxroot.JoinProject(projectRoot, "workers")
}

// LivenessPath returns the status.json path for a worker under projectRoot.
func LivenessPath(projectRoot, workerID string) string {
	return filepath.Join(LivenessDir(projectRoot), workerID, "status.json")
}

// WriteLiveness writes rec to .ddx/workers/<workerID>/status.json using a
// tmp-then-rename so concurrent readers never see a half-written file.
func WriteLiveness(projectRoot, workerID string, rec LivenessRecord) error {
	if projectRoot == "" || workerID == "" {
		return fmt.Errorf("workerstatus: project root and worker id required")
	}
	dir := filepath.Join(LivenessDir(projectRoot), workerID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("workerstatus: mkdir worker dir: %w", err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("workerstatus: marshal liveness: %w", err)
	}
	data = append(data, '\n')
	final := filepath.Join(dir, "status.json")
	tmp, err := os.CreateTemp(dir, "status-*.tmp")
	if err != nil {
		return fmt.Errorf("workerstatus: create tmp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("workerstatus: write tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("workerstatus: close tmp: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("workerstatus: rename: %w", err)
	}
	return nil
}

// ReadLiveness reads the sidecar for workerID under projectRoot.
func ReadLiveness(projectRoot, workerID string) (LivenessRecord, error) {
	var rec LivenessRecord
	data, err := os.ReadFile(LivenessPath(projectRoot, workerID))
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, fmt.Errorf("workerstatus: parse liveness: %w", err)
	}
	return rec, nil
}

// ListLiveness reads every status.json sidecar under projectRoot's worker
// directory. Malformed sidecars are skipped silently — the sidecar is
// best-effort observability, not authoritative state.
func ListLiveness(projectRoot string) ([]LivenessRecord, error) {
	dir := LivenessDir(projectRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []LivenessRecord
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rec, err := ReadLiveness(projectRoot, entry.Name())
		if err != nil {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastActivityAt.After(out[j].LastActivityAt)
	})
	return out, nil
}

// IsFresh reports whether rec.LastActivityAt is within LivenessTTL of now.
func (r LivenessRecord) IsFresh(now time.Time) bool {
	if r.LastActivityAt.IsZero() {
		return false
	}
	return now.Sub(r.LastActivityAt) <= LivenessTTL
}

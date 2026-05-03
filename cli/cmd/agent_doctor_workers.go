package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// ADR-022 step 6: `ddx agent doctor --workers` reads the runtime worker
// view from the server's in-memory registry when the server is reachable;
// falls back to scanning .ddx/workers/<id>/status.json on disk when not.
// Output shape stays consistent across both modes so operators don't have
// to re-learn the table when the server bounces.

// doctorWorkerView is the unified row produced by both the server-runtime
// path and the on-disk fallback. Fields not present in a given source
// (e.g. freshness on disk; state in the runtime registry) are left at the
// zero value and rendered as "-".
type doctorWorkerView struct {
	WorkerID            string    `json:"worker_id"`
	ProjectRoot         string    `json:"project_root,omitempty"`
	Harness             string    `json:"harness,omitempty"`
	Model               string    `json:"model,omitempty"`
	State               string    `json:"state,omitempty"`
	Freshness           string    `json:"freshness,omitempty"`
	MirrorFailuresCount int       `json:"mirror_failures_count"`
	LastEventAt         time.Time `json:"last_event_at,omitempty"`
	Source              string    `json:"source"`
}

// runAgentDoctorWorkers implements the --workers branch of `ddx agent doctor`.
// Tries the server's runtime registry first; on any error (DNS, connection
// refused, non-200, decode failure) falls back to disk scanning. Both paths
// produce a single rendered table and return nil so doctor stays advisory.
func (f *CommandFactory) runAgentDoctorWorkers(cmd *cobra.Command, projectRoot string, asJSON bool) error {
	workers, source := fetchDoctorWorkers(cmd.Context(), projectRoot)
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Source  string             `json:"source"`
			Workers []doctorWorkerView `json:"workers"`
		}{Source: source, Workers: workers})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Worker source: %s\n", source)
	if len(workers) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no active workers")
		return nil
	}
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "WORKER\tHARNESS\tFRESHNESS\tSTATE\tMIRROR_FAILS\tLAST_EVENT")
	for _, w := range workers {
		harness := dashIfEmpty(w.Harness)
		fresh := dashIfEmpty(w.Freshness)
		state := dashIfEmpty(w.State)
		last := "-"
		if !w.LastEventAt.IsZero() {
			last = w.LastEventAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
			w.WorkerID, harness, fresh, state, w.MirrorFailuresCount, last)
	}
	return tw.Flush()
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// fetchDoctorWorkers chooses runtime vs disk source, returning the rendered
// rows and the source label (server|disk) for the operator.
func fetchDoctorWorkers(ctx context.Context, projectRoot string) ([]doctorWorkerView, string) {
	if rows, ok := fetchDoctorWorkersFromServer(ctx, projectRoot); ok {
		return rows, "server"
	}
	return scanDoctorWorkersOnDisk(projectRoot), "disk"
}

func fetchDoctorWorkersFromServer(ctx context.Context, projectRoot string) ([]doctorWorkerView, bool) {
	base := resolveServerURL(projectRoot)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/workers", nil)
	if err != nil {
		return nil, false
	}
	resp, err := newLocalServerClient().Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, false
	}
	var raw []struct {
		WorkerID            string    `json:"worker_id"`
		ProjectRoot         string    `json:"project_root"`
		Harness             string    `json:"harness,omitempty"`
		Model               string    `json:"model,omitempty"`
		LastEventAt         time.Time `json:"last_event_at"`
		MirrorFailuresCount int       `json:"mirror_failures_count"`
		Freshness           string    `json:"freshness"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, false
	}
	out := make([]doctorWorkerView, 0, len(raw))
	for _, r := range raw {
		out = append(out, doctorWorkerView{
			WorkerID:            r.WorkerID,
			ProjectRoot:         r.ProjectRoot,
			Harness:             r.Harness,
			Model:               r.Model,
			Freshness:           r.Freshness,
			LastEventAt:         r.LastEventAt,
			MirrorFailuresCount: r.MirrorFailuresCount,
			Source:              "server",
		})
	}
	return out, true
}

// scanDoctorWorkersOnDisk reads .ddx/workers/<id>/status.json files written
// by the legacy WorkerManager. Kept as the fallback-source-of-truth for one
// alpha release per ADR-022 rev 5.
func scanDoctorWorkersOnDisk(projectRoot string) []doctorWorkerView {
	root := filepath.Join(projectRoot, ".ddx", "workers")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []doctorWorkerView
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, e.Name(), "status.json"))
		if err != nil {
			continue
		}
		var rec struct {
			ID          string    `json:"id"`
			State       string    `json:"state"`
			ProjectRoot string    `json:"project_root"`
			Harness     string    `json:"harness,omitempty"`
			Model       string    `json:"model,omitempty"`
			StartedAt   time.Time `json:"started_at,omitempty"`
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		out = append(out, doctorWorkerView{
			WorkerID:    rec.ID,
			ProjectRoot: rec.ProjectRoot,
			Harness:     rec.Harness,
			Model:       rec.Model,
			State:       rec.State,
			LastEventAt: rec.StartedAt,
			Source:      "disk",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastEventAt.After(out[j].LastEventAt) })
	return out
}

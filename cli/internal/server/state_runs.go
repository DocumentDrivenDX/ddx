package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// runStoreDir is the FEAT-010 run store directory relative to the project root.
const runStoreDir = ".ddx/exec/runs"

// runRecord is the on-disk JSON format for a FEAT-010 run file.
type runRecord struct {
	ID          string   `json:"id"`
	Layer       string   `json:"layer"`
	Status      string   `json:"status"`
	StartedAt   string   `json:"started_at,omitempty"`
	CompletedAt string   `json:"completed_at,omitempty"`
	BeadID      string   `json:"bead_id,omitempty"`
	ArtifactID  string   `json:"artifact_id,omitempty"`
	ParentRunID string   `json:"parent_run_id,omitempty"`
	ChildRunIDs []string `json:"child_run_ids,omitempty"`

	// Work-layer fields
	QueueInputs     json.RawMessage `json:"queue_inputs,omitempty"`
	StopCondition   string          `json:"stop_condition,omitempty"`
	SelectedBeadIDs []string        `json:"selected_bead_ids,omitempty"`

	// Try-layer fields
	BaseRevision   string          `json:"base_revision,omitempty"`
	ResultRevision string          `json:"result_revision,omitempty"`
	WorktreePath   string          `json:"worktree_path,omitempty"`
	MergeOutcome   string          `json:"merge_outcome,omitempty"`
	CheckResults   json.RawMessage `json:"check_results,omitempty"`

	// Run-layer fields
	PromptSummary string   `json:"prompt_summary,omitempty"`
	PowerMin      int      `json:"power_min,omitempty"`
	PowerMax      int      `json:"power_max,omitempty"`
	Harness       string   `json:"harness,omitempty"`
	Provider      string   `json:"provider,omitempty"`
	Model         string   `json:"model,omitempty"`
	TokensIn      int      `json:"tokens_in,omitempty"`
	TokensOut     int      `json:"tokens_out,omitempty"`
	CostUSD       float64  `json:"cost_usd,omitempty"`
	DurationMs    int      `json:"duration_ms,omitempty"`
	OutputExcerpt string   `json:"output_excerpt,omitempty"`
	EvidenceLinks []string `json:"evidence_links,omitempty"`
}

// runRecordToGQL converts a runRecord to the GraphQL Run type.
func runRecordToGQL(rec runRecord) *ddxgraphql.Run {
	layer := ddxgraphql.RunLayerRun
	switch strings.ToLower(rec.Layer) {
	case "work":
		layer = ddxgraphql.RunLayerWork
	case "try":
		layer = ddxgraphql.RunLayerTry
	}

	run := &ddxgraphql.Run{
		ID:          rec.ID,
		Layer:       layer,
		Status:      rec.Status,
		ChildRunIds: rec.ChildRunIDs,
	}
	if run.ChildRunIds == nil {
		run.ChildRunIds = []string{}
	}
	if rec.StartedAt != "" {
		s := normalizeISOTime(rec.StartedAt)
		run.StartedAt = &s
	}
	if rec.CompletedAt != "" {
		s := normalizeISOTime(rec.CompletedAt)
		run.CompletedAt = &s
	}
	if rec.BeadID != "" {
		run.BeadID = &rec.BeadID
	}
	if rec.ArtifactID != "" {
		run.ArtifactID = &rec.ArtifactID
	}
	if rec.ParentRunID != "" {
		run.ParentRunID = &rec.ParentRunID
	}

	switch layer {
	case ddxgraphql.RunLayerWork:
		if len(rec.QueueInputs) > 0 {
			s := string(rec.QueueInputs)
			run.QueueInputs = &s
		}
		if rec.StopCondition != "" {
			run.StopCondition = &rec.StopCondition
		}
		if len(rec.SelectedBeadIDs) > 0 {
			run.SelectedBeadIds = rec.SelectedBeadIDs
		}
	case ddxgraphql.RunLayerTry:
		if rec.BaseRevision != "" {
			run.BaseRevision = &rec.BaseRevision
		}
		if rec.ResultRevision != "" {
			run.ResultRevision = &rec.ResultRevision
		}
		if rec.WorktreePath != "" {
			run.WorktreePath = &rec.WorktreePath
		}
		if rec.MergeOutcome != "" {
			run.MergeOutcome = &rec.MergeOutcome
		}
		if len(rec.CheckResults) > 0 {
			s := string(rec.CheckResults)
			run.CheckResults = &s
		}
	case ddxgraphql.RunLayerRun:
		if rec.PromptSummary != "" {
			run.PromptSummary = &rec.PromptSummary
		}
		if rec.PowerMin > 0 {
			run.PowerMin = &rec.PowerMin
		}
		if rec.PowerMax > 0 {
			run.PowerMax = &rec.PowerMax
		}
		if rec.Harness != "" {
			run.Harness = &rec.Harness
		}
		if rec.Provider != "" {
			run.Provider = &rec.Provider
		}
		if rec.Model != "" {
			run.Model = &rec.Model
		}
		if rec.TokensIn > 0 {
			run.TokensIn = &rec.TokensIn
		}
		if rec.TokensOut > 0 {
			run.TokensOut = &rec.TokensOut
		}
		if rec.CostUSD != 0 {
			run.CostUsd = &rec.CostUSD
		}
		if rec.DurationMs > 0 {
			run.DurationMs = &rec.DurationMs
		}
		if rec.OutputExcerpt != "" {
			run.OutputExcerpt = &rec.OutputExcerpt
		}
		if len(rec.EvidenceLinks) > 0 {
			run.EvidenceLinks = rec.EvidenceLinks
		}
	}
	return run
}

// scanRunStore reads FEAT-010 run JSON files from .ddx/exec/runs/.
func scanRunStore(projectRoot string) []*ddxgraphql.Run {
	dir := filepath.Join(projectRoot, runStoreDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]*ddxgraphql.Run, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var rec runRecord
		if err := json.Unmarshal(data, &rec); err != nil || rec.ID == "" {
			continue
		}
		out = append(out, runRecordToGQL(rec))
	}
	return out
}

// synthesizeRunsFromBundles derives run-layer Run records from .ddx/executions/
// bundle directories. Each bundle = one `run`-layer agent invocation.
func synthesizeRunsFromBundles(projectID, projectRoot string) []*ddxgraphql.Run {
	dir := filepath.Join(projectRoot, agent.ExecuteBeadArtifactDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]*ddxgraphql.Run, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !looksLikeBundleID(name) {
			continue
		}
		bundleDirAbs := filepath.Join(dir, name)
		exec := loadExecutionBundle(projectID, projectRoot, bundleDirAbs, name)
		if exec == nil {
			continue
		}
		run := executionBundleToRun(name, exec)
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool {
		si, sj := "", ""
		if out[i].StartedAt != nil {
			si = *out[i].StartedAt
		}
		if out[j].StartedAt != nil {
			sj = *out[j].StartedAt
		}
		return si > sj
	})
	return out
}

// executionBundleToRun converts an Execution bundle to a run-layer Run record.
func executionBundleToRun(id string, exec *ddxgraphql.Execution) *ddxgraphql.Run {
	status := "success"
	if exec.Status != nil {
		status = *exec.Status
	} else if exec.Verdict != nil {
		status = strings.ToLower(*exec.Verdict)
	}

	run := &ddxgraphql.Run{
		ID:          "exec-" + id,
		Layer:       ddxgraphql.RunLayerRun,
		Status:      status,
		ChildRunIds: []string{},
	}
	if exec.StartedAt != nil {
		run.StartedAt = exec.StartedAt
	} else {
		// Fall back to createdAt
		s := exec.CreatedAt
		run.StartedAt = &s
	}
	if exec.FinishedAt != nil {
		run.CompletedAt = exec.FinishedAt
	}
	if exec.BeadID != nil {
		run.BeadID = exec.BeadID
	}
	if exec.Harness != nil {
		run.Harness = exec.Harness
	}
	if exec.Model != nil {
		run.Model = exec.Model
	}
	if exec.DurationMs != nil {
		run.DurationMs = exec.DurationMs
	}
	if exec.CostUsd != nil {
		run.CostUsd = exec.CostUsd
	}
	if exec.Tokens != nil {
		t := *exec.Tokens
		run.TokensIn = &t
	}
	run.EvidenceLinks = []string{exec.BundlePath}
	return run
}

// GetRunsGraphQL implements RunsStateProvider.
func (s *ServerState) GetRunsGraphQL(projectID string, filter ddxgraphql.RunFilter) []*ddxgraphql.Run {
	var all []*ddxgraphql.Run

	if projectID != "" {
		proj, ok := s.GetProjectByID(projectID)
		if !ok {
			return nil
		}
		all = s.loadRunsForProject(projectID, proj.Path)
	} else {
		for _, proj := range s.GetProjects(false) {
			all = append(all, s.loadRunsForProject(proj.ID, proj.Path)...)
		}
		sort.Slice(all, func(i, j int) bool {
			si, sj := "", ""
			if all[i].StartedAt != nil {
				si = *all[i].StartedAt
			}
			if all[j].StartedAt != nil {
				sj = *all[j].StartedAt
			}
			return si > sj
		})
	}
	return ddxgraphql.ApplyRunFilter(all, filter)
}

// tagProjectID sets ProjectID on each run in-place.
func tagProjectID(runs []*ddxgraphql.Run, projectID string) []*ddxgraphql.Run {
	for _, r := range runs {
		pid := projectID
		r.ProjectID = &pid
	}
	return runs
}

// loadRunsForProject loads runs for a single project, merging the file store
// with synthesized execution bundle records.
func (s *ServerState) loadRunsForProject(projectID, projectRoot string) []*ddxgraphql.Run {
	// Primary: FEAT-010 run store
	stored := scanRunStore(projectRoot)

	// Secondary: synthesize run-layer records from .ddx/executions/ bundles,
	// but only include IDs that are not already in the stored set.
	storedIDs := make(map[string]bool, len(stored))
	for _, r := range stored {
		storedIDs[r.ID] = true
	}

	synthesized := synthesizeRunsFromBundles(projectID, projectRoot)
	for _, r := range synthesized {
		if !storedIDs[r.ID] {
			stored = append(stored, r)
		}
	}

	// Sort newest-first by startedAt.
	sort.Slice(stored, func(i, j int) bool {
		si, sj := "", ""
		if stored[i].StartedAt != nil {
			si = *stored[i].StartedAt
		}
		if stored[j].StartedAt != nil {
			sj = *stored[j].StartedAt
		}
		return si > sj
	})
	return tagProjectID(stored, projectID)
}

// GetRunGraphQL implements RunsStateProvider.
func (s *ServerState) GetRunGraphQL(id string) (*ddxgraphql.Run, bool) {
	if id == "" {
		return nil, false
	}
	// Search all projects.
	for _, proj := range s.GetProjects(false) {
		runs := s.loadRunsForProject(proj.ID, proj.Path)
		for _, r := range runs {
			if r.ID == id {
				return r, true
			}
		}
	}
	return nil, false
}

package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

// synthesizeRunsFromBundles derives try-layer Run records from .ddx/executions/
// bundle directories. Each bundle = one execute-bead attempt (a "try" in the
// three-layer model). Run-layer records are synthesized separately from the
// AgentSession index, since multiple agent invocations may share a try.
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

// executionBundleToRun converts an Execution bundle to a try-layer Run record.
// Bundles correspond to one execute-bead attempt (a try): they carry
// base/result revisions, a verdict, and the bead under attempt. Per-invocation
// fields (harness, model, tokens) live on the run-layer records synthesized
// from AgentSession instead.
func executionBundleToRun(id string, exec *ddxgraphql.Execution) *ddxgraphql.Run {
	status := "success"
	if exec.Status != nil {
		status = *exec.Status
	} else if exec.Verdict != nil {
		status = strings.ToLower(*exec.Verdict)
	}

	run := &ddxgraphql.Run{
		ID:          "exec-" + id,
		Layer:       ddxgraphql.RunLayerTry,
		Status:      status,
		ChildRunIds: []string{},
	}
	if exec.StartedAt != nil {
		run.StartedAt = exec.StartedAt
	} else {
		s := exec.CreatedAt
		run.StartedAt = &s
	}
	if exec.FinishedAt != nil {
		run.CompletedAt = exec.FinishedAt
	}
	if exec.BeadID != nil {
		run.BeadID = exec.BeadID
	}
	if exec.BaseRev != nil {
		run.BaseRevision = exec.BaseRev
	}
	if exec.ResultRev != nil {
		run.ResultRevision = exec.ResultRev
	}
	if outcome := mergeOutcomeFromVerdict(exec); outcome != "" {
		run.MergeOutcome = &outcome
	}
	run.EvidenceLinks = []string{exec.BundlePath}
	return run
}

// mergeOutcomeFromVerdict normalizes the bundle verdict/status into the
// {merged, preserved} space the try layer reports.
func mergeOutcomeFromVerdict(exec *ddxgraphql.Execution) string {
	v := ""
	if exec.Verdict != nil {
		v = strings.ToLower(*exec.Verdict)
	}
	if v == "" && exec.Status != nil {
		v = strings.ToLower(*exec.Status)
	}
	switch v {
	case "preserved", "preserve":
		return "preserved"
	case "merged", "merge", "success", "pass":
		if exec.ResultRev != nil && *exec.ResultRev != "" {
			return "merged"
		}
		return ""
	}
	return ""
}

// synthesizeRunsFromSessions derives run-layer Run records from the agent
// session index. Each AgentSession entry = one agent invocation (one "run"
// in the three-layer model). When the session points at a bundle directory,
// the resulting Run is parented to the matching try-layer record.
func synthesizeRunsFromSessions(projectID, projectRoot string) []*ddxgraphql.Run {
	logDir := agent.SessionLogDirForWorkDir(projectRoot)
	entries, err := agent.ReadSessionIndex(logDir, agent.SessionIndexQuery{DefaultRecent: true})
	if err != nil {
		return nil
	}
	out := make([]*ddxgraphql.Run, 0, len(entries))
	for _, e := range entries {
		out = append(out, sessionEntryToRun(e))
	}
	return out
}

// sessionEntryToRun projects one SessionIndexEntry onto a run-layer Run.
// AgentSession is the canonical backing store under layer=run; this projection
// maps the fields that exist on the Run schema today (id, harness, provider,
// model, tokens, cost, duration, status). Session-only fields (billingMode,
// cached tokens, prompt/response/stderr, outcome detail) remain accessible
// through the AgentSession query keyed by the same id.
func sessionEntryToRun(e agent.SessionIndexEntry) *ddxgraphql.Run {
	status := strings.ToLower(e.Outcome)
	if status == "" {
		if e.ExitCode == 0 {
			status = "success"
		} else {
			status = "failure"
		}
	}
	run := &ddxgraphql.Run{
		ID:          e.ID,
		Layer:       ddxgraphql.RunLayerRun,
		Status:      status,
		ChildRunIds: []string{},
	}
	if !e.StartedAt.IsZero() {
		s := e.StartedAt.UTC().Format(time.RFC3339)
		run.StartedAt = &s
	}
	if !e.EndedAt.IsZero() {
		s := e.EndedAt.UTC().Format(time.RFC3339)
		run.CompletedAt = &s
	} else if !e.StartedAt.IsZero() && e.DurationMS > 0 {
		s := e.StartedAt.Add(time.Duration(e.DurationMS) * time.Millisecond).UTC().Format(time.RFC3339)
		run.CompletedAt = &s
	}
	if e.BeadID != "" {
		bid := e.BeadID
		run.BeadID = &bid
	}
	if e.Harness != "" {
		h := e.Harness
		run.Harness = &h
	}
	if e.Provider != "" {
		p := e.Provider
		run.Provider = &p
	}
	if e.Model != "" {
		m := e.Model
		run.Model = &m
	}
	if e.InputTokens > 0 {
		v := e.InputTokens
		run.TokensIn = &v
	}
	if e.OutputTokens > 0 {
		v := e.OutputTokens
		run.TokensOut = &v
	}
	if e.CostUSD != 0 {
		c := e.CostUSD
		run.CostUsd = &c
	}
	if e.DurationMS > 0 {
		d := e.DurationMS
		run.DurationMs = &d
	}
	var evidence []string
	if e.BundlePath != "" {
		evidence = append(evidence, filepath.ToSlash(e.BundlePath))
		if parent := parentTryIDFromBundlePath(e.BundlePath); parent != "" {
			run.ParentRunID = &parent
		}
	}
	if e.NativeLogRef != "" {
		evidence = append(evidence, filepath.ToSlash(e.NativeLogRef))
	}
	if len(evidence) > 0 {
		run.EvidenceLinks = evidence
	}
	return run
}

// parentTryIDFromBundlePath returns the synthesized try-layer Run ID
// ("exec-<bundle-id>") for a bundle path under .ddx/executions/, or "" when
// the path does not look like a bundle.
func parentTryIDFromBundlePath(p string) string {
	clean := filepath.ToSlash(p)
	prefix := filepath.ToSlash(agent.ExecuteBeadArtifactDir) + "/"
	idx := strings.Index(clean, prefix)
	if idx < 0 {
		return ""
	}
	rest := clean[idx+len(prefix):]
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	if !looksLikeBundleID(rest) {
		return ""
	}
	return "exec-" + rest
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
			storedIDs[r.ID] = true
		}
	}

	sessions := synthesizeRunsFromSessions(projectID, projectRoot)
	for _, r := range sessions {
		if !storedIDs[r.ID] {
			stored = append(stored, r)
			storedIDs[r.ID] = true
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

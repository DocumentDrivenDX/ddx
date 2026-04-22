package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// GetNodeSnapshot implements ddxgraphql.StateProvider.
func (s *ServerState) GetNodeSnapshot() ddxgraphql.NodeStateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ddxgraphql.NodeStateSnapshot{
		ID:        s.Node.ID,
		Name:      s.Node.Name,
		StartedAt: s.Node.StartedAt,
		LastSeen:  s.Node.LastSeen,
	}
}

// GetProjectSnapshots implements ddxgraphql.StateProvider.
func (s *ServerState) GetProjectSnapshots(includeUnreachable bool) []*ddxgraphql.Project {
	entries := s.GetProjects(includeUnreachable)
	out := make([]*ddxgraphql.Project, len(entries))
	for i, e := range entries {
		out[i] = projectEntryToGQL(e)
	}
	return out
}

// GetProjectSnapshotByID implements ddxgraphql.StateProvider.
func (s *ServerState) GetProjectSnapshotByID(id string) (*ddxgraphql.Project, bool) {
	entry, ok := s.GetProjectByID(id)
	if !ok {
		return nil, false
	}
	return projectEntryToGQL(entry), true
}

type beadIndexEntry struct {
	ProjectID   string
	ProjectPath string
}

// GetBeadSnapshots implements ddxgraphql.StateProvider.
func (s *ServerState) GetBeadSnapshots(status, label, projectID, search string) []ddxgraphql.BeadSnapshot {
	projects := s.GetProjects()
	var result []ddxgraphql.BeadSnapshot
	for _, proj := range projects {
		if projectID != "" && proj.ID != projectID {
			continue
		}
		store := bead.NewStore(filepath.Join(proj.Path, ".ddx"))
		beads, err := store.ReadAll()
		if err != nil {
			continue
		}
		for _, b := range beads {
			if status != "" && b.Status != status {
				continue
			}
			if label != "" && !containsString(b.Labels, label) {
				continue
			}
			if search != "" {
				q := strings.ToLower(search)
				if !strings.Contains(strings.ToLower(b.ID), q) && !strings.Contains(strings.ToLower(b.Title), q) {
					continue
				}
			}
			s.rememberBeadLocation(b.ID, proj)
			snap := beadSnapshotFromStoreBead(proj.ID, b)
			result = append(result, snap)
		}
	}
	return result
}

// GetBeadSnapshot implements ddxgraphql.StateProvider.
func (s *ServerState) GetBeadSnapshot(id string) (*ddxgraphql.BeadSnapshot, bool) {
	if id == "" {
		return nil, false
	}

	if loc, ok := s.lookupBeadLocation(id); ok {
		proj := ProjectEntry{ID: loc.ProjectID, Path: loc.ProjectPath}
		if snap, ok := readBeadSnapshotFromProject(proj, id); ok {
			return snap, true
		}
		s.forgetBeadLocation(id)
	}

	for _, proj := range s.GetProjects() {
		if snap, ok := readBeadSnapshotFromProject(proj, id); ok {
			s.rememberBeadLocation(id, proj)
			return snap, true
		}
	}
	return nil, false
}

func (s *ServerState) rememberBeadLocation(id string, proj ProjectEntry) {
	if id == "" || proj.ID == "" || proj.Path == "" {
		return
	}
	s.beadIndexMu.Lock()
	defer s.beadIndexMu.Unlock()
	if s.beadIndex == nil {
		s.beadIndex = make(map[string]beadIndexEntry)
	}
	s.beadIndex[id] = beadIndexEntry{ProjectID: proj.ID, ProjectPath: proj.Path}
}

func (s *ServerState) lookupBeadLocation(id string) (beadIndexEntry, bool) {
	s.beadIndexMu.RLock()
	defer s.beadIndexMu.RUnlock()
	if s.beadIndex == nil {
		return beadIndexEntry{}, false
	}
	loc, ok := s.beadIndex[id]
	return loc, ok
}

func (s *ServerState) forgetBeadLocation(id string) {
	s.beadIndexMu.Lock()
	defer s.beadIndexMu.Unlock()
	delete(s.beadIndex, id)
}

func readBeadSnapshotFromProject(proj ProjectEntry, id string) (*ddxgraphql.BeadSnapshot, bool) {
	store := bead.NewStore(filepath.Join(proj.Path, ".ddx"))
	b, err := store.Get(id)
	if err != nil {
		return nil, false
	}
	snap := beadSnapshotFromStoreBead(proj.ID, *b)
	return &snap, true
}

func beadSnapshotFromStoreBead(projectID string, b bead.Bead) ddxgraphql.BeadSnapshot {
	snap := ddxgraphql.BeadSnapshot{
		ProjectID:   projectID,
		ID:          b.ID,
		Title:       b.Title,
		Status:      b.Status,
		Priority:    b.Priority,
		IssueType:   b.IssueType,
		Owner:       b.Owner,
		CreatedAt:   b.CreatedAt,
		CreatedBy:   b.CreatedBy,
		UpdatedAt:   b.UpdatedAt,
		Labels:      b.Labels,
		Parent:      b.Parent,
		Description: b.Description,
		Acceptance:  b.Acceptance,
		Notes:       b.Notes,
	}
	for _, d := range b.Dependencies {
		snap.Dependencies = append(snap.Dependencies, ddxgraphql.BeadDependencySnapshot{
			IssueID:     d.IssueID,
			DependsOnID: d.DependsOnID,
			Type:        d.Type,
			CreatedAt:   d.CreatedAt,
			CreatedBy:   d.CreatedBy,
			Metadata:    d.Metadata,
		})
	}
	return snap
}

func projectEntryToGQL(e ProjectEntry) *ddxgraphql.Project {
	p := &ddxgraphql.Project{
		ID:           e.ID,
		Name:         e.Name,
		Path:         e.Path,
		RegisteredAt: e.RegisteredAt.UTC().Format(time.RFC3339),
		LastSeen:     e.LastSeen.UTC().Format(time.RFC3339),
	}
	if e.GitRemote != "" {
		p.GitRemote = &e.GitRemote
	}
	if e.Unreachable {
		b := true
		p.Unreachable = &b
	}
	if e.TombstonedAt != nil {
		ts := e.TombstonedAt.UTC().Format(time.RFC3339)
		p.TombstonedAt = &ts
	}
	return p
}

// ─── Worker queries ──────────────────────────────────────────────────────────

// GetWorkersGraphQL implements ddxgraphql.StateProvider.
// Reads worker records from disk and returns them as GraphQL Worker values.
// If projectID is non-empty, only workers for that project root are returned.
func (s *ServerState) GetWorkersGraphQL(projectID string) []*ddxgraphql.Worker {
	if s.workingDir == "" {
		return nil
	}
	workersDir := filepath.Join(s.workingDir, ".ddx", "workers")
	entries, err := os.ReadDir(workersDir)
	if err != nil {
		return nil
	}
	var out []*ddxgraphql.Worker
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(workersDir, entry.Name(), "status.json"))
		if err != nil {
			continue
		}
		var rec WorkerRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		if projectID != "" && rec.ProjectRoot != projectID {
			continue
		}
		out = append(out, workerFromRecord(rec))
	}
	// Newest first.
	sort.Slice(out, func(i, j int) bool {
		si, sj := out[i].StartedAt, out[j].StartedAt
		if si == nil || sj == nil {
			return si != nil
		}
		ti, _ := time.Parse(time.RFC3339, *si)
		tj, _ := time.Parse(time.RFC3339, *sj)
		return ti.After(tj)
	})
	return out
}

// GetWorkerGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetWorkerGraphQL(id string) (*ddxgraphql.Worker, bool) {
	if s.workingDir == "" {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Join(s.workingDir, ".ddx", "workers", id, "status.json"))
	if err != nil {
		return nil, false
	}
	var rec WorkerRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}
	return workerFromRecord(rec), true
}

// GetWorkerLogGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetWorkerLogGraphQL(id string) *ddxgraphql.WorkerLog {
	if s.workingDir == "" {
		return &ddxgraphql.WorkerLog{}
	}
	// Read the worker record to find the log path.
	data, err := os.ReadFile(filepath.Join(s.workingDir, ".ddx", "workers", id, "status.json"))
	if err != nil {
		return &ddxgraphql.WorkerLog{}
	}
	var rec WorkerRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return &ddxgraphql.WorkerLog{}
	}
	stdout := ""
	if rec.StdoutPath != "" {
		logPath := rec.StdoutPath
		if !filepath.IsAbs(logPath) {
			logPath = filepath.Join(s.workingDir, logPath)
		}
		if raw, err := os.ReadFile(logPath); err == nil {
			stdout = string(raw)
		}
	}
	return &ddxgraphql.WorkerLog{Stdout: stdout, Stderr: ""}
}

// GetWorkerProgressGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetWorkerProgressGraphQL(id string) []*ddxgraphql.PhaseTransition {
	if s.workingDir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(s.workingDir, ".ddx", "workers", id, "status.json"))
	if err != nil {
		return nil
	}
	var rec WorkerRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil
	}
	out := make([]*ddxgraphql.PhaseTransition, len(rec.RecentPhases))
	for i, p := range rec.RecentPhases {
		out[i] = &ddxgraphql.PhaseTransition{
			Phase:    p.Phase,
			Ts:       p.TS.UTC().Format(time.RFC3339),
			PhaseSeq: p.PhaseSeq,
		}
	}
	return out
}

// GetWorkerPromptGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetWorkerPromptGraphQL(id string) string {
	if s.workingDir == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(s.workingDir, ".ddx", "workers", id, "spec.json"))
	if err != nil {
		return ""
	}
	return string(raw)
}

// workerFromRecord converts a WorkerRecord to the GraphQL Worker type.
func workerFromRecord(rec WorkerRecord) *ddxgraphql.Worker {
	attempts := rec.Attempts
	successes := rec.Successes
	failures := rec.Failures
	w := &ddxgraphql.Worker{
		ID:           rec.ID,
		Kind:         rec.Kind,
		State:        rec.State,
		ProjectRoot:  rec.ProjectRoot,
		Attempts:     &attempts,
		Successes:    &successes,
		Failures:     &failures,
		RecentEvents: []*ddxgraphql.WorkerRecentEvent{},
	}
	if rec.Status != "" {
		w.Status = &rec.Status
	}
	if rec.Harness != "" {
		w.Harness = &rec.Harness
	}
	if rec.Provider != "" {
		w.Provider = &rec.Provider
	}
	if rec.Model != "" {
		w.Model = &rec.Model
	}
	if rec.Effort != "" {
		w.Effort = &rec.Effort
	}
	if rec.Once {
		b := true
		w.Once = &b
	}
	if rec.PollInterval != "" {
		w.PollInterval = &rec.PollInterval
	}
	if !rec.StartedAt.IsZero() {
		s := rec.StartedAt.UTC().Format(time.RFC3339)
		w.StartedAt = &s
	}
	if !rec.FinishedAt.IsZero() {
		s := rec.FinishedAt.UTC().Format(time.RFC3339)
		w.FinishedAt = &s
	}
	if rec.Error != "" {
		w.Error = &rec.Error
	}
	if rec.StdoutPath != "" {
		w.StdoutPath = &rec.StdoutPath
	}
	if rec.SpecPath != "" {
		w.SpecPath = &rec.SpecPath
	}
	if rec.CurrentBead != "" {
		w.CurrentBead = &rec.CurrentBead
	}
	if rec.LastError != "" {
		w.LastError = &rec.LastError
	}
	if rec.LastResult != nil {
		w.LastResult = workerResultFromRecord(rec.LastResult)
	}
	if rec.CurrentAttempt != nil {
		w.CurrentAttempt = currentAttemptFromRecord(rec.CurrentAttempt)
	}
	for _, p := range rec.RecentPhases {
		pt := p // copy
		w.RecentPhases = append(w.RecentPhases, &ddxgraphql.PhaseTransition{
			Phase:    pt.Phase,
			Ts:       pt.TS.UTC().Format(time.RFC3339),
			PhaseSeq: pt.PhaseSeq,
		})
	}
	if rec.LastAttempt != nil {
		w.LastAttempt = lastAttemptFromRecord(rec.LastAttempt)
	}
	if rec.LandSummary != nil {
		w.LandSummary = landSummaryFromRecord(rec.LandSummary)
	}
	return w
}

func workerResultFromRecord(r *WorkerExecutionResult) *ddxgraphql.WorkerExecutionResult {
	out := &ddxgraphql.WorkerExecutionResult{}
	if r.BeadID != "" {
		out.BeadID = &r.BeadID
	}
	if r.AttemptID != "" {
		out.AttemptID = &r.AttemptID
	}
	if r.WorkerID != "" {
		out.WorkerID = &r.WorkerID
	}
	if r.Harness != "" {
		out.Harness = &r.Harness
	}
	if r.Provider != "" {
		out.Provider = &r.Provider
	}
	if r.Model != "" {
		out.Model = &r.Model
	}
	if r.Status != "" {
		out.Status = &r.Status
	}
	if r.Detail != "" {
		out.Detail = &r.Detail
	}
	if r.SessionID != "" {
		out.SessionID = &r.SessionID
	}
	if r.BaseRev != "" {
		out.BaseRev = &r.BaseRev
	}
	if r.ResultRev != "" {
		out.ResultRev = &r.ResultRev
	}
	if r.RetryAfter != "" {
		out.RetryAfter = &r.RetryAfter
	}
	return out
}

func currentAttemptFromRecord(a *CurrentAttemptInfo) *ddxgraphql.CurrentAttemptInfo {
	out := &ddxgraphql.CurrentAttemptInfo{
		AttemptID: a.AttemptID,
		BeadID:    a.BeadID,
		Phase:     a.Phase,
		PhaseSeq:  a.PhaseSeq,
		StartedAt: a.StartedAt.UTC().Format(time.RFC3339),
		ElapsedMs: int(a.ElapsedMS),
	}
	if a.BeadTitle != "" {
		out.BeadTitle = &a.BeadTitle
	}
	if a.Harness != "" {
		out.Harness = &a.Harness
	}
	if a.Model != "" {
		out.Model = &a.Model
	}
	if a.Profile != "" {
		out.Profile = &a.Profile
	}
	return out
}

func lastAttemptFromRecord(a *LastAttemptInfo) *ddxgraphql.LastAttemptInfo {
	return &ddxgraphql.LastAttemptInfo{
		AttemptID: a.AttemptID,
		BeadID:    a.BeadID,
		Phase:     a.Phase,
		StartedAt: a.StartedAt.UTC().Format(time.RFC3339),
		EndedAt:   a.EndedAt.UTC().Format(time.RFC3339),
		ElapsedMs: int(a.ElapsedMS),
	}
}

func landSummaryFromRecord(m *CoordinatorMetrics) *ddxgraphql.CoordinatorMetrics {
	out := &ddxgraphql.CoordinatorMetrics{
		Landed:          int(m.Landed),
		Preserved:       int(m.Preserved),
		Failed:          int(m.Failed),
		PushFailed:      int(m.PushFailed),
		TotalDurationMs: int(m.TotalDurationMS),
		TotalCommits:    int(m.TotalCommits),
		PreservedRatio:  m.PreservedRatio,
	}
	for _, sub := range m.LastSubmissions {
		s := sub // copy
		entry := &ddxgraphql.LandOutcomeSummary{
			Ts:          s.TS.UTC().Format(time.RFC3339),
			Outcome:     s.Outcome,
			DurationMs:  int(s.DurationMS),
			CommitCount: int(s.CommitCount),
		}
		if s.BeadID != "" {
			entry.BeadID = &s.BeadID
		}
		if s.AttemptID != "" {
			entry.AttemptID = &s.AttemptID
		}
		out.LastSubmissions = append(out.LastSubmissions, entry)
	}
	return out
}

// ─── Coordinator queries ──────────────────────────────────────────────────────

// GetCoordinatorsGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetCoordinatorsGraphQL() []*ddxgraphql.CoordinatorMetricsEntry {
	if s.coordinatorReg == nil {
		return nil
	}
	all := s.coordinatorReg.AllMetrics()
	out := make([]*ddxgraphql.CoordinatorMetricsEntry, len(all))
	for i, e := range all {
		e := e // copy
		out[i] = &ddxgraphql.CoordinatorMetricsEntry{
			ProjectRoot: e.ProjectRoot,
			Metrics:     landSummaryFromRecord(&e.Metrics),
		}
	}
	return out
}

// GetCoordinatorMetricsByProjectGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetCoordinatorMetricsByProjectGraphQL(projectRoot string) *ddxgraphql.CoordinatorMetrics {
	if s.coordinatorReg == nil {
		return nil
	}
	m := s.coordinatorReg.MetricsFor(projectRoot)
	if m == nil {
		return nil
	}
	return landSummaryFromRecord(m)
}

// ─── AgentSession queries ─────────────────────────────────────────────────────

// GetAgentSessionsGraphQL implements ddxgraphql.StateProvider.
// Reads sessions.jsonl from every registered project's agent-logs directory.
func (s *ServerState) GetAgentSessionsGraphQL() []*ddxgraphql.AgentSession {
	projects := s.GetProjects(false)
	var out []*ddxgraphql.AgentSession
	for _, proj := range projects {
		sessions := readProjectSessions(proj)
		out = append(out, sessions...)
	}
	// Newest first.
	sort.Slice(out, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, out[i].StartedAt)
		tj, _ := time.Parse(time.RFC3339, out[j].StartedAt)
		return ti.After(tj)
	})
	return out
}

// GetAgentSessionGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetAgentSessionGraphQL(id string) (*ddxgraphql.AgentSession, bool) {
	sessions := s.GetAgentSessionsGraphQL()
	for _, sess := range sessions {
		if sess.ID == id {
			return sess, true
		}
	}
	return nil, false
}

// readProjectSessions reads sessions.jsonl for one project and maps to graphql types.
func readProjectSessions(proj ProjectEntry) []*ddxgraphql.AgentSession {
	path := filepath.Join(proj.Path, ".ddx", "agent-logs", "sessions.jsonl")
	entries, err := agent.ReadAllJSONL[agent.SessionEntry](path)
	if err != nil {
		return nil
	}
	out := make([]*ddxgraphql.AgentSession, 0, len(entries))
	for _, e := range entries {
		sess := agentSessionFromEntry(proj.ID, e)
		out = append(out, sess)
	}
	return out
}

func agentSessionFromEntry(projectID string, e agent.SessionEntry) *ddxgraphql.AgentSession {
	sess := &ddxgraphql.AgentSession{
		ID:         e.ID,
		ProjectID:  projectID,
		Harness:    e.Harness,
		Model:      e.Model,
		Effort:     e.Correlation["effort"],
		DurationMs: e.Duration,
		StartedAt:  e.Timestamp.UTC().Format(time.RFC3339),
	}

	// Derive status and outcome from exit code / error.
	if e.Error != "" {
		sess.Status = "failed"
		outcome := "failure"
		sess.Outcome = &outcome
		sess.Detail = &e.Error
	} else {
		sess.Status = "completed"
		outcome := "success"
		sess.Outcome = &outcome
	}

	// EndedAt = StartedAt + duration.
	if e.Duration > 0 {
		endedAt := e.Timestamp.Add(time.Duration(e.Duration) * time.Millisecond).UTC().Format(time.RFC3339)
		sess.EndedAt = &endedAt
	}

	if e.CostUSD > 0 {
		sess.Cost = &e.CostUSD
	}

	// Token breakdown.
	if e.InputTokens > 0 || e.OutputTokens > 0 || e.TotalTokens > 0 {
		prompt := e.InputTokens
		completion := e.OutputTokens
		total := e.TotalTokens
		if total == 0 {
			total = e.Tokens
		}
		sess.Tokens = &ddxgraphql.TokenUsage{
			Prompt:     &prompt,
			Completion: &completion,
			Total:      &total,
		}
	}

	if beadID, ok := e.Correlation["bead_id"]; ok && beadID != "" {
		sess.BeadID = &beadID
	}
	if e.NativeLogRef != "" {
		sess.StdoutPath = &e.NativeLogRef
	}
	if e.BaseRev != "" {
		sess.BaseRev = &e.BaseRev
	}
	if e.ResultRev != "" {
		sess.ResultRev = &e.ResultRev
	}
	return sess
}

// ─── Exec queries ─────────────────────────────────────────────────────────────

// GetExecDefinitionsGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetExecDefinitionsGraphQL(artifactID string) []*ddxgraphql.ExecutionDefinition {
	projects := s.GetProjects(false)
	var out []*ddxgraphql.ExecutionDefinition
	for _, proj := range projects {
		store := ddxexec.NewStore(proj.Path)
		defs, err := store.ListDefinitions(artifactID)
		if err != nil {
			continue
		}
		for _, def := range defs {
			d := def // copy
			out = append(out, execDefinitionFromRecord(d))
		}
	}
	return out
}

// GetExecDefinitionGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetExecDefinitionGraphQL(id string) (*ddxgraphql.ExecutionDefinition, bool) {
	projects := s.GetProjects(false)
	for _, proj := range projects {
		store := ddxexec.NewStore(proj.Path)
		def, err := store.ShowDefinition(id)
		if err != nil {
			continue
		}
		return execDefinitionFromRecord(def), true
	}
	return nil, false
}

// GetExecRunsGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetExecRunsGraphQL(artifactID, definitionID string) []*ddxgraphql.ExecutionRun {
	projects := s.GetProjects(false)
	var out []*ddxgraphql.ExecutionRun
	for _, proj := range projects {
		store := ddxexec.NewStore(proj.Path)
		runs, err := store.History(artifactID, definitionID)
		if err != nil {
			continue
		}
		for _, rec := range runs {
			r := rec // copy
			out = append(out, execRunFromRecord(r))
		}
	}
	// Newest first.
	sort.Slice(out, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, out[i].StartedAt)
		tj, _ := time.Parse(time.RFC3339, out[j].StartedAt)
		return ti.After(tj)
	})
	return out
}

// GetExecRunGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetExecRunGraphQL(id string) (*ddxgraphql.ExecutionRun, bool) {
	projects := s.GetProjects(false)
	for _, proj := range projects {
		store := ddxexec.NewStore(proj.Path)
		runs, err := store.History("", "")
		if err != nil {
			continue
		}
		for _, rec := range runs {
			if rec.RunID == id {
				return execRunFromRecord(rec), true
			}
		}
	}
	return nil, false
}

// GetExecRunLogGraphQL implements ddxgraphql.StateProvider.
func (s *ServerState) GetExecRunLogGraphQL(runID string) *ddxgraphql.ExecutionRunLog {
	projects := s.GetProjects(false)
	for _, proj := range projects {
		store := ddxexec.NewStore(proj.Path)
		stdout, stderr, err := store.Log(runID)
		if err != nil {
			continue
		}
		return &ddxgraphql.ExecutionRunLog{Stdout: stdout, Stderr: stderr}
	}
	return &ddxgraphql.ExecutionRunLog{}
}

func execDefinitionFromRecord(def ddxexec.Definition) *ddxgraphql.ExecutionDefinition {
	d := &ddxgraphql.ExecutionDefinition{
		ID:          def.ID,
		ArtifactIds: def.ArtifactIDs,
		Active:      def.Active,
		CreatedAt:   def.CreatedAt.UTC().Format(time.RFC3339),
	}
	d.Executor = &ddxgraphql.ExecutorSpec{
		Kind:    def.Executor.Kind,
		Command: def.Executor.Command,
	}
	if def.Executor.Cwd != "" {
		d.Executor.Cwd = &def.Executor.Cwd
	}
	if def.Required {
		b := true
		d.Required = &b
	}
	if def.GraphSource {
		b := true
		d.GraphSource = &b
	}
	if def.Result.Metric != nil {
		d.Result = &ddxgraphql.ResultSpec{}
		if def.Result.Metric.Unit != "" || def.Result.Metric.ValuePath != "" {
			d.Result.Metric = &ddxgraphql.MetricResultSpec{
				Unit:      &def.Result.Metric.Unit,
				ValuePath: &def.Result.Metric.ValuePath,
			}
		}
	}
	if def.Evaluation.Comparison != "" || def.Evaluation.Thresholds.WarnMS != 0 || def.Evaluation.Thresholds.RatchetMS != 0 {
		eval := &ddxgraphql.Evaluation{}
		if def.Evaluation.Comparison != "" {
			eval.Comparison = &def.Evaluation.Comparison
		}
		if def.Evaluation.Thresholds.WarnMS != 0 || def.Evaluation.Thresholds.RatchetMS != 0 {
			eval.Thresholds = &ddxgraphql.Thresholds{
				WarnMs:    &def.Evaluation.Thresholds.WarnMS,
				RatchetMs: &def.Evaluation.Thresholds.RatchetMS,
			}
		}
		d.Evaluation = eval
	}
	return d
}

func execRunFromRecord(rec ddxexec.RunRecord) *ddxgraphql.ExecutionRun {
	r := &ddxgraphql.ExecutionRun{
		ID:           rec.RunID,
		DefinitionID: rec.DefinitionID,
		ArtifactIds:  rec.ArtifactIDs,
		StartedAt:    rec.StartedAt.UTC().Format(time.RFC3339),
		FinishedAt:   rec.FinishedAt.UTC().Format(time.RFC3339),
		Status:       rec.Status,
		ExitCode:     rec.ExitCode,
	}
	if rec.MergeBlocking {
		b := true
		r.MergeBlocking = &b
	}
	if rec.AgentSessionID != "" {
		r.AgentSessionID = &rec.AgentSessionID
	}
	if rec.Result.Stdout != "" {
		r.Stdout = &rec.Result.Stdout
	}
	if rec.Result.Stderr != "" {
		r.Stderr = &rec.Result.Stderr
	}
	if rec.Result.Parsed {
		b := true
		r.Parsed = &b
		r.Value = &rec.Result.Value
		if rec.Result.Unit != "" {
			r.Unit = &rec.Result.Unit
		}
	}
	if rec.Result.Metric != nil {
		m := rec.Result.Metric
		r.Metric = &ddxgraphql.MetricObservation{
			ArtifactID:   m.ArtifactID,
			DefinitionID: m.DefinitionID,
			ObservedAt:   m.ObservedAt.UTC().Format(time.RFC3339),
			Status:       m.Status,
			Value:        m.Value,
		}
		if m.Unit != "" {
			r.Metric.Unit = &m.Unit
		}
		if len(m.Samples) > 0 {
			r.Metric.Samples = m.Samples
		}
	}
	if rec.Provenance.Host != "" || rec.Provenance.Actor != "" {
		r.Provenance = &ddxgraphql.Provenance{
			Host:  &rec.Provenance.Host,
			Actor: &rec.Provenance.Actor,
		}
	}
	return r
}

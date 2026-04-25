package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// executionBundleManifest captures the union of fields used by either the
// rich manifest schema (attempt_id, bead, paths, ...) and the older terse
// schema (harness, model, base_rev, result_rev, verdict, bead_id,
// execution_dir).
type executionBundleManifest struct {
	AttemptID    string `json:"attempt_id,omitempty"`
	WorkerID     string `json:"worker_id,omitempty"`
	BeadID       string `json:"bead_id,omitempty"`
	BaseRev      string `json:"base_rev,omitempty"`
	ResultRev    string `json:"result_rev,omitempty"`
	Verdict      string `json:"verdict,omitempty"`
	Harness      string `json:"harness,omitempty"`
	Model        string `json:"model,omitempty"`
	ExecutionDir string `json:"execution_dir,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	Requested    struct {
		Harness string `json:"harness,omitempty"`
		Model   string `json:"model,omitempty"`
	} `json:"requested,omitempty"`
	Bead struct {
		ID    string `json:"id,omitempty"`
		Title string `json:"title,omitempty"`
	} `json:"bead,omitempty"`
	Paths struct {
		Dir      string `json:"dir,omitempty"`
		Prompt   string `json:"prompt,omitempty"`
		Manifest string `json:"manifest,omitempty"`
		Result   string `json:"result,omitempty"`
		Usage    string `json:"usage,omitempty"`
	} `json:"paths,omitempty"`
}

// executionBundleResult captures the union of fields across result.json
// schemas observed in the wild.
type executionBundleResult struct {
	BeadID             string  `json:"bead_id,omitempty"`
	AttemptID          string  `json:"attempt_id,omitempty"`
	Verdict            string  `json:"verdict,omitempty"`
	Outcome            string  `json:"outcome,omitempty"`
	Status             string  `json:"status,omitempty"`
	Detail             string  `json:"detail,omitempty"`
	Rationale          string  `json:"rationale,omitempty"`
	NoChangesRationale string  `json:"no_changes_rationale,omitempty"`
	Harness            string  `json:"harness,omitempty"`
	Model              string  `json:"model,omitempty"`
	SessionID          string  `json:"session_id,omitempty"`
	WorkerID           string  `json:"worker_id,omitempty"`
	DurationMS         int     `json:"duration_ms,omitempty"`
	Tokens             int     `json:"tokens,omitempty"`
	CostUSD            float64 `json:"cost_usd,omitempty"`
	ExitCode           int     `json:"exit_code"`
	BaseRev            string  `json:"base_rev,omitempty"`
	ResultRev          string  `json:"result_rev,omitempty"`
	StartedAt          string  `json:"started_at,omitempty"`
	FinishedAt         string  `json:"finished_at,omitempty"`
	AgentLogPath       string  `json:"agent_log_path,omitempty"`
}

// loadExecutionBundle reads and merges manifest.json and result.json from a
// bundle directory. The two are complementary — manifest has the request,
// result has the outcome — so the merged record is the canonical one.
func loadExecutionBundle(projectID, projectRoot, bundleDirAbs, bundleID string) *ddxgraphql.Execution {
	manifest := executionBundleManifest{}
	if data, err := os.ReadFile(filepath.Join(bundleDirAbs, "manifest.json")); err == nil {
		_ = json.Unmarshal(data, &manifest)
	}
	result := executionBundleResult{}
	if data, err := os.ReadFile(filepath.Join(bundleDirAbs, "result.json")); err == nil {
		_ = json.Unmarshal(data, &result)
	}
	bundleRel := filepath.ToSlash(filepath.Join(agent.ExecuteBeadArtifactDir, bundleID))

	exec := &ddxgraphql.Execution{
		ID:         bundleID,
		ProjectID:  projectID,
		BundlePath: bundleRel,
	}
	// Created-at: prefer manifest.created_at; fall back to bundle directory
	// timestamp prefix (e.g. 20260423T053812).
	if manifest.CreatedAt != "" {
		exec.CreatedAt = normalizeISOTime(manifest.CreatedAt)
	} else if t, ok := parseBundleTimestamp(bundleID); ok {
		exec.CreatedAt = t.UTC().Format(time.RFC3339)
	} else if info, err := os.Stat(bundleDirAbs); err == nil {
		exec.CreatedAt = info.ModTime().UTC().Format(time.RFC3339)
	}

	// Bead info — prefer richer manifest, then either source's bead_id.
	beadID := firstNonEmptyStr(manifest.BeadID, manifest.Bead.ID, result.BeadID)
	if beadID != "" {
		exec.BeadID = &beadID
	}
	if manifest.Bead.Title != "" {
		title := manifest.Bead.Title
		exec.BeadTitle = &title
	}

	// Harness / model — manifest.requested takes precedence (user intent),
	// else result, else manifest top-level (terse schema).
	harness := firstNonEmptyStr(manifest.Requested.Harness, result.Harness, manifest.Harness)
	if harness != "" {
		exec.Harness = &harness
	}
	model := firstNonEmptyStr(manifest.Requested.Model, result.Model, manifest.Model)
	if model != "" {
		exec.Model = &model
	}

	// Verdict — manifest.verdict (terse schema) wins, else result.verdict,
	// else result.outcome / status.
	verdict := firstNonEmptyStr(manifest.Verdict, result.Verdict, result.Outcome, result.Status)
	if verdict != "" {
		exec.Verdict = &verdict
	}
	if result.Status != "" {
		s := result.Status
		exec.Status = &s
	}

	// Rationale — prefer the explicit rationale field, else no_changes
	// rationale, else generic detail.
	rationale := firstNonEmptyStr(result.Rationale, result.NoChangesRationale, result.Detail)
	if rationale != "" {
		exec.Rationale = &rationale
	}

	if result.SessionID != "" {
		s := result.SessionID
		exec.SessionID = &s
	}
	workerID := firstNonEmptyStr(manifest.WorkerID, result.WorkerID)
	if workerID != "" {
		exec.WorkerID = &workerID
	}
	if result.StartedAt != "" {
		s := normalizeISOTime(result.StartedAt)
		exec.StartedAt = &s
	}
	if result.FinishedAt != "" {
		s := normalizeISOTime(result.FinishedAt)
		exec.FinishedAt = &s
	}
	if result.DurationMS > 0 {
		d := result.DurationMS
		exec.DurationMs = &d
	}
	if result.CostUSD != 0 {
		c := result.CostUSD
		exec.CostUsd = &c
	}
	if result.Tokens > 0 {
		t := result.Tokens
		exec.Tokens = &t
	}
	ec := result.ExitCode
	exec.ExitCode = &ec
	baseRev := firstNonEmptyStr(manifest.BaseRev, result.BaseRev)
	if baseRev != "" {
		exec.BaseRev = &baseRev
	}
	resultRev := firstNonEmptyStr(manifest.ResultRev, result.ResultRev)
	if resultRev != "" {
		exec.ResultRev = &resultRev
	}

	// Path pointers — prefer manifest.paths if populated, else default
	// names within the bundle dir.
	promptRel := firstNonEmptyStr(manifest.Paths.Prompt, filepath.ToSlash(filepath.Join(bundleRel, "prompt.md")))
	exec.PromptPath = &promptRel
	manifestRel := firstNonEmptyStr(manifest.Paths.Manifest, filepath.ToSlash(filepath.Join(bundleRel, "manifest.json")))
	exec.ManifestPath = &manifestRel
	resultRel := firstNonEmptyStr(manifest.Paths.Result, filepath.ToSlash(filepath.Join(bundleRel, "result.json")))
	exec.ResultPath = &resultRel

	// Agent-log path — prefer explicit pointer, else the conventional
	// per-session file under .ddx/agent-logs/.
	if result.AgentLogPath != "" {
		s := filepath.ToSlash(result.AgentLogPath)
		exec.AgentLogPath = &s
	} else if result.SessionID != "" {
		s := filepath.ToSlash(filepath.Join(".ddx/agent-logs", "agent-"+result.SessionID+".jsonl"))
		exec.AgentLogPath = &s
	}
	return exec
}

// loadExecutionBundleDetail returns the same Execution but with prompt /
// manifest / result string bodies attached for execution(id:) lookups.
func loadExecutionBundleDetail(projectID, projectRoot, bundleDirAbs, bundleID string) *ddxgraphql.Execution {
	exec := loadExecutionBundle(projectID, projectRoot, bundleDirAbs, bundleID)
	if exec == nil {
		return nil
	}
	if data, err := os.ReadFile(filepath.Join(bundleDirAbs, "prompt.md")); err == nil {
		clamped, _, _ := evidence.ClampOutput(string(data), serverInlineCapBytes)
		exec.Prompt = &clamped
	}
	if data, err := os.ReadFile(filepath.Join(bundleDirAbs, "manifest.json")); err == nil {
		clamped, _, _ := evidence.ClampOutput(string(data), serverInlineCapBytes)
		exec.Manifest = &clamped
	}
	if data, err := os.ReadFile(filepath.Join(bundleDirAbs, "result.json")); err == nil {
		clamped, _, _ := evidence.ClampOutput(string(data), serverInlineCapBytes)
		exec.Result = &clamped
	}
	return exec
}

// scanExecutionBundles enumerates `.ddx/executions/` for one project, returns
// merged Execution records sorted newest-first.
func scanExecutionBundles(projectID, projectRoot string) []*ddxgraphql.Execution {
	dir := filepath.Join(projectRoot, agent.ExecuteBeadArtifactDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]*ddxgraphql.Execution, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !looksLikeBundleID(name) {
			continue
		}
		bundleDirAbs := filepath.Join(dir, name)
		if exec := loadExecutionBundle(projectID, projectRoot, bundleDirAbs, name); exec != nil {
			out = append(out, exec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out
}

// looksLikeBundleID checks for the YYYYMMDDTHHMMSS-<hex> shape used by
// execute-bead, plus a permissive fallback that accepts any non-special name.
// It rejects the special "mirror-index.jsonl" file and other reserved names.
func looksLikeBundleID(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	if strings.Contains(name, ".jsonl") || strings.Contains(name, ".json") {
		return false
	}
	return true
}

func parseBundleTimestamp(name string) (time.Time, bool) {
	idx := strings.IndexByte(name, '-')
	if idx <= 0 {
		return time.Time{}, false
	}
	stamp := name[:idx]
	if t, err := time.Parse("20060102T150405", stamp); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func normalizeISOTime(s string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return s
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// GetExecutionsGraphQL implements the optional ExecutionsStateProvider.
func (s *ServerState) GetExecutionsGraphQL(projectID string, filter ddxgraphql.ExecutionFilter) []*ddxgraphql.Execution {
	if projectID == "" {
		return nil
	}
	proj, ok := s.GetProjectByID(projectID)
	if !ok {
		return nil
	}
	all := scanExecutionBundles(projectID, proj.Path)
	return applyExecutionFilter(all, filter)
}

func applyExecutionFilter(in []*ddxgraphql.Execution, filter ddxgraphql.ExecutionFilter) []*ddxgraphql.Execution {
	if len(in) == 0 {
		return in
	}
	if filter.BeadID == "" && filter.Verdict == "" && filter.Harness == "" && filter.Search == "" && filter.Since == nil && filter.Until == nil {
		return in
	}
	q := strings.ToLower(filter.Search)
	out := in[:0:0]
	for _, e := range in {
		if filter.BeadID != "" {
			if e.BeadID == nil || *e.BeadID != filter.BeadID {
				continue
			}
		}
		if filter.Verdict != "" {
			if e.Verdict == nil || !strings.EqualFold(*e.Verdict, filter.Verdict) {
				continue
			}
		}
		if filter.Harness != "" {
			if e.Harness == nil || *e.Harness != filter.Harness {
				continue
			}
		}
		if filter.Since != nil {
			if t, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
				if t.Before(*filter.Since) {
					continue
				}
			}
		}
		if filter.Until != nil {
			if t, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
				if !t.Before(*filter.Until) {
					continue
				}
			}
		}
		if q != "" {
			matched := false
			if e.BeadTitle != nil && strings.Contains(strings.ToLower(*e.BeadTitle), q) {
				matched = true
			}
			if !matched && e.BeadID != nil && strings.Contains(strings.ToLower(*e.BeadID), q) {
				matched = true
			}
			if !matched {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// GetExecutionGraphQL implements ExecutionsStateProvider.
func (s *ServerState) GetExecutionGraphQL(id string) (*ddxgraphql.Execution, bool) {
	if id == "" {
		return nil, false
	}
	for _, proj := range s.GetProjects(false) {
		bundleDirAbs := filepath.Join(proj.Path, agent.ExecuteBeadArtifactDir, id)
		if info, err := os.Stat(bundleDirAbs); err != nil || !info.IsDir() {
			continue
		}
		exec := loadExecutionBundleDetail(proj.ID, proj.Path, bundleDirAbs, id)
		if exec != nil {
			return exec, true
		}
	}
	return nil, false
}

// agentLogFrame captures the union of agent-log frame schemas. Different
// harnesses emit slightly different shapes; this struct picks up the common
// ones used by the workers detail page.
type agentLogFrame struct {
	Kind   string          `json:"kind,omitempty"`
	Type   string          `json:"type,omitempty"`
	Name   string          `json:"name,omitempty"`
	Tool   string          `json:"tool,omitempty"`
	TS     string          `json:"ts,omitempty"`
	Time   string          `json:"time,omitempty"`
	Inputs json.RawMessage `json:"inputs,omitempty"`
	Input  json.RawMessage `json:"input,omitempty"`
	Output json.RawMessage `json:"output,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// GetExecutionToolCallsGraphQL implements ExecutionsStateProvider.
//
// Reads the agent-log file for the given execution and returns one
// ExecutionToolCall per recognized tool_call/tool_use frame.
func (s *ServerState) GetExecutionToolCallsGraphQL(id string) []*ddxgraphql.ExecutionToolCall {
	if id == "" {
		return nil
	}
	for _, proj := range s.GetProjects(false) {
		bundleDirAbs := filepath.Join(proj.Path, agent.ExecuteBeadArtifactDir, id)
		if info, err := os.Stat(bundleDirAbs); err != nil || !info.IsDir() {
			continue
		}
		// Find the agent log path: prefer manifest pointer, else session id
		// convention, else look for embedded/agent-*.jsonl in the bundle.
		exec := loadExecutionBundle(proj.ID, proj.Path, bundleDirAbs, id)
		if exec == nil {
			return nil
		}
		var paths []string
		if exec.AgentLogPath != nil {
			abs := *exec.AgentLogPath
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(proj.Path, abs)
			}
			paths = append(paths, abs)
		}
		// Also scan the bundle's embedded/ directory for agent-*.jsonl.
		embedded := filepath.Join(bundleDirAbs, "embedded")
		if entries, err := os.ReadDir(embedded); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				n := e.Name()
				if strings.HasPrefix(n, "agent-") && strings.HasSuffix(n, ".jsonl") {
					paths = append(paths, filepath.Join(embedded, n))
				}
			}
		}
		for _, p := range paths {
			if calls := readToolCallsFile(p); len(calls) > 0 {
				return calls
			}
		}
		return nil
	}
	return nil
}

func readToolCallsFile(path string) []*ddxgraphql.ExecutionToolCall {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var calls []*ddxgraphql.ExecutionToolCall
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	seq := 0
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var frame agentLogFrame
		if err := json.Unmarshal(line, &frame); err != nil {
			continue
		}
		kind := frame.Kind
		if kind == "" {
			kind = frame.Type
		}
		if !isToolCallKind(kind) {
			continue
		}
		name := frame.Name
		if name == "" {
			name = frame.Tool
		}
		if name == "" {
			continue
		}
		ts := frame.TS
		if ts == "" {
			ts = frame.Time
		}
		input := string(firstNonEmptyRaw(frame.Inputs, frame.Input))
		output := string(firstNonEmptyRaw(frame.Output, frame.Result))
		const maxOut = 64 * 1024
		truncated := false
		if len(output) > maxOut {
			output = output[:maxOut]
			truncated = true
		}
		call := &ddxgraphql.ExecutionToolCall{
			ID:   formatToolCallID(seq),
			Name: name,
			Seq:  seq,
		}
		if ts != "" {
			s := normalizeISOTime(ts)
			call.Ts = &s
		}
		if input != "" {
			call.Inputs = &input
		}
		if output != "" {
			call.Output = &output
		}
		if truncated {
			t := true
			call.Truncated = &t
		}
		calls = append(calls, call)
		seq++
	}
	return calls
}

func isToolCallKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "tool_call", "tool_use":
		return true
	}
	return false
}

func firstNonEmptyRaw(values ...json.RawMessage) json.RawMessage {
	for _, v := range values {
		if len(v) > 0 && string(v) != "null" {
			return v
		}
	}
	return nil
}

func formatToolCallID(seq int) string {
	return "tc-" + strconv.Itoa(seq)
}

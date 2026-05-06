package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/DocumentDrivenDX/fizeau"
)

const (
	postAttemptTriagePromptSource = "bead-lifecycle-triage"
	postAttemptTriageSkillName    = "bead-lifecycle"
	maxTriageEventBodyBytes       = 4096
	maxTriageEventCount           = 8
	defaultSessionLogExcerptBytes = 4096
)

// SessionLogExcerptReader reads a bounded excerpt from a session log path.
// The default implementation searches the project agent log dir for a
// matching agent-*.jsonl or svc-*.jsonl file.
type SessionLogExcerptReader interface {
	ReadSessionLogExcerpt(ctx context.Context, projectRoot, sessionID string, maxBytes int) (string, error)
}

type fileSessionLogExcerptReader struct{}

func (fileSessionLogExcerptReader) ReadSessionLogExcerpt(ctx context.Context, projectRoot, sessionID string, maxBytes int) (string, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	path, ok := resolveSessionLogPath(projectRoot, sessionID)
	if !ok {
		return "", os.ErrNotExist
	}
	return readBoundedFileTail(path, maxBytes)
}

type triagePromptBead struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Status       string            `json:"status,omitempty"`
	Priority     int               `json:"priority,omitempty"`
	IssueType    string            `json:"issue_type,omitempty"`
	Labels       []string          `json:"labels,omitempty"`
	Parent       string            `json:"parent,omitempty"`
	Description  string            `json:"description,omitempty"`
	Acceptance   string            `json:"acceptance,omitempty"`
	Notes        string            `json:"notes,omitempty"`
	Dependencies []bead.Dependency `json:"dependencies,omitempty"`
	Extra        map[string]any    `json:"extra,omitempty"`
}

type triagePromptEnvelope struct {
	Bead           triagePromptBead  `json:"bead"`
	ExecuteBead    ExecuteBeadReport `json:"execute_bead_report"`
	RecentEvents   []bead.BeadEvent  `json:"recent_bead_events,omitempty"`
	SessionExcerpt string            `json:"session_log_excerpt,omitempty"`
	ProjectRoot    string            `json:"project_root,omitempty"`
	SessionLogPath string            `json:"session_log_path,omitempty"`
}

// NewPostAttemptTriageHook constructs the bead-lifecycle post-attempt triage
// hook. The hook is runner-backed, uses the resolved config / runner dispatch
// seam, and never shells out through os/exec.
func NewPostAttemptTriageHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner, sessionLogReader SessionLogExcerptReader) func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
	reader := sessionLogReader
	if reader == nil {
		reader = fileSessionLogExcerptReader{}
	}
	return func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return TriageResult{}, err
			}
		}
		if store == nil {
			return TriageResult{}, fmt.Errorf("triage hook: bead reader required")
		}
		if !hasBeadLifecycleSkill(projectRoot) {
			return TriageResult{}, fmt.Errorf("triage hook: skill missing: bead-lifecycle")
		}

		b, err := store.Get(beadID)
		if err != nil {
			return TriageResult{}, fmt.Errorf("triage hook: load bead %s: %w", beadID, err)
		}

		var eventLister interface {
			Events(id string) ([]bead.BeadEvent, error)
		}
		if l, ok := any(store).(interface {
			Events(id string) ([]bead.BeadEvent, error)
		}); ok {
			eventLister = l
		}

		prompt, err := buildPostAttemptTriagePrompt(ctx, projectRoot, b, report, reader, eventLister)
		if err != nil {
			return TriageResult{}, err
		}

		result, err := dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, AgentRunRuntime{
			Prompt:       prompt,
			WorkDir:      projectRoot,
			PromptSource: postAttemptTriagePromptSource,
		})
		if err != nil {
			return TriageResult{}, fmt.Errorf("triage hook: dispatch: %w", err)
		}

		payload, err := triageResultPayload(result)
		if err != nil {
			return TriageResult{}, err
		}

		var out TriageResult
		if err := json.Unmarshal([]byte(payload), &out); err != nil {
			return TriageResult{}, fmt.Errorf("triage hook: decode result: %w", err)
		}
		out.Classification = strings.TrimSpace(out.Classification)
		out.RecommendedAction = strings.TrimSpace(out.RecommendedAction)
		out.Rationale = strings.TrimSpace(out.Rationale)
		out.SuggestedAmendments = strings.TrimSpace(out.SuggestedAmendments)
		if out.Classification == "" {
			return TriageResult{}, fmt.Errorf("triage hook: missing classification")
		}
		return out, nil
	}
}

func buildPostAttemptTriagePrompt(ctx context.Context, projectRoot string, b *bead.Bead, report ExecuteBeadReport, reader SessionLogExcerptReader, eventLister interface {
	Events(id string) ([]bead.BeadEvent, error)
}) (string, error) {
	env := triagePromptEnvelope{
		Bead: triagePromptBead{
			ID:           b.ID,
			Title:        strings.TrimSpace(b.Title),
			Status:       strings.TrimSpace(b.Status),
			Priority:     b.Priority,
			IssueType:    strings.TrimSpace(b.IssueType),
			Labels:       append([]string(nil), b.Labels...),
			Parent:       strings.TrimSpace(b.Parent),
			Description:  strings.TrimSpace(b.Description),
			Acceptance:   strings.TrimSpace(b.Acceptance),
			Notes:        strings.TrimSpace(b.Notes),
			Dependencies: append([]bead.Dependency(nil), b.Dependencies...),
		},
		ExecuteBead: report,
		ProjectRoot: projectRoot,
	}
	if len(b.Extra) > 0 {
		env.Bead.Extra = make(map[string]any, len(b.Extra))
		for k, v := range b.Extra {
			if k == "events" || k == "events_attachment" {
				continue
			}
			env.Bead.Extra[k] = v
		}
	}

	if eventLister != nil {
		events, err := eventLister.Events(b.ID)
		if err == nil && len(events) > 0 {
			start := len(events) - maxTriageEventCount
			if start < 0 {
				start = 0
			}
			env.RecentEvents = make([]bead.BeadEvent, 0, len(events)-start)
			for _, ev := range events[start:] {
				env.RecentEvents = append(env.RecentEvents, clampTriageEvent(ev))
			}
		}
	}

	if report.SessionID != "" && reader != nil {
		excerpt, err := reader.ReadSessionLogExcerpt(ctx, projectRoot, report.SessionID, defaultSessionLogExcerptBytes)
		if err == nil && strings.TrimSpace(excerpt) != "" {
			env.SessionExcerpt = excerpt
			if path, ok := resolveSessionLogPath(projectRoot, report.SessionID); ok {
				env.SessionLogPath = path
			}
		}
	}

	body, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("MODE: triage\n")
	sb.WriteString("You are the bead-lifecycle skill (" + postAttemptTriageSkillName + "). Classify the outcome of the attempt and return exactly one JSON object.\n")
	sb.WriteString("Return structured JSON with the fields classification, recommended_action, rationale, suggested_amendments, suggested_followup_beads.\n")
	sb.WriteString("Required output shape example: {\"classification\":\"transport\",\"recommended_action\":\"retry\",\"rationale\":\"transient\",\"suggested_amendments\":\"none\",\"suggested_followup_beads\":[]}\n")
	sb.WriteString("Use stable machine-readable classifications such as transport, quota, routing, timeout, merge_conflict, tests_red, needs_decomposition, needs_human, or success.\n")
	sb.WriteString("Do not wrap the answer in markdown or prose.\n\n")
	sb.WriteString("```json\n")
	sb.Write(body)
	sb.WriteString("\n```\n")
	return sb.String(), nil
}

func triageResultPayload(result *Result) (string, error) {
	if result == nil {
		return "", fmt.Errorf("triage hook: empty runner result")
	}
	text := strings.TrimSpace(result.CondensedOutput)
	if text == "" {
		text = strings.TrimSpace(result.Output)
	}
	if text == "" {
		return "", fmt.Errorf("triage hook: empty output")
	}
	candidate, ok := extractJSONCandidate(text)
	if !ok {
		return "", fmt.Errorf("triage hook: no JSON object found")
	}
	return candidate, nil
}

func recordPostAttemptTriageEvent(store BeadEventAppender, beadID string, report ExecuteBeadReport, triage TriageResult, actor string, createdAt time.Time) {
	if store == nil || beadID == "" || strings.TrimSpace(triage.Classification) == "" {
		return
	}
	body := map[string]any{
		"classification":           triage.Classification,
		"recommended_action":       triage.RecommendedAction,
		"rationale":                triage.Rationale,
		"suggested_amendments":     triage.SuggestedAmendments,
		"suggested_followup_beads": triage.SuggestedFollowupBeads,
		"status":                   report.Status,
		"detail":                   report.Detail,
		"base_rev":                 report.BaseRev,
		"result_rev":               report.ResultRev,
		"session_id":               report.SessionID,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return
	}
	summary := triage.Classification
	if triage.RecommendedAction != "" {
		summary = triage.Classification + ": " + triage.RecommendedAction
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "bead-quality.triage",
		Summary:   summary,
		Body:      string(encoded),
		Actor:     actor,
		Source:    "ddx agent execute-loop",
		CreatedAt: createdAt.UTC(),
	})
}

func clampTriageEvent(ev bead.BeadEvent) bead.BeadEvent {
	ev.Summary = truncateBytes(ev.Summary, maxTriageEventBodyBytes)
	ev.Body = truncateBytes(ev.Body, maxTriageEventBodyBytes)
	return ev
}

func truncateBytes(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	const markerFmt = "\n…[truncated, %d bytes]\n"
	marker := fmt.Sprintf(markerFmt, len(s))
	if len(marker) >= limit {
		return s[:limit]
	}
	head := (limit - len(marker)) / 2
	tail := limit - len(marker) - head
	return s[:head] + marker + s[len(s)-tail:]
}

func resolveSessionLogPath(projectRoot, sessionID string) (string, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", false
	}
	candidates := []string{sessionID}
	base := filepath.Base(sessionID)
	logDir := ResolveLogDir(projectRoot, "")
	if base != "" {
		candidates = append(candidates,
			filepath.Join(logDir, base),
			filepath.Join(logDir, "agent-"+base),
			filepath.Join(logDir, "svc-"+base),
		)
		if filepath.Ext(base) == "" {
			candidates = append(candidates,
				filepath.Join(logDir, "agent-"+base+".jsonl"),
				filepath.Join(logDir, "svc-"+base+".jsonl"),
				filepath.Join(logDir, base+".jsonl"),
			)
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func readBoundedFileTail(path string, limitBytes int) (string, error) {
	if limitBytes <= 0 {
		return "", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := info.Size()
	if size <= 0 {
		return "", nil
	}
	seek := int64(0)
	if size > int64(limitBytes) {
		seek = size - int64(limitBytes)
	}
	if _, err := f.Seek(seek, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	if seek > 0 {
		return fmt.Sprintf("…[tail clipped to %d bytes]\n%s", limitBytes, string(data)), nil
	}
	return string(bytes.TrimSpace(data)), nil
}

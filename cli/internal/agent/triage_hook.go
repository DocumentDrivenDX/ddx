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
	agentlib "github.com/easel/fizeau"
)

const (
	postAttemptTriagePromptSource = "bead-lifecycle-triage"
	postAttemptTriageSkillName    = "bead-lifecycle"
	maxTriageEventBodyBytes       = 4096
	maxTriageEventCount           = 8
	maxTriageWarningExcerptBytes  = 512
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
		if err := ensureBeadLifecycleSkill(projectRoot); err != nil {
			return TriageResult{}, fmt.Errorf("triage hook: %w", err)
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
			Prompt:           prompt,
			WorkDir:          projectRoot,
			PromptSource:     postAttemptTriagePromptSource,
			ProfileOverride:  selectProfileForDispatch(ctx, projectRoot, svc, runner, SelectCheapestProfile),
			ClearRoutingPins: true,
			ClearProfile:     true,
			ClearMinPower:    true,
			ClearMaxPower:    true,
		})
		if err != nil {
			return TriageResult{}, fmt.Errorf("triage hook: dispatch: %w", err)
		}

		text, err := triageResultOutput(result)
		if err != nil {
			return TriageResult{}, err
		}

		payload, ok := extractJSONCandidate(text)
		if !ok {
			return malformedTriageResult("output", "no JSON object found", text), nil
		}

		out := decodeTriageResult(payload)
		if len(out.DecodeWarnings) > 0 {
			out.Malformed = true
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
	sb.WriteString("Required output shape example: {\"classification\":\"transport\",\"recommended_action\":\"release_claim_retry\",\"rationale\":\"transient provider failure\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}\n")
	sb.WriteString("Array fields must be arrays; use [] when none are needed.\n")
	sb.WriteString("Use only the documented classifications and recommended_action values from the skill contract.\n")
	sb.WriteString("Do not wrap the answer in markdown or prose.\n\n")
	sb.WriteString("```json\n")
	sb.Write(body)
	sb.WriteString("\n```\n")
	return sb.String(), nil
}

func triageResultOutput(result *Result) (string, error) {
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
	return text, nil
}

func recordPostAttemptTriageEvent(store BeadEventAppender, beadID string, report ExecuteBeadReport, triage TriageResult, actor string, createdAt time.Time) {
	if store == nil || beadID == "" {
		return
	}
	triage = normalizeTriageResult(triage)
	hasClassification := triage.Classification != ""
	hasWarnings := len(triage.DecodeWarnings) > 0 || triage.Malformed
	if !hasClassification && !hasWarnings {
		return
	}
	amendments := triage.SuggestedAmendments
	if amendments == nil {
		amendments = []TriageAmendment{}
	}
	followups := triage.SuggestedFollowupBeads
	if followups == nil {
		followups = []FollowupBead{}
	}
	body := map[string]any{
		"classification":           triage.Classification,
		"recommended_action":       triage.RecommendedAction,
		"rationale":                triage.Rationale,
		"suggested_amendments":     amendments,
		"suggested_followup_beads": followups,
		"status":                   report.Status,
		"detail":                   report.Detail,
		"base_rev":                 report.BaseRev,
		"result_rev":               report.ResultRev,
		"session_id":               report.SessionID,
	}
	if len(triage.DecodeWarnings) > 0 {
		body["decode_warnings"] = triage.DecodeWarnings
	}
	if triage.Malformed {
		body["malformed"] = true
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return
	}
	kind := "bead-quality.triage"
	summary := triage.Classification
	if !isRecognizedTriageClassification(triage.Classification) {
		kind = "bead-quality.triage-warning"
		summary = "malformed triage output"
		if triage.Classification != "" {
			summary = "unusable triage classification: " + triage.Classification
		} else if len(triage.DecodeWarnings) > 0 && triage.DecodeWarnings[0].Warning != "" {
			summary = triage.DecodeWarnings[0].Warning
		}
	} else if triage.RecommendedAction != "" {
		summary = triage.Classification + ": " + triage.RecommendedAction
		if len(triage.DecodeWarnings) > 0 {
			summary += fmt.Sprintf(" (warnings=%d)", len(triage.DecodeWarnings))
		}
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      kind,
		Summary:   summary,
		Body:      string(encoded),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: createdAt.UTC(),
	})
}

type triageResultRaw struct {
	Classification         string          `json:"classification,omitempty"`
	RecommendedAction      string          `json:"recommended_action,omitempty"`
	Rationale              string          `json:"rationale,omitempty"`
	SuggestedAmendments    json.RawMessage `json:"suggested_amendments,omitempty"`
	SuggestedFollowupBeads json.RawMessage `json:"suggested_followup_beads,omitempty"`
}

func decodeTriageResult(payload string) TriageResult {
	var raw triageResultRaw
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return malformedTriageResult("json", "invalid JSON object", payload)
	}

	out := TriageResult{
		Classification:    strings.TrimSpace(raw.Classification),
		RecommendedAction: strings.TrimSpace(raw.RecommendedAction),
		Rationale:         strings.TrimSpace(raw.Rationale),
	}
	out.SuggestedAmendments = []TriageAmendment{}
	out.SuggestedFollowupBeads = []FollowupBead{}

	amendments, warnings := decodeTriageAmendments(raw.SuggestedAmendments)
	out.SuggestedAmendments = amendments
	out.DecodeWarnings = append(out.DecodeWarnings, warnings...)

	followups, warnings := decodeTriageFollowupBeads(raw.SuggestedFollowupBeads)
	out.SuggestedFollowupBeads = followups
	out.DecodeWarnings = append(out.DecodeWarnings, warnings...)

	out = normalizeTriageResult(out)
	out = addTriageRequiredFieldWarnings(out)
	return out
}

func normalizeTriageResult(in TriageResult) TriageResult {
	in.Classification = strings.TrimSpace(in.Classification)
	in.RecommendedAction = strings.TrimSpace(in.RecommendedAction)
	in.Rationale = strings.TrimSpace(in.Rationale)
	if in.SuggestedAmendments == nil {
		in.SuggestedAmendments = []TriageAmendment{}
	}
	if in.SuggestedFollowupBeads == nil {
		in.SuggestedFollowupBeads = []FollowupBead{}
	}
	if in.Classification != "" && !isRecognizedTriageClassification(in.Classification) {
		in.DecodeWarnings = appendTriageDecodeWarning(in.DecodeWarnings, newTriageDecodeWarning("classification", "unknown classification", in.Classification))
	}
	if in.RecommendedAction != "" && !isRecognizedTriageRecommendedAction(in.RecommendedAction) {
		in.DecodeWarnings = appendTriageDecodeWarning(in.DecodeWarnings, newTriageDecodeWarning("recommended_action", "unknown recommended_action", in.RecommendedAction))
	}
	if len(in.DecodeWarnings) > 0 {
		in.Malformed = true
	}
	return in
}

func addTriageRequiredFieldWarnings(in TriageResult) TriageResult {
	if strings.TrimSpace(in.Classification) == "" {
		in.DecodeWarnings = appendTriageDecodeWarning(in.DecodeWarnings, newTriageDecodeWarning("classification", "missing classification", ""))
	}
	if strings.TrimSpace(in.RecommendedAction) == "" {
		in.DecodeWarnings = appendTriageDecodeWarning(in.DecodeWarnings, newTriageDecodeWarning("recommended_action", "missing recommended_action", ""))
	}
	if len(in.DecodeWarnings) > 0 {
		in.Malformed = true
	}
	return in
}

func decodeTriageAmendments(raw json.RawMessage) ([]TriageAmendment, []TriageDecodeWarning) {
	if isEmptyTriageRaw(raw) {
		return []TriageAmendment{}, nil
	}
	if s, ok := decodeTriageString(raw); ok {
		if isTriageEmptySentinel(s) {
			return []TriageAmendment{}, nil
		}
		return []TriageAmendment{}, []TriageDecodeWarning{newTriageDecodeWarning("suggested_amendments", "expected array; got string; dropped field", s)}
	}
	var out []TriageAmendment
	if err := json.Unmarshal(raw, &out); err != nil {
		return []TriageAmendment{}, []TriageDecodeWarning{newTriageDecodeWarning("suggested_amendments", "expected array; dropped malformed field", string(raw))}
	}
	if out == nil {
		out = []TriageAmendment{}
	}
	return out, nil
}

func decodeTriageFollowupBeads(raw json.RawMessage) ([]FollowupBead, []TriageDecodeWarning) {
	if isEmptyTriageRaw(raw) {
		return []FollowupBead{}, nil
	}
	if s, ok := decodeTriageString(raw); ok {
		if isTriageEmptySentinel(s) {
			return []FollowupBead{}, nil
		}
		return []FollowupBead{}, []TriageDecodeWarning{newTriageDecodeWarning("suggested_followup_beads", "expected array; got string; dropped field", s)}
	}
	var out []FollowupBead
	if err := json.Unmarshal(raw, &out); err != nil {
		return []FollowupBead{}, []TriageDecodeWarning{newTriageDecodeWarning("suggested_followup_beads", "expected array; dropped malformed field", string(raw))}
	}
	if out == nil {
		out = []FollowupBead{}
	}
	return out, nil
}

func malformedTriageResult(field, warning, raw string) TriageResult {
	return TriageResult{
		SuggestedAmendments:    []TriageAmendment{},
		SuggestedFollowupBeads: []FollowupBead{},
		DecodeWarnings:         []TriageDecodeWarning{newTriageDecodeWarning(field, warning, raw)},
		Malformed:              true,
	}
}

func newTriageDecodeWarning(field, warning, raw string) TriageDecodeWarning {
	return TriageDecodeWarning{
		Field:      field,
		Warning:    warning,
		RawExcerpt: triageWarningExcerpt(raw),
	}
}

func appendTriageDecodeWarning(warnings []TriageDecodeWarning, next TriageDecodeWarning) []TriageDecodeWarning {
	for _, existing := range warnings {
		if existing.Field == next.Field && existing.Warning == next.Warning && existing.RawExcerpt == next.RawExcerpt {
			return warnings
		}
	}
	return append(warnings, next)
}

func triageWarningExcerpt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Join(strings.Fields(raw), " ")
	if len(raw) <= maxTriageWarningExcerptBytes {
		return raw
	}
	const marker = "...[truncated]"
	limit := maxTriageWarningExcerptBytes - len(marker)
	if limit <= 0 {
		return raw[:maxTriageWarningExcerptBytes]
	}
	return raw[:limit] + marker
}

func formatTriageWarnings(warnings []TriageDecodeWarning) string {
	if len(warnings) == 0 {
		return "malformed triage output"
	}
	first := warnings[0]
	if first.Field != "" && first.Warning != "" {
		return first.Field + ": " + first.Warning
	}
	if first.Warning != "" {
		return first.Warning
	}
	return "malformed triage output"
}

func isEmptyTriageRaw(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}

func decodeTriageString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func isTriageEmptySentinel(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none", "n/a", "na", "null":
		return true
	default:
		return false
	}
}

func isRecognizedTriageClassification(classification string) bool {
	switch strings.TrimSpace(classification) {
	case "already_satisfied",
		"no_changes_unverified",
		"no_changes_unjustified",
		"needs_investigation",
		"decomposed",
		"blocked",
		"superseded",
		"routing",
		"quota",
		"transport",
		"tests_red",
		"merge_conflict",
		"review_block",
		"timeout",
		"recoverable":
		return true
	default:
		return false
	}
}

func isRecognizedTriageRecommendedAction(action string) bool {
	switch strings.TrimSpace(action) {
	case "close_already_satisfied",
		"release_claim_retry",
		"release_claim_needs_investigation",
		"release_claim_mark_blocked",
		"release_claim_mark_superseded",
		"release_claim_wait_retry",
		"close_decomposed_or_mark_execution_ineligible":
		return true
	default:
		return false
	}
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

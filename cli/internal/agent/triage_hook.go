package agent

import (
	"context"
	"encoding/json"
	"fmt"
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
)

// SessionLogExcerptReader is retained in the hook constructor interface for
// compatibility. Control triage deliberately does not read raw session logs:
// they can contain concrete Fizeau route identity that must remain audit-only.
type SessionLogExcerptReader interface {
	ReadSessionLogExcerpt(ctx context.Context, projectRoot, sessionID string, maxBytes int) (string, error)
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
}

type triagePromptEnvelope struct {
	Bead         triagePromptBead    `json:"bead"`
	ExecuteBead  triagePromptReport  `json:"execute_bead_report"`
	RecentEvents []triagePromptEvent `json:"recent_bead_events,omitempty"`
	ProjectRoot  string              `json:"project_root,omitempty"`
}

// triagePromptReport is the control-safe projection of an execution report.
// Post-attempt triage can affect retry and lifecycle decisions, so concrete
// Fizeau route identity must never enter this model prompt. Keep only
// route-neutral outcome facts and public abstract power intent.
type triagePromptReport struct {
	BeadID                        string        `json:"bead_id,omitempty"`
	AttemptID                     string        `json:"attempt_id,omitempty"`
	Status                        string        `json:"status"`
	RateLimitBudget               time.Duration `json:"rate_limit_budget,omitempty"`
	SessionID                     string        `json:"session_id,omitempty"`
	BaseRev                       string        `json:"base_rev,omitempty"`
	ResultRev                     string        `json:"result_rev,omitempty"`
	ReviewVerdict                 string        `json:"review_verdict,omitempty"`
	ActualPower                   int           `json:"actual_power,omitempty"`
	CostUSD                       float64       `json:"cost_usd,omitempty"`
	DurationMS                    int64         `json:"duration_ms,omitempty"`
	RoutingIntentSource           string        `json:"routing_intent_source,omitempty"`
	EstimatedDifficulty           string        `json:"estimated_difficulty,omitempty"`
	InferredMinPower              *int          `json:"inferred_min_power,omitempty"`
	RequestedMinPower             int           `json:"requested_min_power,omitempty"`
	RequestedMaxPower             int           `json:"requested_max_power,omitempty"`
	EscalationCount               int           `json:"escalation_count,omitempty"`
	ExecutionDecision             string        `json:"execution_decision,omitempty"`
	Disrupted                     bool          `json:"disrupted,omitempty"`
	DisruptionReason              string        `json:"disruption_reason,omitempty"`
	OutcomeReason                 string        `json:"outcome_reason,omitempty"`
	ResourceExhaustionDiagnosis   string        `json:"resource_exhaustion_diagnosis,omitempty"`
	ResourceExhaustionRestartable bool          `json:"resource_exhaustion_restartable,omitempty"`
}

type triagePromptEvent struct {
	Category            string    `json:"category"`
	Status              string    `json:"status,omitempty"`
	Classification      string    `json:"classification,omitempty"`
	RecommendedAction   string    `json:"recommended_action,omitempty"`
	EstimatedDifficulty string    `json:"estimated_difficulty,omitempty"`
	ActualPower         *int      `json:"actual_power,omitempty"`
	InferredMinPower    *int      `json:"inferred_min_power,omitempty"`
	RequestedMinPower   *int      `json:"requested_min_power,omitempty"`
	RequestedMaxPower   *int      `json:"requested_max_power,omitempty"`
	Disrupted           *bool     `json:"disrupted,omitempty"`
	CreatedAt           time.Time `json:"created_at,omitempty"`
}

// NewPostAttemptTriageHook constructs the bead-lifecycle post-attempt triage
// hook. The hook is runner-backed, uses the resolved config / runner dispatch
// seam, and never shells out through os/exec.
func NewPostAttemptTriageHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner, sessionLogReader SessionLogExcerptReader) func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
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

		b, err := store.Get(ctx, beadID)
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

		prompt, err := buildPostAttemptTriagePrompt(ctx, projectRoot, b, report, sessionLogReader, eventLister)
		if err != nil {
			return TriageResult{}, err
		}

		result, err := dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, AgentRunRuntime{
			Prompt:        prompt,
			WorkDir:       projectRoot,
			PromptSource:  postAttemptTriagePromptSource,
			Role:          config.EvidenceRoleLifecycle,
			ClearProfile:  true,
			ClearMinPower: true,
		})
		if err != nil {
			return TriageResult{}, fmt.Errorf("triage hook: dispatch: %w", err)
		}

		text, err := triageResultOutput(result)
		if err != nil {
			return TriageResult{}, err
		}
		if text == "" {
			return malformedTriageResult("output", "empty output", ""), nil
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
	// Raw session logs can contain Fizeau routing decisions. Triage is a
	// control path, so retain the reader parameter for API compatibility but
	// never put a route-bearing session excerpt into its prompt.
	_ = ctx
	_ = reader
	env := triagePromptEnvelope{
		Bead:        projectTriagePromptBead(b),
		ExecuteBead: projectTriagePromptReport(report),
		ProjectRoot: projectRoot,
	}

	if eventLister != nil {
		events, err := eventLister.Events(b.ID)
		if err == nil && len(events) > 0 {
			start := len(events) - maxTriageEventCount
			if start < 0 {
				start = 0
			}
			env.RecentEvents = make([]triagePromptEvent, 0, len(events)-start)
			for _, ev := range events[start:] {
				env.RecentEvents = append(env.RecentEvents, projectTriagePromptEvent(clampTriageEvent(ev)))
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

func projectTriagePromptBead(b *bead.Bead) triagePromptBead {
	return triagePromptBead{
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
	}
}

func projectTriagePromptReport(report ExecuteBeadReport) triagePromptReport {
	out := triagePromptReport{
		BeadID:                        report.BeadID,
		AttemptID:                     report.AttemptID,
		Status:                        controlSafeTriageStatus(report.Status),
		RateLimitBudget:               report.RateLimitBudget,
		SessionID:                     report.SessionID,
		BaseRev:                       report.BaseRev,
		ResultRev:                     report.ResultRev,
		ReviewVerdict:                 controlSafeReviewVerdict(report.ReviewVerdict),
		ActualPower:                   report.ActualPower,
		CostUSD:                       report.CostUSD,
		DurationMS:                    report.DurationMS,
		RoutingIntentSource:           controlSafeRoutingIntentSource(report.RoutingIntentSource),
		EstimatedDifficulty:           controlSafeEstimatedDifficulty(report.EstimatedDifficulty),
		RequestedMinPower:             report.RequestedMinPower,
		RequestedMaxPower:             report.RequestedMaxPower,
		EscalationCount:               report.EscalationCount,
		ExecutionDecision:             controlSafeExecutionDecision(report.ExecutionDecision),
		Disrupted:                     report.Disrupted,
		DisruptionReason:              controlSafeOutcomeReason(report.DisruptionReason),
		OutcomeReason:                 controlSafeOutcomeReason(report.OutcomeReason),
		ResourceExhaustionDiagnosis:   controlSafeResourceDiagnosis(report.ResourceExhaustionDiagnosis),
		ResourceExhaustionRestartable: report.ResourceExhaustionRestartable,
	}
	if report.InferredMinPowerPresent {
		value := report.InferredMinPower
		out.InferredMinPower = &value
	}
	return out
}

func projectTriagePromptEvent(event bead.BeadEvent) triagePromptEvent {
	out := triagePromptEvent{
		Category:  triageEventCategory(event.Kind),
		CreatedAt: event.CreatedAt,
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(event.Body), &body); err != nil {
		return out
	}
	if status, ok := body["status"].(string); ok && isControlSafeTriageStatus(status) {
		out.Status = status
	}
	if classification, ok := body["classification"].(string); ok && isRecognizedTriageClassification(classification) {
		out.Classification = classification
	}
	if action, ok := body["recommended_action"].(string); ok && isRecognizedTriageRecommendedAction(action) {
		out.RecommendedAction = action
	}
	if difficulty, ok := body["estimated_difficulty"].(string); ok && isControlSafeEstimatedDifficulty(difficulty) {
		out.EstimatedDifficulty = difficulty
	}
	out.ActualPower = triageEventInt(body, "actual_power")
	out.InferredMinPower = triageEventInt(body, "inferred_min_power")
	out.RequestedMinPower = triageEventInt(body, "requested_min_power")
	out.RequestedMaxPower = triageEventInt(body, "requested_max_power")
	if disrupted, ok := body["disrupted"].(bool); ok {
		out.Disrupted = &disrupted
	}
	return out
}

func triageEventCategory(kind string) string {
	switch strings.TrimSpace(kind) {
	case "execution-routing-intent", "routing", "route-failure":
		return "routing"
	case "execute-bead", "bead.result", "attempt.terminated":
		return "attempt"
	case "bead-quality.triage", "bead-quality.triage-warning":
		return "triage"
	case "bead-quality.lint", "intake.warn", "triage-overflow":
		return "quality"
	case "resource-exhausted", "disruption_detected", "operator_attention":
		return "system"
	default:
		return "other"
	}
}

func triageEventInt(body map[string]any, key string) *int {
	value, ok := body[key].(float64)
	if !ok || value < 0 || value != float64(int(value)) {
		return nil
	}
	out := int(value)
	return &out
}

func isControlSafeEstimatedDifficulty(value string) bool {
	switch strings.TrimSpace(value) {
	case "easy", "medium", "hard":
		return true
	default:
		return false
	}
}

func isControlSafeTriageStatus(value string) bool {
	switch strings.TrimSpace(value) {
	case ExecuteBeadStatusStructuralValidationFailed,
		ExecuteBeadStatusExecutionFailed,
		ExecuteBeadStatusPostRunCheckFailed,
		ExecuteBeadStatusRatchetFailed,
		ExecuteBeadStatusLandConflict,
		ExecuteBeadStatusLandRetry,
		ExecuteBeadStatusLandOperatorAttention,
		ExecuteBeadStatusPushFailed,
		ExecuteBeadStatusPushConflict,
		ExecuteBeadStatusPreservedNeedsReview,
		ExecuteBeadStatusNoEvidenceProduced,
		ExecuteBeadStatusNoChanges,
		ExecuteBeadStatusAlreadySatisfied,
		ExecuteBeadStatusSuccess,
		ExecuteBeadStatusResourceExhausted,
		ExecuteBeadStatusDeclinedNeedsDecomposition,
		ExecuteBeadStatusReviewRequestChanges,
		ExecuteBeadStatusReviewBlock,
		ExecuteBeadStatusReviewMalfunction,
		ExecuteBeadStatusReviewRequestClarification,
		ExecuteBeadStatusLandConflictUnresolvable,
		ExecuteBeadStatusLandConflictOperatorRequired,
		ExecuteBeadStatusReviewTerminalBlock,
		ExecuteBeadStatusReviewFixableGap,
		ExecuteBeadStatusRepairCycleExhausted:
		return true
	default:
		return false
	}
}

func controlSafeTriageStatus(value string) string {
	value = strings.TrimSpace(value)
	if isControlSafeTriageStatus(value) {
		return value
	}
	return ""
}

func controlSafeEstimatedDifficulty(value string) string {
	value = strings.TrimSpace(value)
	if isControlSafeEstimatedDifficulty(value) {
		return value
	}
	return ""
}

func controlSafeReviewVerdict(value string) string {
	value = strings.TrimSpace(value)
	switch Verdict(value) {
	case VerdictApprove, VerdictRequestChanges, VerdictBlock, VerdictRequestClarification:
		return value
	default:
		return ""
	}
}

func controlSafeRoutingIntentSource(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "default", "bead_hint", "readiness", "project_config", "cli", "profile":
		return value
	default:
		return ""
	}
}

func controlSafeExecutionDecision(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "proposed", "execution_ineligible":
		return value
	default:
		return ""
	}
}

func controlSafeOutcomeReason(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "routing", "quota", "transport", "timeout", "recoverable",
		"context_canceled", "transport_error", "resource_exhausted", "repo_concurrency",
		"unavailable", "environment_repairable", "preflight_failed", "intake_block",
		"claim_race", "actionable_but_rewritten", "too_large_decomposed", "decomposed",
		"intake_error", "ambiguous_needs_human", "operator_required",
		FailureModeContextOverflow, FailureModeMergeConflict, FailureModeTestFailure,
		FailureModeBuildFailure, FailureModeAuthError, FailureModeNoChanges,
		FailureModeNoEvidenceProduced, FailureModeRatchetMiss, FailureModeNoViableProvider,
		FailureModeProviderConnectivity, FailureModeServerUnavailable, FailureModeHarnessNotInstalled,
		FailureModeBlockedByPassthroughConstraint, FailureModeAgentPowerUnsatisfied,
		FailureModeLockContention, FailureModeWorktreeLost, FailureModeRouteResolutionTimeout,
		FailureModeProgressWatchdog, FailureModeConsecutiveWedge, FailureModeAttemptIntegrity,
		FailureModeLandRetry, FailureModeLandOperatorAttention, FailureModeUnknown:
		return value
	default:
		return ""
	}
}

func controlSafeResourceDiagnosis(value string) string {
	value = strings.TrimSpace(value)
	if value == ResourceExhaustionDiagnosisFD {
		return value
	}
	return ""
}

func triageResultOutput(result *Result) (string, error) {
	if result == nil {
		return "", fmt.Errorf("triage hook: empty runner result")
	}
	text := strings.TrimSpace(result.CondensedOutput)
	if text == "" {
		text = strings.TrimSpace(result.Output)
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

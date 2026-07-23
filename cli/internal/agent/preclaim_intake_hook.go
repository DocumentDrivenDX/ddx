package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	agentlib "github.com/easel/fizeau"
)

// PreClaimIntakePromptSource identifies the pre-claim complexity-eval
// dispatch in session logs and tests.
const PreClaimIntakePromptSource = "bead-lifecycle-intake"

const readinessChecksSchemaPath = "cli/internal/agent/schema/readiness-checks.schema.json"

type preClaimIntakePromptEnvelope struct {
	Title         string                  `json:"title"`
	Description   string                  `json:"description"`
	Acceptance    string                  `json:"acceptance"`
	Notes         string                  `json:"notes,omitempty"`
	Labels        []string                `json:"labels"`
	PriorAttempts []preClaimIntakeAttempt `json:"prior_attempts"`
	Depth         int                     `json:"depth"`
	Parent        string                  `json:"parent,omitempty"`
	Dependencies  []string                `json:"dependencies,omitempty"`
}

type preClaimIntakeAttempt struct {
	Status    string `json:"status"`
	Rationale string `json:"rationale,omitempty"`
}

type preClaimIntakePromptResult struct {
	Classification string                      `json:"classification"`
	Confidence     any                         `json:"confidence,omitempty"`
	Reasoning      string                      `json:"reasoning,omitempty"`
	Rewrite        preClaimIntakePromptRewrite `json:"rewrite,omitempty"`
}

type preClaimIntakePromptRewrite struct {
	Description   string   `json:"description,omitempty"`
	Acceptance    string   `json:"acceptance,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
}

func (r *preClaimIntakePromptRewrite) UnmarshalJSON(data []byte) error {
	var raw struct {
		Description   string          `json:"description,omitempty"`
		Acceptance    json.RawMessage `json:"acceptance,omitempty"`
		ChangedFields []string        `json:"changed_fields,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	acceptance, err := decodeReadinessAcceptanceText(raw.Acceptance, "rewrite.acceptance")
	if err != nil {
		return err
	}
	r.Description = strings.TrimSpace(raw.Description)
	r.Acceptance = acceptance
	r.ChangedFields = raw.ChangedFields
	return nil
}

// preClaimReadinessPromptResult is the canonical readiness JSON schema returned
// by skills that use the FEAT-010/ADR-023 outcome vocabulary.
type preClaimReadinessPromptResult struct {
	Outcome    string                      `json:"outcome"`
	Reason     string                      `json:"reason,omitempty"`
	Detail     string                      `json:"detail,omitempty"`
	Difficulty preClaimReadinessDifficulty `json:"difficulty,omitempty"`
	Rewrite    preClaimIntakePromptRewrite `json:"rewrite,omitempty"`
}

type preClaimReadinessClassificationPromptResult struct {
	Classification    string                                 `json:"classification"`
	Tractability      string                                 `json:"tractability,omitempty"`
	Score             preClaimReadinessScore                 `json:"score,omitempty"`
	Rationale         string                                 `json:"rationale,omitempty"`
	Detail            string                                 `json:"detail,omitempty"`
	Reasoning         string                                 `json:"reasoning,omitempty"`
	Difficulty        preClaimReadinessDifficulty            `json:"difficulty,omitempty"`
	ReadinessChecks   preClaimReadinessChecksPayload         `json:"readiness_checks,omitempty"`
	SuggestedFixes    preClaimReadinessSuggestedFixesPayload `json:"suggested_fixes,omitempty"`
	SuggestedChildren []preClaimReadinessSuggestedChild      `json:"suggested_child_beads,omitempty"`
	WaiversApplied    preClaimReadinessWaiversPayload        `json:"waivers_applied,omitempty"`
	Rewrite           preClaimIntakePromptRewrite            `json:"rewrite,omitempty"`
}

type preClaimReadinessCheck struct {
	Reason                 string           `json:"reason,omitempty"`
	Verdict                readinessVerdict `json:"verdict,omitempty"`
	Evidence               string           `json:"evidence,omitempty"`
	CheckableBeforeAttempt bool             `json:"checkable_before_attempt,omitempty"`
}

// readinessVerdict is the verdict reported for a single readiness check.
// Keep this coercion aligned with cli/internal/agent/schema/readiness-checks.schema.json
// so the shared producer prompt and this decoder accept the same payload shape.
// The intake producer may emit verdict as a JSON bool, a JSON string, or omit
// it entirely; UnmarshalJSON coerces all three forms to a canonical lowercased
// string so downstream fail-detection (failedReadinessReasons) can compare it
// deterministically:
//   - JSON bool true  -> "pass"
//   - JSON bool false -> "fail"
//   - JSON string     -> trimmed, lowercased value (for example "PASS" -> "pass")
//   - absent / null   -> "" (empty, same as a missing field)
type readinessVerdict string

func (v *readinessVerdict) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*v = ""
		return nil
	}
	switch trimmed[0] {
	case 't', 'f':
		var b bool
		if err := json.Unmarshal(data, &b); err != nil {
			return err
		}
		if b {
			*v = "pass"
		} else {
			*v = "fail"
		}
		return nil
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*v = readinessVerdict(strings.ToLower(strings.TrimSpace(s)))
		return nil
	default:
		return fmt.Errorf("readiness_checks verdict must be a bool, string, or null, got %s", jsonKindName(trimmed[0]))
	}
}

func jsonKindName(first byte) string {
	switch first {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "bool"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

type preClaimReadinessSuggestedFix struct {
	Target string `json:"target,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

type preClaimReadinessDifficulty struct {
	EstimatedDifficulty string `json:"estimated_difficulty,omitempty"`
	Reason              string `json:"reason,omitempty"`
}

type preClaimReadinessScore struct {
	Present bool
}

func (s *preClaimReadinessScore) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	s.Present = true
	return nil
}

type preClaimReadinessSuggestedFixesPayload []preClaimReadinessSuggestedFix

func (p *preClaimReadinessSuggestedFixesPayload) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed[0] != '[' {
		return fmt.Errorf("suggested_fixes must be a JSON array")
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	fixes := make([]preClaimReadinessSuggestedFix, 0, len(raw))
	for _, item := range raw {
		itemTrimmed := strings.TrimSpace(string(item))
		if itemTrimmed == "" || itemTrimmed == "null" {
			continue
		}
		switch itemTrimmed[0] {
		case '"':
			var fix string
			if err := json.Unmarshal(item, &fix); err != nil {
				return err
			}
			fixes = append(fixes, preClaimReadinessSuggestedFix{Fix: strings.TrimSpace(fix)})
		case '{':
			var fix preClaimReadinessSuggestedFix
			if err := json.Unmarshal(item, &fix); err != nil {
				return err
			}
			fix.Target = strings.TrimSpace(fix.Target)
			fix.Fix = strings.TrimSpace(fix.Fix)
			fixes = append(fixes, fix)
		default:
			return fmt.Errorf("suggested_fixes entries must be strings or objects")
		}
	}
	*p = fixes
	return nil
}

func normalizeReadinessStringList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func decodeReadinessStringListField(data []byte, fieldName string, splitLines bool) ([]string, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	switch trimmed[0] {
	case '[':
		var items []string
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
		return normalizeReadinessStringList(items), nil
	case '"':
		var item string
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if splitLines {
			return normalizeReadinessStringList(strings.Split(item, "\n")), nil
		}
		return normalizeReadinessStringList([]string{item}), nil
	default:
		return nil, fmt.Errorf("%s must be a string or string array", fieldName)
	}
}

func decodeReadinessAcceptanceList(data []byte, fieldName string) ([]string, error) {
	return decodeReadinessStringListField(data, fieldName, true)
}

func decodeReadinessAcceptanceText(data []byte, fieldName string) (string, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return "", nil
	}
	switch trimmed[0] {
	case '[':
		var items []string
		if err := json.Unmarshal(data, &items); err != nil {
			return "", fmt.Errorf("%s must be a string or string array: %w", fieldName, err)
		}
		items = normalizeReadinessStringList(items)
		if len(items) == 0 {
			return "", nil
		}
		return strings.Join(items, "\n"), nil
	case '"':
		var item string
		if err := json.Unmarshal(data, &item); err != nil {
			return "", fmt.Errorf("%s must be a string or string array: %w", fieldName, err)
		}
		return strings.TrimSpace(item), nil
	default:
		return "", fmt.Errorf("%s must be a string or string array", fieldName)
	}
}

type preClaimReadinessSuggestedChild struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Acceptance  []string `json:"acceptance,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	Deps        []string `json:"deps,omitempty"`
}

func (c *preClaimReadinessSuggestedChild) UnmarshalJSON(data []byte) error {
	var raw struct {
		Title       string          `json:"title,omitempty"`
		Description string          `json:"description,omitempty"`
		Acceptance  json.RawMessage `json:"acceptance,omitempty"`
		Labels      []string        `json:"labels,omitempty"`
		Parent      string          `json:"parent,omitempty"`
		Deps        []string        `json:"deps,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	acceptance, err := decodeReadinessAcceptanceList(raw.Acceptance, "suggested_child_beads[].acceptance")
	if err != nil {
		return err
	}
	c.Title = raw.Title
	c.Description = raw.Description
	c.Acceptance = acceptance
	c.Labels = raw.Labels
	c.Parent = raw.Parent
	c.Deps = raw.Deps
	return nil
}

type preClaimReadinessWaiver struct {
	Reason   string   `json:"reason,omitempty"`
	Criteria []string `json:"criteria,omitempty"`
	Evidence string   `json:"evidence,omitempty"`
}

func (w *preClaimReadinessWaiver) UnmarshalJSON(data []byte) error {
	var raw struct {
		Reason   string          `json:"reason,omitempty"`
		Criteria json.RawMessage `json:"criteria,omitempty"`
		Evidence string          `json:"evidence,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	criteria, err := decodeReadinessStringListField(raw.Criteria, "waivers_applied[].criteria", false)
	if err != nil {
		return err
	}
	w.Reason = strings.TrimSpace(raw.Reason)
	w.Criteria = criteria
	w.Evidence = strings.TrimSpace(raw.Evidence)
	return nil
}

type preClaimReadinessWaiversPayload []preClaimReadinessWaiver

func (p *preClaimReadinessWaiversPayload) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var reason string
		if err := json.Unmarshal(data, &reason); err != nil {
			return err
		}
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return nil
		}
		*p = []preClaimReadinessWaiver{{Reason: reason}}
		return nil
	}
	if trimmed[0] != '[' {
		return fmt.Errorf("waivers_applied must be a JSON array or string")
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	waivers := make([]preClaimReadinessWaiver, 0, len(raw))
	for _, item := range raw {
		itemTrimmed := strings.TrimSpace(string(item))
		if itemTrimmed == "" || itemTrimmed == "null" {
			continue
		}
		switch itemTrimmed[0] {
		case '"':
			var reason string
			if err := json.Unmarshal(item, &reason); err != nil {
				return err
			}
			reason = strings.TrimSpace(reason)
			if reason == "" {
				continue
			}
			waivers = append(waivers, preClaimReadinessWaiver{Reason: reason})
		case '{':
			var waiver preClaimReadinessWaiver
			if err := json.Unmarshal(item, &waiver); err != nil {
				return err
			}
			waiver.Reason = strings.TrimSpace(waiver.Reason)
			waiver.Criteria = normalizeReadinessStringList(waiver.Criteria)
			waiver.Evidence = strings.TrimSpace(waiver.Evidence)
			waivers = append(waivers, waiver)
		default:
			return fmt.Errorf("waivers_applied entries must be strings or objects")
		}
	}
	*p = waivers
	return nil
}

// preClaimReadinessChecksPayload mirrors cli/internal/agent/schema/readiness-checks.schema.json
// so the producer prompt and this decoder stay aligned on the same readiness payload shape.
type preClaimReadinessChecksPayload struct {
	Checks    []preClaimReadinessCheck
	Present   bool
	Malformed string
	Evidence  string
}

func (p *preClaimReadinessChecksPayload) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	p.Present = true
	switch trimmed[0] {
	case '[':
		return json.Unmarshal(data, &p.Checks)
	case '{':
		var check preClaimReadinessCheck
		if err := json.Unmarshal(data, &check); err != nil {
			return err
		}
		p.Checks = []preClaimReadinessCheck{check}
		return nil
	case '"':
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		p.Malformed = "readiness_checks must be an object or array"
		p.Evidence = value
		return nil
	default:
		p.Malformed = "readiness_checks must be an object or array"
		p.Evidence = trimmed
		return nil
	}
}

func (p preClaimReadinessChecksPayload) Len() int {
	return len(p.Checks)
}

// NewPreClaimIntakeHook constructs the bead-intake complexity gate used
// before claim in the work / work paths. The hook evaluates the bead
// using the repository's triage prompt and returns one of the typed intake
// outcomes so the loop can decide whether to claim or skip the candidate.
//
// The hook uses the normal service execution path. Unpinned workers use the
// warmed lifecycle profile snapshot when available; explicitly pinned workers
// keep their operator route pins. Route failures are infrastructure failures, not
// bead-readiness decisions, so they return intake_error and let the loop use
// its fail-open readiness path.
func NewPreClaimIntakeHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner) func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
	return newPreClaimIntakeHook(projectRoot, store, rcfg, svc, runner, nil, false)
}

// NewPreClaimIntakeHookWithLog constructs the pre-claim intake hook and emits
// compact prompt metadata plus live service progress to log when provided.
func NewPreClaimIntakeHookWithLog(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner, log io.Writer) func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
	return newPreClaimIntakeHook(projectRoot, store, rcfg, svc, runner, log, false)
}

// NewPreClaimIntakeHookWithLogVerbose constructs the pre-claim intake hook and
// prints the full prompt only when promptVerbose is true.
func NewPreClaimIntakeHookWithLogVerbose(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner, log io.Writer, promptVerbose bool) func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
	_ = NewPreClaimIntakeHook(projectRoot, store, rcfg, svc, runner)
	return newPreClaimIntakeHook(projectRoot, store, rcfg, svc, runner, log, promptVerbose)
}

func newPreClaimIntakeHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner, log io.Writer, promptVerbose bool) func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
	return func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return PreClaimIntakeResult{}, err
			}
		}
		if strings.TrimSpace(projectRoot) == "" {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: project root required")
		}
		if err := ensureBeadLifecycleSkill(projectRoot); err != nil {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: %w", err)
		}
		if store == nil {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: bead reader required")
		}

		b, err := store.Get(ctx, beadID)
		if err != nil {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: load bead %s: %w", beadID, err)
		}

		prompt, err := buildPreClaimIntakePrompt(projectRoot, store, b)
		if err != nil {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: build prompt: %w", err)
		}

		runtime := AgentRunRuntime{
			Prompt:           prompt,
			PromptSource:     PreClaimIntakePromptSource,
			Role:             config.EvidenceRoleLifecycle,
			Output:           log,
			WorkLogPhase:     "readiness",
			Correlation:      map[string]string{"bead_id": beadID},
			ClearProfile:     true,
			MinPowerOverride: lifecycleStandardMinPower,
			// Readiness is a text-only judgment of the bead's own definition; it
			// must not spider the repository (ddx-d5d1ada7). Do not request repo
			// tools for this dispatch.
			RequiresTools: false,
		}
		logPreClaimIntakePrompt(log, projectRoot, beadID, prompt, runtime, promptVerbose)
		applyLifecycleHookRouting(rcfg, &runtime)
		logPreClaimIntakeRoute(log, beadID, runtime, promptVerbose)
		payload, err := dispatchPreClaimIntakePayload(ctx, projectRoot, svc, runner, rcfg, runtime)
		if err != nil {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeError,
				Detail:  preClaimIntakeRouteUnavailableDetail(err),
			}, nil
		}

		return decodePreClaimIntakePayloadResultWithMode(payload, rcfg.BeadQualityMode())
	}
}

func logPreClaimIntakePrompt(w io.Writer, projectRoot, beadID, prompt string, runtime AgentRunRuntime, verbose bool) {
	if w == nil || !verbose {
		return
	}
	logDir := ResolveLogDir(projectRoot, "")
	_, _ = fmt.Fprintf(w, "readiness prompt %s: sent source=%s bytes=%d session_logs=%s\n", beadID, PreClaimIntakePromptSource, len(prompt), logDir)
	_, _ = fmt.Fprintf(w, "readiness prompt %s begin\n%s", beadID, truncatePreClaimIntakePromptForLog(prompt))
	if !strings.HasSuffix(prompt, "\n") {
		_, _ = fmt.Fprintln(w)
	}
	_, _ = fmt.Fprintf(w, "readiness prompt %s end\n", beadID)
}

func logPreClaimIntakeRoute(w io.Writer, beadID string, runtime AgentRunRuntime, verbose bool) {
	if w == nil || !verbose {
		return
	}
	if route := preClaimIntakeRouteSummary(runtime); route != "" {
		_, _ = fmt.Fprintf(w, "readiness prompt %s: route %s\n", beadID, route)
	}
}

func preClaimIntakeRouteSummary(runtime AgentRunRuntime) string {
	var fields []string
	if runtime.ProfileOverride != "" {
		fields = append(fields, "profile="+runtime.ProfileOverride)
	}
	if runtime.HarnessOverride != "" {
		fields = append(fields, "harness="+runtime.HarnessOverride)
	}
	if runtime.ProviderOverride != "" {
		fields = append(fields, "provider="+runtime.ProviderOverride)
	}
	if runtime.ModelOverride != "" {
		fields = append(fields, "model="+runtime.ModelOverride)
	}
	if runtime.MinPowerOverride > 0 {
		fields = append(fields, "min_power="+strconv.Itoa(runtime.MinPowerOverride))
	}
	return strings.Join(fields, " ")
}

func truncatePreClaimIntakePromptForLog(prompt string) string {
	const limit = 32 * 1024
	if len(prompt) <= limit {
		return prompt
	}
	const marker = "\n...[readiness prompt truncated in worker log; full prompt is in the service session log]...\n"
	head := (limit - len(marker)) / 2
	tail := limit - len(marker) - head
	if head <= 0 || tail <= 0 {
		return prompt[:limit]
	}
	return prompt[:head] + marker + prompt[len(prompt)-tail:]
}

func applyLifecycleHookRouting(rcfg config.ResolvedConfig, runtime *AgentRunRuntime) {
	runtime.Role = config.EvidenceRoleLifecycle
	runtime.ClearRoutingPins = true

	if harness, ok := rcfg.ExplicitHarness(); ok {
		runtime.HarnessOverride = harness
	}
	if provider, ok := rcfg.ExplicitProvider(); ok {
		runtime.ProviderOverride = provider
	}
	if model, ok := rcfg.ExplicitModel(); ok {
		runtime.ModelOverride = model
	}
	if rcfg.Profile() != "" {
		runtime.ProfileOverride = rcfg.Profile()
	}
}

func dispatchPreClaimIntakePayload(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (string, error) {
	return dispatchPreClaimIntakePayloadOnce(ctx, projectRoot, svc, runner, rcfg, runtime)
}

func preClaimIntakeRouteUnavailableDetail(err error) string {
	detail := ""
	if err != nil {
		detail = strings.TrimSpace(err.Error())
	}
	detail = trimDiagnosticPrefix(detail, "pre-claim intake")
	detail = strings.TrimPrefix(strings.TrimSpace(detail), "dispatch: ")
	if detail == "" {
		return "readiness route unavailable"
	}
	return "readiness route unavailable: " + detail
}

func dispatchPreClaimIntakePayloadOnce(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (string, error) {
	result, err := dispatchLifecycleRun(ctx, projectRoot, svc, runner, rcfg, runtime)
	if err != nil {
		return "", fmt.Errorf("pre-claim intake: dispatch: %w", err)
	}
	return intakeResultPayload(result)
}

func buildPreClaimIntakePrompt(projectRoot string, store BeadReader, b *bead.Bead) (string, error) {
	env := preClaimIntakePromptEnvelope{
		Title:         strings.TrimSpace(b.Title),
		Description:   strings.TrimSpace(b.Description),
		Acceptance:    strings.TrimSpace(b.Acceptance),
		Notes:         strings.TrimSpace(b.Notes),
		Labels:        append([]string(nil), b.Labels...),
		PriorAttempts: []preClaimIntakeAttempt{},
		Depth:         beadDecompositionDepth(projectRoot, b),
		Parent:        strings.TrimSpace(b.Parent),
		Dependencies:  append([]string(nil), b.DepIDs()...),
	}

	// Prior attempts are optional for the gate. If bead history is available,
	// include the last few attempt/result summaries to help the classifier
	// distinguish a first-pass atomic bead from repeated decomposition.
	if store != nil {
		if eventStore, ok := any(store).(interface {
			Events(id string) ([]bead.BeadEvent, error)
		}); ok {
			if events, err := eventStore.Events(b.ID); err == nil && len(events) > 0 {
				for i := len(events) - 1; i >= 0 && len(env.PriorAttempts) < 5; i-- {
					ev := events[i]
					if ev.Kind != "bead.result" && ev.Kind != "execute-bead" {
						continue
					}
					env.PriorAttempts = append(env.PriorAttempts, preClaimIntakeAttempt{
						Status:    strings.TrimSpace(ev.Summary),
						Rationale: strings.TrimSpace(ev.Body),
					})
				}
			}
		}
	}

	body, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("MODE: intake\n")
	sb.WriteString("You are evaluating whether this bead is atomic, decomposable, ambiguous, or safely refinable before claim.\n")
	sb.WriteString("Judge ONLY from the bead fields in the JSON below. Do not read files, run commands, grep, or explore the repository; readiness is a fast text-only assessment of the bead's own definition. A file:line or Test* reference in the bead is a quality signal to look for in the text, not a cue to open the file.\n")
	sb.WriteString("Do not rewrite bead fields in intake mode. If the bead is executable as written, classify it as ready even when the prose could be cleaner.\n")
	sb.WriteString("Canonical schema: " + readinessChecksSchemaPath + ". Treat it as the source of truth for readiness_checks[].verdict and for forward-compatible extra fields.\n")
	sb.WriteString("readiness_checks[].verdict may be a JSON bool, string, null, or omitted; match the schema and the Go decoder contract exactly.\n")
	sb.WriteString("Return exactly one JSON object matching the readiness schema with classification, tractability, score, rationale, difficulty, readiness_checks, suggested_fixes, rewrite, suggested_child_beads, and waivers_applied.\n")
	sb.WriteString("readiness_checks MUST be a JSON array; it may be empty, and every entry MUST be an object with reason, verdict, evidence, and checkable_before_attempt. It must not be an object or string.\n")
	sb.WriteString("suggested_fixes MUST be a JSON array; use a flat string list for prompt-quality suggestions, or an empty array when none apply.\n")
	sb.WriteString("suggested_child_beads MUST be a JSON array of objects. When a child includes acceptance, prefer a JSON string array of numbered criteria; the decoder tolerates a single string fallback, but do not rely on it.\n")
	sb.WriteString("waivers_applied MUST be a JSON array. Prefer waiver objects with reason, criteria, and evidence; the decoder tolerates a flat string list fallback, but do not rely on it.\n")
	sb.WriteString("difficulty MUST be an object with estimated_difficulty and reason. estimated_difficulty MUST be exactly one of easy, medium, or hard.\n")
	sb.WriteString("Use medium as the default implementation difficulty when uncertain or when the task is ordinary build/test/code work; medium maps to standard dispatch.\n")
	sb.WriteString("Use easy only for work suitable for cheap dispatch: narrow mechanical edits such as typo fixes, formatting, simple docs/prose tweaks, straightforward fixture updates, or one-file transforms with low blast radius.\n")
	sb.WriteString("Use hard only for work suitable for smart dispatch: architecture or ambiguous tradeoff judgment, multiple subsystems with high blast radius, security/data-loss/concurrency risk, or prior attempts showing standard power is insufficient.\n")
	sb.WriteString("Do not choose hard just because a bead is important, long, or could be written more cleanly. Readiness score and difficulty are separate: low readiness means refine/split/block, not hard.\n")
	sb.WriteString("If the bead is not executable as written but can be made executable by a narrow, semantics-preserving metadata/AC rewrite, emit rewrite with changed_fields, description, and acceptance.\n")
	sb.WriteString("Put prompt-quality improvements in suggested_fixes only; keep operator_required for actual blockers.\n")
	sb.WriteString("Preservation rules: non-scope items, governing artifact references (FEAT-NNN, ADR-NNN), named test functions (TestFoo), file:line evidence, and dependency IDs (ddx-XXXXXXXX) must all appear in the replacement description.\n")
	sb.WriteString("Classify as operator_required only when ambiguity, missing prerequisites, hidden external blockers, or unsafe scope choices prevent an implementation attempt.\n")
	sb.WriteString("Stale-blocker precedence: if bead notes or a reopen event since the last blocker prior_attempt explicitly state the blocker was cleared, resolved, or unblocked, treat that prior_attempt blocker as historical context only. Do not classify the bead as operator_required or blocked based on a stale prior_attempt event alone when notes or a reopen since that event explicitly cleared it. A newer note or reopen supersedes an older blocker prior_attempt unless current cheap evidence revalidates the blocker.\n")
	sb.WriteString("Do not include prose or markdown.\n\n")
	sb.WriteString("```json\n")
	sb.Write(body)
	sb.WriteString("\n```\n")
	return sb.String(), nil
}

func intakeResultPayload(result *Result) (string, error) {
	if result == nil {
		return "", fmt.Errorf("pre-claim intake: empty runner result")
	}
	text := strings.TrimSpace(result.CondensedOutput)
	if text == "" {
		text = strings.TrimSpace(result.Output)
	}
	if text == "" {
		if strings.TrimSpace(result.Error) != "" {
			return "", fmt.Errorf("pre-claim intake: runner error: %s", strings.TrimSpace(result.Error))
		}
		if result.ExitCode != 0 {
			return "", fmt.Errorf("pre-claim intake: runner exited with code %d and empty output", result.ExitCode)
		}
		return "", fmt.Errorf("pre-claim intake: empty output")
	}
	candidate, ok := extractJSONCandidate(text)
	if !ok {
		return "", fmt.Errorf("pre-claim intake: no JSON object found")
	}
	return candidate, nil
}

func normalizePreClaimIntakeRewriteFields(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.ToLower(strings.TrimSpace(field))
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func decodePreClaimIntakePayloadResultWithMode(payload string, qualityMode string) (PreClaimIntakeResult, error) {
	var probe struct {
		Classification  string                                 `json:"classification"`
		Outcome         string                                 `json:"outcome"`
		Tractability    string                                 `json:"tractability"`
		Rationale       string                                 `json:"rationale"`
		Score           preClaimReadinessScore                 `json:"score"`
		Difficulty      preClaimReadinessDifficulty            `json:"difficulty"`
		ReadinessChecks preClaimReadinessChecksPayload         `json:"readiness_checks"`
		SuggestedFixes  preClaimReadinessSuggestedFixesPayload `json:"suggested_fixes"`
	}
	if err := json.Unmarshal([]byte(payload), &probe); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode result: %w", err)
	}
	if probe.Outcome != "" {
		return decodeCanonicalReadinessPayloadWithMode(payload, qualityMode)
	}
	if probe.Classification != "" {
		if isReadinessClassificationPayload(probe.Classification, probe.Tractability, probe.Rationale, probe.Score.Present, normalizeReadinessEstimatedDifficulty(probe.Difficulty.EstimatedDifficulty) != "", probe.ReadinessChecks.Present, probe.ReadinessChecks.Len(), len(probe.SuggestedFixes)) {
			return decodeReadinessClassificationPayloadWithMode(payload, qualityMode)
		}
		return decodeLegacyIntakePayload(payload)
	}
	return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: missing classification or outcome field")
}

func isReadinessClassificationPayload(classification, tractability, rationale string, scorePresent, difficultyPresent bool, readinessChecksPresent bool, checkCount, fixCount int) bool {
	switch strings.ToLower(strings.TrimSpace(classification)) {
	case ReadinessClassificationSystemUnready,
		"readiness_error",
		"intake_error",
		ReadinessClassificationNeedsRefine,
		ReadinessClassificationNeedsSplit,
		ReadinessClassificationOperatorRequired,
		"ambiguous",
		"safely_refinable",
		"split":
		return true
	case "ready":
		return readinessChecksPresent ||
			strings.TrimSpace(tractability) != "" ||
			strings.TrimSpace(rationale) != "" ||
			scorePresent ||
			difficultyPresent ||
			checkCount > 0 ||
			fixCount > 0
	default:
		return false
	}
}

func decodeCanonicalReadinessPayloadWithMode(payload string, qualityMode string) (PreClaimIntakeResult, error) {
	var out preClaimReadinessPromptResult
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode readiness result: %w", err)
	}
	detail := strings.TrimSpace(out.Reason)
	if detail == "" {
		detail = strings.TrimSpace(out.Detail)
	}
	rewrite := PreClaimIntakeRewrite{
		Description:   strings.TrimSpace(out.Rewrite.Description),
		Acceptance:    strings.TrimSpace(out.Rewrite.Acceptance),
		ChangedFields: normalizePreClaimIntakeRewriteFields(out.Rewrite.ChangedFields),
	}
	estimatedDifficulty := normalizeReadinessEstimatedDifficulty(out.Difficulty.EstimatedDifficulty)
	switch strings.ToLower(strings.TrimSpace(out.Outcome)) {
	case "actionable_atomic":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic, Detail: detail, EstimatedDifficulty: estimatedDifficulty}, nil
	case "actionable_but_rewritten":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableButRewritten, Detail: detail, EstimatedDifficulty: estimatedDifficulty, Rewrite: rewrite}, nil
	case "too_large_decomposed":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeTooLargeDecomposed, Reason: ReadinessReasonTooLarge, Detail: detail, EstimatedDifficulty: estimatedDifficulty}, nil
	case "operator_required", "human_review_required":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeOperatorRequired, Reason: ReadinessReasonAmbiguousScope, Detail: detail, EstimatedDifficulty: estimatedDifficulty}, nil
	case "ambiguous_needs_human", "needs_human":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: legacy readiness outcome %q removed by lifecycle migration; use %q", out.Outcome, PreClaimIntakeOperatorRequired)
	case "readiness_error", "system_unready":
		// system_unready maps to fail-open/skip per ADR-023/FEAT-010 policy
		classified := ClassifyReadinessWithMode(ReadinessClassificationSystemUnready, nil, detail, qualityMode)
		return PreClaimIntakeResult{
			Outcome:      PreClaimIntakeError,
			Reason:       classified.Reason,
			SystemReason: classified.SystemReason,
			Detail:       detail,
		}, nil
	case "":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: missing outcome")
	default:
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: unknown readiness outcome %q: expected one of actionable_atomic, actionable_but_rewritten, too_large_decomposed, operator_required, readiness_error, system_unready", out.Outcome)
	}
}

func decodeReadinessClassificationPayloadWithMode(payload string, qualityMode string) (PreClaimIntakeResult, error) {
	var out preClaimReadinessClassificationPromptResult
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode readiness classification result: %w", err)
	}
	if out.ReadinessChecks.Malformed != "" {
		detail := malformedReadinessChecksDetail(out.ReadinessChecks)
		classified := ClassifyReadinessWithMode(ReadinessClassificationSystemUnready, nil, detail, qualityMode)
		return PreClaimIntakeResult{
			Outcome:      classified.IntakeOutcome,
			Reason:       classified.Reason,
			SystemReason: classified.SystemReason,
			Detail:       detail,
		}, nil
	}
	reasons := failedReadinessReasons(out.ReadinessChecks.Checks)
	detail := firstNonEmptyReadinessDetail(out.Rationale, out.Detail, out.Reasoning)
	classified := ClassifyReadinessWithMode(out.Classification, reasons, detail, qualityMode)
	if detail == "" {
		detail = readinessCheckEvidence(out.ReadinessChecks.Checks)
	}
	detail = readinessDetailWithReason(classified, detail)

	rewrite := PreClaimIntakeRewrite{
		Description:   strings.TrimSpace(out.Rewrite.Description),
		Acceptance:    strings.TrimSpace(out.Rewrite.Acceptance),
		ChangedFields: normalizePreClaimIntakeRewriteFields(out.Rewrite.ChangedFields),
	}
	result := PreClaimIntakeResult{
		Outcome:             classified.IntakeOutcome,
		Reason:              classified.Reason,
		SystemReason:        classified.SystemReason,
		Detail:              detail,
		EstimatedDifficulty: normalizeReadinessEstimatedDifficulty(out.Difficulty.EstimatedDifficulty),
	}
	if classified.Classification == ReadinessClassificationNeedsRefine && hasPreClaimIntakeRewrite(rewrite) {
		result.Outcome = PreClaimIntakeActionableButRewritten
		result.Rewrite = rewrite
	}
	return result, nil
}

func failedReadinessReasons(checks []preClaimReadinessCheck) []string {
	reasons := make([]string, 0, len(checks))
	for _, check := range checks {
		verdict := strings.ToLower(strings.TrimSpace(string(check.Verdict)))
		if verdict != "fail" {
			continue
		}
		if reason := strings.TrimSpace(check.Reason); reason != "" {
			reasons = append(reasons, reason)
		}
	}
	return reasons
}

func malformedReadinessChecksDetail(checks preClaimReadinessChecksPayload) string {
	detail := strings.TrimSpace(checks.Malformed)
	if detail == "" {
		detail = "readiness_checks must be an object or array"
	}
	if evidence := strings.TrimSpace(checks.Evidence); evidence != "" {
		detail += ": " + strconv.Quote(evidence)
	}
	return detail
}

func readinessCheckEvidence(checks []preClaimReadinessCheck) string {
	for _, check := range checks {
		if strings.ToLower(strings.TrimSpace(string(check.Verdict))) != "fail" {
			continue
		}
		if evidence := strings.TrimSpace(check.Evidence); evidence != "" {
			return evidence
		}
	}
	return ""
}

func readinessDetailWithReason(classified ReadinessClassificationResult, detail string) string {
	detail = strings.TrimSpace(detail)
	reason := strings.TrimSpace(classified.Reason)
	if reason == "" {
		return detail
	}
	if detail == "" {
		return reason
	}
	if strings.Contains(strings.ToLower(detail), strings.ToLower(reason)) {
		return detail
	}
	return reason + ": " + detail
}

func firstNonEmptyReadinessDetail(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func hasPreClaimIntakeRewrite(rewrite PreClaimIntakeRewrite) bool {
	return strings.TrimSpace(rewrite.Description) != "" ||
		strings.TrimSpace(rewrite.Acceptance) != "" ||
		len(rewrite.ChangedFields) > 0
}

func normalizeReadinessEstimatedDifficulty(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(escalation.DifficultyEasy):
		return string(escalation.DifficultyEasy)
	case string(escalation.DifficultyMedium):
		return string(escalation.DifficultyMedium)
	case string(escalation.DifficultyHard):
		return string(escalation.DifficultyHard)
	default:
		return ""
	}
}

func decodeLegacyIntakePayload(payload string) (PreClaimIntakeResult, error) {
	var out preClaimIntakePromptResult
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode result: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(out.Classification)) {
	case "atomic", "ok", "ready", "actionable", "pass":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic, Detail: strings.TrimSpace(out.Reasoning)}, nil
	case "rewritten":
		return PreClaimIntakeResult{
			Outcome: PreClaimIntakeActionableButRewritten,
			Detail:  strings.TrimSpace(out.Reasoning),
			Rewrite: PreClaimIntakeRewrite{
				Description:   strings.TrimSpace(out.Rewrite.Description),
				Acceptance:    strings.TrimSpace(out.Rewrite.Acceptance),
				ChangedFields: normalizePreClaimIntakeRewriteFields(out.Rewrite.ChangedFields),
			},
		}, nil
	case "decomposable":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeTooLargeDecomposed, Detail: strings.TrimSpace(out.Reasoning)}, nil
	case "ambiguous", "ambiguous_scope", "needs_human", "ambiguous_needs_human":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: legacy classification %q removed by lifecycle migration; use outcome %q", out.Classification, PreClaimIntakeOperatorRequired)
	case "":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: missing classification")
	default:
		return PreClaimIntakeResult{}, fmt.Errorf(
			"pre-claim intake: unknown classification %q: expected one of ready, needs_refine, needs_split, operator_required, system_unready",
			out.Classification,
		)
	}
}

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// LintHookErrorKind classifies infra failures returned by the pre-dispatch
// lint hook. The execute loop treats these as fail-open warnings rather than
// quality-policy results.
type LintHookErrorKind string

const (
	LintHookErrorKindMissingSkill    LintHookErrorKind = "missing-skill"
	LintHookErrorKindMissingHarness  LintHookErrorKind = "missing-harness"
	LintHookErrorKindEmptyOutput     LintHookErrorKind = "empty-output"
	LintHookErrorKindBadJSON         LintHookErrorKind = "bad-json"
	LintHookErrorKindCanceled        LintHookErrorKind = "canceled"
	LintHookErrorKindDispatchFailure LintHookErrorKind = "dispatch-failure"
)

// LintHookError is a typed infrastructure error returned by the lint hook.
// The Kind field is stable for errors.Is comparisons; Err carries the wrapped
// cause for logs and diagnostics.
type LintHookError struct {
	Kind LintHookErrorKind
	Err  error
}

func (e *LintHookError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return "lint hook: " + string(e.Kind)
	}
	return fmt.Sprintf("lint hook: %s: %v", e.Kind, e.Err)
}

func (e *LintHookError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *LintHookError) Is(target error) bool {
	t, ok := target.(*LintHookError)
	if !ok || e == nil || t == nil {
		return false
	}
	return e.Kind == t.Kind
}

var (
	ErrLintHookMissingSkill   = &LintHookError{Kind: LintHookErrorKindMissingSkill}
	ErrLintHookMissingHarness = &LintHookError{Kind: LintHookErrorKindMissingHarness}
	ErrLintHookEmptyOutput    = &LintHookError{Kind: LintHookErrorKindEmptyOutput}
	ErrLintHookBadJSON        = &LintHookError{Kind: LintHookErrorKindBadJSON}
	ErrLintHookCanceled       = &LintHookError{Kind: LintHookErrorKindCanceled}
	ErrLintHookDispatch       = &LintHookError{Kind: LintHookErrorKindDispatchFailure}
)

type lintPromptBead struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Type        string         `json:"type"`
	Labels      []string       `json:"labels,omitempty"`
	Parent      string         `json:"parent,omitempty"`
	Deps        []string       `json:"deps,omitempty"`
	Description string         `json:"description,omitempty"`
	Acceptance  string         `json:"acceptance,omitempty"`
	Notes       string         `json:"notes,omitempty"`
	Custom      map[string]any `json:"custom_fields,omitempty"`
}

// NewPreDispatchLintHook constructs the bead-lifecycle lint hook used before
// execute-loop dispatch.
func NewPreDispatchLintHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner) func(ctx context.Context, beadID string) (LintResult, error) {
	return func(ctx context.Context, beadID string) (LintResult, error) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return LintResult{}, &LintHookError{Kind: LintHookErrorKindCanceled, Err: err}
			}
		}
		if strings.TrimSpace(rcfg.Harness()) == "" {
			return LintResult{}, ErrLintHookMissingHarness
		}
		if !hasBeadLifecycleSkill(projectRoot) {
			return LintResult{}, ErrLintHookMissingSkill
		}
		if store == nil {
			return LintResult{}, &LintHookError{Kind: LintHookErrorKindDispatchFailure, Err: fmt.Errorf("bead reader required")}
		}

		b, err := store.Get(beadID)
		if err != nil {
			return LintResult{}, &LintHookError{Kind: LintHookErrorKindDispatchFailure, Err: fmt.Errorf("load bead %s: %w", beadID, err)}
		}

		if findings := escalation.LintExecutionHints(b); len(findings) > 0 {
			rationaleParts := make([]string, 0, len(findings))
			suggestedFixes := make([]string, 0, len(findings))
			for _, finding := range findings {
				rationaleParts = append(rationaleParts, finding.Message)
				suggestedFixes = append(suggestedFixes, finding.Message)
			}
			return LintResult{
				Score:          0,
				Rationale:      strings.Join(rationaleParts, "; "),
				SuggestedFixes: suggestedFixes,
			}, nil
		}

		prompt, err := buildPreDispatchLintPrompt(b)
		if err != nil {
			return LintResult{}, &LintHookError{Kind: LintHookErrorKindDispatchFailure, Err: err}
		}

		result, err := dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, AgentRunRuntime{
			Prompt:       prompt,
			WorkDir:      projectRoot,
			PromptSource: "bead-lifecycle-lint",
		})
		if err != nil {
			switch {
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return LintResult{}, &LintHookError{Kind: LintHookErrorKindCanceled, Err: err}
			case isUnknownHarnessError(err):
				return LintResult{}, &LintHookError{Kind: LintHookErrorKindMissingHarness, Err: err}
			case isMissingSkillError(err):
				return LintResult{}, &LintHookError{Kind: LintHookErrorKindMissingSkill, Err: err}
			default:
				return LintResult{}, &LintHookError{Kind: LintHookErrorKindDispatchFailure, Err: err}
			}
		}

		payload, parseErr := lintResultPayload(result)
		if parseErr != nil {
			return LintResult{}, parseErr
		}
		var out LintResult
		if err := json.Unmarshal([]byte(payload), &out); err != nil {
			return LintResult{}, &LintHookError{Kind: LintHookErrorKindBadJSON, Err: err}
		}
		return out, nil
	}
}

func buildPreDispatchLintPrompt(b *bead.Bead) (string, error) {
	env := lintPromptBead{
		ID:          b.ID,
		Title:       strings.TrimSpace(b.Title),
		Type:        b.IssueType,
		Labels:      append([]string(nil), b.Labels...),
		Parent:      strings.TrimSpace(b.Parent),
		Deps:        append([]string(nil), b.DepIDs()...),
		Description: strings.TrimSpace(b.Description),
		Acceptance:  strings.TrimSpace(b.Acceptance),
		Notes:       strings.TrimSpace(b.Notes),
	}
	if len(b.Extra) > 0 {
		env.Custom = make(map[string]any, len(b.Extra))
		for k, v := range b.Extra {
			env.Custom[k] = v
		}
	}

	body, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("MODE: lint\n")
	sb.WriteString("You are the bead-lifecycle skill. Score the bead below using lint mode and return exactly one JSON object matching LintResult.\n")
	sb.WriteString("Return only JSON with these top-level fields.\n")
	sb.WriteString("Required output shape example: {\"score\":0,\"rationale\":\"\",\"suggested_fixes\":[],\"waivers_applied\":[]}\n")
	sb.WriteString("Do not wrap the answer in markdown or prose.\n\n")
	sb.WriteString("```json\n")
	sb.Write(body)
	sb.WriteString("\n```\n")
	return sb.String(), nil
}

func lintResultPayload(result *Result) (string, error) {
	if result == nil {
		return "", &LintHookError{Kind: LintHookErrorKindEmptyOutput, Err: fmt.Errorf("nil result")}
	}
	text := strings.TrimSpace(result.CondensedOutput)
	if text == "" {
		text = strings.TrimSpace(result.Output)
	}
	if text == "" {
		return "", ErrLintHookEmptyOutput
	}
	candidate, ok := extractJSONCandidate(text)
	if !ok {
		return "", &LintHookError{Kind: LintHookErrorKindBadJSON, Err: fmt.Errorf("no JSON object found")}
	}
	return candidate, nil
}

func hasBeadLifecycleSkill(projectRoot string) bool {
	if strings.TrimSpace(projectRoot) == "" {
		return false
	}
	candidates := []string{
		filepath.Join(projectRoot, ".agents", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
		filepath.Join(projectRoot, ".claude", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func isUnknownHarnessError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unknown harness") || strings.Contains(s, "harness not found")
}

func isMissingSkillError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "skill missing") || strings.Contains(s, "bead-lifecycle")
}

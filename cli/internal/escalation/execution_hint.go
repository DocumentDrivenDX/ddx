package escalation

import (
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// ExecutionIntentSource identifies where a routing intent came from.
type ExecutionIntentSource string

const (
	ExecutionIntentSourceDefault     ExecutionIntentSource = "default"
	ExecutionIntentSourceHeuristic   ExecutionIntentSource = "heuristic"
	ExecutionIntentSourceBeadHint    ExecutionIntentSource = "bead_hint"
	ExecutionIntentSourceCLIPassthru ExecutionIntentSource = "cli"
)

// ExecutionHint captures the durable bead-level execution intent that DDx
// can infer from bead metadata without binding to a concrete harness/provider
// pin.
type ExecutionHint struct {
	Source              ExecutionIntentSource
	EstimatedDifficulty EstimatedDifficulty
	InferredPowerClass  PowerClass
	RejectedRoutePins   []string
}

// ExecutionHintFinding is a single lint finding emitted when bead metadata
// carries a durable route pin.
type ExecutionHintFinding struct {
	Field   string `json:"field"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

// ParseExecutionHint derives the bead's abstract execution intent from its
// durable metadata. Only triage.estimated_difficulty is recognized as a
// bead-level difficulty hint; labels such as kind, priority, or power do not
// affect routing intent.
func ParseExecutionHint(b *bead.Bead) ExecutionHint {
	if b == nil {
		return ExecutionHint{Source: ExecutionIntentSourceDefault}
	}

	hint := ExecutionHint{
		Source:             ExecutionIntentSourceDefault,
		InferredPowerClass: InferPowerClass(b),
	}
	if difficulty, ok := BeadEstimatedDifficulty(b); ok {
		hint.Source = ExecutionIntentSourceBeadHint
		hint.EstimatedDifficulty = difficulty
	}

	for _, raw := range b.Labels {
		if pin, ok := parseDurableRoutePinLabel(raw); ok {
			hint.RejectedRoutePins = append(hint.RejectedRoutePins, pin)
		}
	}
	if len(b.Extra) > 0 {
		for k, v := range b.Extra {
			if pin, ok := parseDurableRoutePinField(k, v); ok {
				hint.RejectedRoutePins = append(hint.RejectedRoutePins, pin)
			}
		}
	}

	return hint
}

// LintExecutionHints returns durable-routing findings for the bead. The lint
// surface is intentionally narrow: it rejects durable model/provider/harness
// pins that should stay on one-off CLI flags.
func LintExecutionHints(b *bead.Bead) []ExecutionHintFinding {
	if b == nil {
		return nil
	}
	hint := ParseExecutionHint(b)
	findings := make([]ExecutionHintFinding, 0, len(hint.RejectedRoutePins))

	for _, pin := range hint.RejectedRoutePins {
		findings = append(findings, durableRoutePinFinding(b.ID, pin))
	}

	return findings
}

func parseDurableRoutePinLabel(raw string) (string, bool) {
	l := strings.ToLower(strings.TrimSpace(raw))
	if l == "" {
		return "", false
	}
	switch {
	case strings.HasPrefix(l, "harness:"),
		strings.HasPrefix(l, "provider:"),
		strings.HasPrefix(l, "model:"):
		if idx := strings.IndexByte(l, ':'); idx > 0 {
			return l[:idx], true
		}
		return l, true
	default:
		return "", false
	}
}

func parseDurableRoutePinField(key string, value any) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(key))
	switch lower {
	case "harness", "agent-harness", "execution-harness", "try-harness",
		"provider", "agent-provider", "execution-provider", "try-provider",
		"model", "agent-model", "execution-model", "try-model":
		return renderDurableRoutePin(lower, value), true
	default:
		return "", false
	}
}

func renderDurableRoutePin(key string, value any) string {
	if value == nil {
		return key
	}
	if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
		return fmt.Sprintf("%s=%s", key, s)
	}
	return key
}

func durableRoutePinFinding(beadID, pin string) ExecutionHintFinding {
	field := pin
	if idx := strings.IndexByte(pin, '='); idx > 0 {
		field = pin[:idx]
	}
	var message string
	switch {
	case strings.Contains(field, "harness"):
		message = fmt.Sprintf("bead metadata contains %s; durable harness pins are not allowed. Use ddx try %s --harness ... for one-off debugging.", pin, beadID)
	case strings.Contains(field, "provider"):
		message = fmt.Sprintf("bead metadata contains %s; durable provider pins are not allowed. Use ddx try %s --provider ... for one-off debugging.", pin, beadID)
	case strings.Contains(field, "model"):
		message = fmt.Sprintf("bead metadata contains %s; durable model pins are not allowed. Use ddx try %s --model ... for one-off debugging.", pin, beadID)
	default:
		message = fmt.Sprintf("bead metadata contains %s; durable route pins are not allowed.", pin)
	}
	return ExecutionHintFinding{
		Field:   field,
		Value:   pin,
		Message: message,
	}
}

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
	Source             ExecutionIntentSource
	RequestedTier      ModelTier
	SmartJustification string
	RejectedRoutePins  []string
}

// ExecutionHintFinding is a single lint finding emitted when bead metadata
// carries a durable route pin or a missing smart-tier justification.
type ExecutionHintFinding struct {
	Field   string `json:"field"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

// ParseExecutionHint derives the bead's abstract execution intent from its
// durable metadata. A tier label wins over heuristics and marks the source as
// bead_hint; otherwise the caller should fall back to heuristics/defaults.
func ParseExecutionHint(b *bead.Bead) ExecutionHint {
	if b == nil {
		return ExecutionHint{Source: ExecutionIntentSourceDefault}
	}

	hint := ExecutionHint{
		Source:             ExecutionIntentSourceHeuristic,
		RequestedTier:      InferTier(b),
		SmartJustification: extractSmartJustification(b.Description),
	}
	if hint.RequestedTier == "" {
		hint.Source = ExecutionIntentSourceDefault
	}

	for _, raw := range b.Labels {
		tier, ok := parseTierLabel(raw)
		if ok {
			hint.Source = ExecutionIntentSourceBeadHint
			hint.RequestedTier = tier
		}
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

	// The smart-tier justification is only meaningful when the bead explicitly
	// asks for smart. Heuristic smart inference does not require the authored
	// SMART JUSTIFICATION section.
	if hint.RequestedTier != TierSmart {
		hint.SmartJustification = ""
	}
	return hint
}

// LintExecutionHints returns durable-routing findings for the bead. The lint
// surface is intentionally narrow: it rejects smart-tier beads without an
// explicit justification and durable model/provider/harness pins that should
// stay on one-off CLI flags.
func LintExecutionHints(b *bead.Bead) []ExecutionHintFinding {
	if b == nil {
		return nil
	}
	hint := ParseExecutionHint(b)
	findings := make([]ExecutionHintFinding, 0, len(hint.RejectedRoutePins)+1)

	if hint.Source == ExecutionIntentSourceBeadHint && hint.RequestedTier == TierSmart && strings.TrimSpace(hint.SmartJustification) == "" {
		findings = append(findings, ExecutionHintFinding{
			Field:   "description",
			Message: "bead uses tier:smart but has no SMART JUSTIFICATION section",
		})
	}

	for _, pin := range hint.RejectedRoutePins {
		findings = append(findings, durableRoutePinFinding(b.ID, pin))
	}

	return findings
}

func parseTierLabel(raw string) (ModelTier, bool) {
	l := strings.ToLower(strings.TrimSpace(raw))
	if l == "" {
		return "", false
	}
	tier, ok := strings.CutPrefix(l, "tier:")
	if !ok {
		return "", false
	}
	switch strings.TrimSpace(tier) {
	case string(TierSmart):
		return TierSmart, true
	case string(TierStandard):
		return TierStandard, true
	case string(TierCheap):
		return TierCheap, true
	default:
		return "", false
	}
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

var smartJustificationPrefix = "SMART JUSTIFICATION:"

func extractSmartJustification(description string) string {
	desc := strings.TrimSpace(description)
	if desc == "" {
		return ""
	}

	lines := strings.Split(desc, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != smartJustificationPrefix {
			continue
		}
		var b strings.Builder
		for _, next := range lines[i+1:] {
			trimmed := strings.TrimSpace(next)
			if trimmed == "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				continue
			}
			if isSectionHeading(trimmed) {
				break
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(trimmed)
		}
		return strings.TrimSpace(b.String())
	}

	return ""
}

func isSectionHeading(line string) bool {
	if !strings.HasSuffix(line, ":") {
		return false
	}
	head := strings.TrimSuffix(line, ":")
	if head == "" {
		return false
	}
	for _, r := range head {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == ' ', r == '_', r == '-', r == '/':
		default:
			return false
		}
	}
	return true
}

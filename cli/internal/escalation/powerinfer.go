package escalation

import (
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const triagePowerHintKey = "triage.power_hint"

// InferPowerClass returns a recommended PowerClass for a bead based on its metadata.
// It inspects, in priority order: an explicit "power:" label, the kind/priority
// labels and IssueType, then falls back to an estimated-scope heuristic on the
// description length.
//
// The default for a bead with no useful signal is PowerCheap, matching the
// hot-fix behavior in cmd/agent_cmd.go runAgentExecuteLoopImpl that this
// engine replaces. Aligns with the endpoint-first routing redesign goal of
// cheap-by-default with metadata-driven escalation.
func InferPowerClass(b *bead.Bead) PowerClass {
	if b == nil {
		return PowerCheap
	}
	if powerClass, ok := triagePowerHint(b); ok {
		return powerClass
	}

	kindLabel := ""
	priorityLabel := ""
	for _, raw := range b.Labels {
		l := strings.ToLower(strings.TrimSpace(raw))
		if l == "" {
			continue
		}
		// Explicit override wins.
		if powerClass, ok := parsePowerLabel(l); ok {
			return powerClass
		}
		if v, ok := strings.CutPrefix(l, "kind:"); ok {
			kindLabel = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(l, "priority:"); ok {
			priorityLabel = strings.TrimSpace(v)
		}
	}

	// Priority drives the floor: critical/high work gets smart, low/trivial
	// is happy at cheap. Medium falls through to kind- and scope-based logic.
	switch priorityLabel {
	case "critical", "urgent":
		return PowerSmart
	case "high":
		// High priority bugs/incidents go to smart; high enhancements stay
		// at standard so we don't burn smart-powerClass budget on routine work.
		if isBugKind(kindLabel) || isBugKind(b.IssueType) {
			return PowerSmart
		}
		return PowerStandard
	case "low", "trivial":
		return PowerCheap
	}

	kind := kindLabel
	if kind == "" {
		kind = strings.ToLower(strings.TrimSpace(b.IssueType))
	}
	switch {
	case isBugKind(kind):
		return PowerStandard
	case isCheapKind(kind):
		return PowerCheap
	case isStandardKind(kind):
		return PowerStandard
	}

	// Estimated scope from description length. Long, dense descriptions tend
	// to indicate larger work; short descriptions are typically mechanical.
	descLen := len(b.Description) + len(b.Acceptance)
	switch {
	case descLen >= 4000:
		return PowerSmart
	case descLen >= 1500:
		return PowerStandard
	default:
		return PowerCheap
	}
}

func triagePowerHint(b *bead.Bead) (PowerClass, bool) {
	if b == nil || b.Extra == nil {
		return "", false
	}
	raw, ok := b.Extra[triagePowerHintKey]
	if !ok {
		return "", false
	}
	return parsePowerValue(fmt.Sprint(raw))
}

func parsePowerValue(raw string) (PowerClass, bool) {
	l := strings.ToLower(strings.TrimSpace(raw))
	if l == "" {
		return "", false
	}
	if strings.HasPrefix(l, "power:") {
		return parsePowerLabel(l)
	}
	switch l {
	case string(PowerSmart):
		return PowerSmart, true
	case string(PowerStandard):
		return PowerStandard, true
	case string(PowerCheap):
		return PowerCheap, true
	default:
		return "", false
	}
}

func isBugKind(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bug", "fix", "incident", "regression", "defect":
		return true
	}
	return false
}

func isCheapKind(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "chore", "docs", "doc", "documentation", "cleanup", "typo", "rename":
		return true
	}
	return false
}

func isStandardKind(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "enhancement", "feature", "story", "task", "refactor", "test":
		return true
	}
	return false
}

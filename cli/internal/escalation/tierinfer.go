package escalation

import (
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const triageTierHintKey = "triage.tier_hint"

// InferTier returns a recommended ModelTier for a bead based on its metadata.
// It inspects, in priority order: an explicit "tier:" label, the kind/priority
// labels and IssueType, then falls back to an estimated-scope heuristic on the
// description length.
//
// The default for a bead with no useful signal is TierCheap, matching the
// hot-fix behavior in cmd/agent_cmd.go runAgentExecuteLoopImpl that this
// engine replaces. Aligns with the endpoint-first routing redesign goal of
// cheap-by-default with metadata-driven escalation.
func InferTier(b *bead.Bead) ModelTier {
	if b == nil {
		return TierCheap
	}
	if tier, ok := triageTierHint(b); ok {
		return tier
	}

	kindLabel := ""
	priorityLabel := ""
	for _, raw := range b.Labels {
		l := strings.ToLower(strings.TrimSpace(raw))
		if l == "" {
			continue
		}
		// Explicit override wins.
		if tier, ok := parseTierLabel(l); ok {
			return tier
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
		return TierSmart
	case "high":
		// High priority bugs/incidents go to smart; high enhancements stay
		// at standard so we don't burn smart-tier budget on routine work.
		if isBugKind(kindLabel) || isBugKind(b.IssueType) {
			return TierSmart
		}
		return TierStandard
	case "low", "trivial":
		return TierCheap
	}

	kind := kindLabel
	if kind == "" {
		kind = strings.ToLower(strings.TrimSpace(b.IssueType))
	}
	switch {
	case isBugKind(kind):
		return TierStandard
	case isCheapKind(kind):
		return TierCheap
	case isStandardKind(kind):
		return TierStandard
	}

	// Estimated scope from description length. Long, dense descriptions tend
	// to indicate larger work; short descriptions are typically mechanical.
	descLen := len(b.Description) + len(b.Acceptance)
	switch {
	case descLen >= 4000:
		return TierSmart
	case descLen >= 1500:
		return TierStandard
	default:
		return TierCheap
	}
}

func triageTierHint(b *bead.Bead) (ModelTier, bool) {
	if b == nil || b.Extra == nil {
		return "", false
	}
	raw, ok := b.Extra[triageTierHintKey]
	if !ok {
		return "", false
	}
	return parseTierValue(fmt.Sprint(raw))
}

func parseTierValue(raw string) (ModelTier, bool) {
	l := strings.ToLower(strings.TrimSpace(raw))
	if l == "" {
		return "", false
	}
	if strings.HasPrefix(l, "tier:") {
		return parseTierLabel(l)
	}
	switch l {
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

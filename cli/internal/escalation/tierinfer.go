package escalation

import (
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

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

	kindLabel := ""
	priorityLabel := ""
	for _, raw := range b.Labels {
		l := strings.ToLower(strings.TrimSpace(raw))
		if l == "" {
			continue
		}
		// Explicit override wins.
		if v, ok := strings.CutPrefix(l, "tier:"); ok {
			switch strings.TrimSpace(v) {
			case "smart":
				return TierSmart
			case "standard":
				return TierStandard
			case "cheap":
				return TierCheap
			}
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

// TierToProfile maps a ModelTier to the routing profile string consumed by
// agent.NormalizeRoutingProfile / LoadAndResolve. Tier names align with
// profile names in the built-in catalog (smart/standard/cheap).
func TierToProfile(t ModelTier) string {
	switch t {
	case TierSmart:
		return "smart"
	case TierStandard:
		return "standard"
	case TierCheap:
		return "cheap"
	}
	return "cheap"
}

package docprose

import (
	"sort"
	"strings"
)

// ddxRuleMeta carries the DDx-owned messaging for a normalized rule.
type ddxRuleMeta struct {
	RuleID        string
	Rationale     string
	SuggestedEdit string
}

// valeCheckToDDxRule maps the Vale check identifiers shipped in the initial
// rule pack to DDx-owned rule ids and messaging. The Vale check names are an
// implementation detail and must not leak into user-facing output.
var valeCheckToDDxRule = map[string]ddxRuleMeta{
	"DDx.UnsupportedClaim": {
		RuleID:        "prose.claim.unsupported",
		Rationale:     "The sentence uses an unsupported promotional claim instead of a concrete subject, mechanism, or measured effect.",
		SuggestedEdit: "Replace the claim with a concrete description of the actual change and its observable effect.",
	},
	"DDx.AISlop": {
		RuleID:        "prose.ai_slop.polish",
		Rationale:     "The sentence uses polished but empty AI-default phrasing instead of concrete detail.",
		SuggestedEdit: "Replace the polished phrasing with specific behavior, constraints, or evidence.",
	},
	"DDx.FillerTransition": {
		RuleID:        "prose.filler.transition",
		Rationale:     "The transition adds throat-clearing instead of information.",
		SuggestedEdit: "Remove the transition or replace it with a specific point.",
	},
	"DDx.MissingActorAction": {
		RuleID:        "prose.specificity.actor_action",
		Rationale:     "The sentence describes a capability without naming the actor, action, artifact, or boundary.",
		SuggestedEdit: "Name the actor, action, artifact, or boundary that makes the statement concrete.",
	},
	"DDx.TokenCost": {
		RuleID:        "prose.cost.filler",
		Rationale:     "The phrase increases token cost without naming the technical consequence.",
		SuggestedEdit: "Remove the filler or replace it with the concrete consequence.",
	},
	"DDx.RepeatedOpening": {
		RuleID:        "prose.structure.repeated_opening",
		Rationale:     "The same opening sentence shape repeats across adjacent paragraphs, reading like generated filler.",
		SuggestedEdit: "Keep the first instance and replace the duplicate with a distinct sentence that adds new information.",
	},
	"DDx.Vocabulary": {
		RuleID:        "prose.vocabulary.generic_substitute",
		Rationale:     "The sentence uses a generic substitute that hides the DDx-specific term for the concept.",
		SuggestedEdit: "Replace the generic substitute with the DDx-specific term that names the concept.",
	},
}

// NormalizeValeAlerts translates raw Vale alerts into DDx findings. It
// applies the DDx rule-id mapping, attaches DDx-owned rationale and
// suggested-edit metadata, and merges multiple word-level hits on the same
// (file, line) when they share the same underlying DDx rule.
//
// Vale check identifiers and Vale messages are intentionally dropped from
// the output so that user-facing findings stay free of Vale implementation
// details.
func NormalizeValeAlerts(alerts []ValeAlert) []Finding {
	type key struct {
		File   string
		Line   int
		RuleID string
	}
	seen := make(map[key]bool, len(alerts))
	findings := make([]Finding, 0, len(alerts))

	for _, alert := range alerts {
		meta, ok := valeCheckToDDxRule[alert.Check]
		if !ok {
			continue
		}
		k := key{File: alert.File, Line: alert.Line, RuleID: meta.RuleID}
		if seen[k] {
			continue
		}
		seen[k] = true

		severity := strings.TrimSpace(alert.Severity)
		if severity == "" {
			severity = "warning"
		}
		findings = append(findings, Finding{
			File:          alert.File,
			Line:          alert.Line,
			RuleID:        meta.RuleID,
			Severity:      severity,
			Rationale:     meta.Rationale,
			SuggestedEdit: meta.SuggestedEdit,
		})
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].RuleID < findings[j].RuleID
	})
	return findings
}

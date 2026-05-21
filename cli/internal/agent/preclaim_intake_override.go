package agent

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

func preClaimIntakeFindingFingerprint(candidate *bead.Bead, ruleID, reason, decisionSource, policyMode, decision, suggestedAction string) string {
	promptFingerprint := ""
	if candidate != nil {
		promptFingerprint = bead.PromptFingerprint(*candidate)
	}
	baseFingerprint := readinessDecisionFingerprint(ruleID, reason, decisionSource, policyMode, decision, suggestedAction)
	if baseFingerprint == "" && promptFingerprint == "" {
		return ""
	}
	return hashText(baseFingerprint + "\x00" + promptFingerprint)
}

func preClaimIntakeOverrideHonored(store ExecuteBeadLoopStore, candidate *bead.Bead, findingFingerprint string) (bool, error) {
	if store == nil || candidate == nil {
		return false, nil
	}
	findingFingerprint = strings.TrimSpace(findingFingerprint)
	if findingFingerprint == "" {
		return false, nil
	}
	events, err := store.Events(candidate.ID)
	if err != nil {
		return false, err
	}
	promptFingerprint := bead.PromptFingerprint(*candidate)
	latestTriaged := -1
	latestBlocked := -1
	for i, ev := range events {
		fields := eventBodyFields(ev.Body)
		switch ev.Kind {
		case "triaged":
			if strings.TrimSpace(fields["accepted_fingerprint"]) == findingFingerprint &&
				strings.TrimSpace(fields["accepted_prompt_fingerprint"]) == promptFingerprint {
				latestTriaged = i
			}
		case "intake.blocked", "pre_claim_intake.blocked":
			if strings.TrimSpace(fields["fingerprint"]) == findingFingerprint {
				latestBlocked = i
			}
		}
	}
	return latestTriaged >= 0 && latestTriaged > latestBlocked, nil
}

func eventBodyFields(body string) map[string]string {
	fields := map[string]string{}
	if strings.TrimSpace(body) == "" {
		return fields
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return fields
	}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			fields[k] = s
		}
	}
	return fields
}

func appendPreClaimIntakeOverrideHonoredEvent(store ExecuteBeadLoopStore, beadID, actor, findingFingerprint, promptFingerprint string, at time.Time) {
	if store == nil {
		return
	}
	body, _ := json.Marshal(map[string]any{
		"rule_id":                     "pre_claim_intake.operator_required",
		"decision_source":             "operator_override",
		"policy_mode":                 "override",
		"decision":                    "continue",
		"reason":                      "operator acceptance already covers the current readiness finding",
		"fingerprint":                 findingFingerprint,
		"prompt_fingerprint":          promptFingerprint,
		"accepted_fingerprint":        findingFingerprint,
		"accepted_prompt_fingerprint": promptFingerprint,
	})
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "pre_claim_intake.warn",
		Summary:   "operator_override_honored",
		Body:      string(body),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
}

func detectIntakeBlockedOperatorOverride(store ExecuteBeadLoopStore, candidate *bead.Bead, ruleID, reason, decisionSource, policyMode, decision, suggestedAction string) (bool, error) {
	if store == nil || candidate == nil {
		return false, nil
	}
	findingFingerprint := preClaimIntakeFindingFingerprint(candidate, ruleID, reason, decisionSource, policyMode, decision, suggestedAction)
	honored, err := preClaimIntakeOverrideHonored(store, candidate, findingFingerprint)
	return honored, err
}

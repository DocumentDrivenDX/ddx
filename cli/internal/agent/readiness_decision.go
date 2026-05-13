package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func readinessDecisionBody(ruleID, reason, decisionSource, policyMode, decision, suggestedAction string, extra map[string]any) map[string]any {
	body := make(map[string]any, 7+len(extra))
	add := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			body[key] = value
		}
	}
	add("rule_id", ruleID)
	add("reason", reason)
	add("decision_source", decisionSource)
	add("policy_mode", policyMode)
	add("decision", decision)
	add("suggested_action", suggestedAction)
	if fingerprint := readinessDecisionFingerprint(ruleID, reason, decisionSource, policyMode, decision, suggestedAction); fingerprint != "" {
		body["fingerprint"] = fingerprint
	}
	for k, v := range extra {
		if v != nil {
			body[k] = v
		}
	}
	return body
}

func readinessDecisionBodyJSON(ruleID, reason, decisionSource, policyMode, decision, suggestedAction string, extra map[string]any) string {
	body, _ := json.Marshal(readinessDecisionBody(ruleID, reason, decisionSource, policyMode, decision, suggestedAction, extra))
	return string(body)
}

func readinessDecisionFingerprint(ruleID, reason, decisionSource, policyMode, decision, suggestedAction string) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(ruleID)),
		strings.ToLower(strings.TrimSpace(reason)),
		strings.ToLower(strings.TrimSpace(decisionSource)),
		strings.ToLower(strings.TrimSpace(policyMode)),
		strings.ToLower(strings.TrimSpace(decision)),
		strings.ToLower(strings.TrimSpace(suggestedAction)),
	}
	if strings.Join(parts, "") == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

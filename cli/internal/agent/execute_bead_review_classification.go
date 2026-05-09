package agent

import "strings"

const (
	ReviewFindingClassFixableGap  = "review_fixable_gap"
	ReviewFindingClassMalfunction = "review_malfunction"
)

// ReviewFindingClassification is the strict retry/manual class derived from
// structured review evidence. Rationale is retained for operator context, but
// freeform rationale alone is not evidence and cannot make a result repairable.
type ReviewFindingClassification struct {
	Class                   string
	Evidence                []string
	AutomatedRepairEligible bool
	Reason                  string
}

// ClassifyReviewFindings maps a structured reviewer result to the FEAT-010
// review classes used as the policy boundary before repair or manual routing.
func ClassifyReviewFindings(res *ReviewResult) ReviewFindingClassification {
	if res == nil {
		return reviewClassification(ReviewFindingClassMalfunction, nil, "missing review result", false)
	}

	switch res.Verdict {
	case VerdictApprove:
		return ReviewFindingClassification{}
	case VerdictRequestChanges, VerdictBlock:
	default:
		return reviewClassification(ReviewFindingClassMalfunction, nil, "malformed review verdict", false)
	}

	evidence := reviewStructuredEvidence(res)
	if len(evidence) == 0 {
		return reviewClassification(ReviewFindingClassMalfunction, nil, "review lacks structured findings or per-AC evidence", false)
	}

	text := strings.ToLower(strings.Join(evidence, "\n"))
	switch {
	case containsAny(text, ReviewTerminalClassUnsafeOrOutScope, "unsafe", "out of scope", "out-of-scope", "outside scope", "non-scope", "forbidden", "scope change", "destructive"):
		return reviewClassification(ReviewTerminalClassUnsafeOrOutScope, evidence, "", false)
	case containsAny(text, ReviewTerminalClassTooLarge, "too large", "too broad", "decompose", "split this bead", "split the bead", "needs decomposition"):
		return reviewClassification(ReviewTerminalClassTooLarge, evidence, "", false)
	case containsAny(text, ReviewTerminalClassSpecGap, "spec gap", "specification gap", "contradictory", "contradiction", "ambiguous requirement", "requirements are unclear", "unclear requirement"):
		return reviewClassification(ReviewTerminalClassSpecGap, evidence, "", false)
	case containsAny(text, ReviewTerminalClassMissingAcceptance, "missing acceptance", "acceptance criteria missing", "acceptance criterion missing", "unverifiable acceptance", "acceptance is unverifiable"):
		return reviewClassification(ReviewTerminalClassMissingAcceptance, evidence, "", false)
	default:
		return reviewClassification(ReviewFindingClassFixableGap, evidence, "", true)
	}
}

func reviewClassification(class string, evidence []string, reason string, repair bool) ReviewFindingClassification {
	return ReviewFindingClassification{
		Class:                   class,
		Evidence:                append([]string(nil), evidence...),
		AutomatedRepairEligible: repair,
		Reason:                  strings.TrimSpace(reason),
	}
}

func reviewStructuredEvidence(res *ReviewResult) []string {
	if res == nil {
		return nil
	}
	var evidence []string
	for _, ac := range res.PerAC {
		if strings.TrimSpace(ac.Evidence) == "" {
			continue
		}
		parts := []string{ac.Item, ac.Grade, ac.Evidence}
		line := strings.TrimSpace(strings.Join(parts, " "))
		if line != "" {
			evidence = append(evidence, line)
		}
	}
	for _, finding := range res.Findings {
		summary := strings.TrimSpace(finding.Summary)
		if summary == "" {
			continue
		}
		if loc := strings.TrimSpace(finding.Location); loc != "" {
			summary = loc + ": " + summary
		}
		if sev := strings.TrimSpace(finding.Severity); sev != "" {
			summary = sev + ": " + summary
		}
		evidence = append(evidence, summary)
	}
	return evidence
}

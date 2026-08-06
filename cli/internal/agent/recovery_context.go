package agent

import (
	"regexp"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

type RecoveryReferenceContext struct {
	Parent        string   `json:"parent,omitempty"`
	RelatedDeps   []string `json:"related_deps,omitempty"`
	GoverningRefs []string `json:"governing_refs,omitempty"`
}

var (
	recoveryBeadIDPattern    = regexp.MustCompile(`ddx-[a-z0-9]+`)
	recoveryGoverningPattern = regexp.MustCompile(`(?:TD|FEAT)-\d+`)
)

func postLadderExhaustionContextFromReport(report ExecuteBeadReport) PostLadderExhaustionContext {
	return postLadderExhaustionContextFromReportAndBead(report, nil)
}

func postLadderExhaustionContextFromReportAndBead(report ExecuteBeadReport, b *bead.Bead) PostLadderExhaustionContext {
	ctx := PostLadderExhaustionContext{
		ResultRev:            strings.TrimSpace(report.ResultRev),
		CandidateRef:         strings.TrimSpace(report.CandidateRef),
		PreserveRef:          strings.TrimSpace(report.PreserveRef),
		ReviewGroupID:        strings.TrimSpace(report.ReviewGroupID),
		ReviewVerdict:        strings.TrimSpace(report.ReviewVerdict),
		ReviewRationale:      strings.TrimSpace(report.ReviewRationale),
		ReviewClassification: strings.TrimSpace(report.ReviewClassification),
		ReviewPerAC:          append([]ReviewAC(nil), report.ReviewPerAC...),
		ReviewFindings:       append([]Finding(nil), report.ReviewFindings...),
	}
	if b != nil {
		ctx.RecoveryReferenceContext = recoveryReferenceContextFromBead(b)
	}
	return ctx
}

func recoveryReferenceContextFromBead(b *bead.Bead) RecoveryReferenceContext {
	if b == nil {
		return RecoveryReferenceContext{}
	}
	desc := strings.TrimSpace(b.Description)
	out := RecoveryReferenceContext{}
	out.Parent = firstMatchingRecoveryRef(recoverySection(desc, "PARENT"), recoveryBeadIDPattern)
	out.RelatedDeps = uniqueRecoveryRefs(recoveryIDs(recoverySection(desc, "DEPS"), recoveryBeadIDPattern))
	out.GoverningRefs = uniqueRecoveryRefs(recoveryIDs(recoverySection(desc, "GOVERNING"), recoveryGoverningPattern))
	return out
}

func recoverySection(text, marker string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != marker {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			line := strings.TrimSpace(lines[j])
			if line != "" {
				return line
			}
		}
		return ""
	}
	return ""
}

func recoveryIDs(text string, pattern *regexp.Regexp) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	matches := pattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	return uniqueRecoveryRefs(matches)
}

func firstMatchingRecoveryRef(text string, pattern *regexp.Regexp) string {
	for _, match := range pattern.FindAllString(text, -1) {
		if match != "" {
			return match
		}
	}
	return ""
}

func uniqueRecoveryRefs(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

package agent

import (
	"fmt"
	"strings"
)

// RepairPromptInput carries the evidence needed to ask an implementer to make
// one append-only repair commit in the still-live attempt worktree.
type RepairPromptInput struct {
	BeadID               string
	BaseRev              string
	FailedCandidateRev   string
	CycleIndex           int
	ReviewRationale      string
	PerAC                []ReviewAC
	Findings             []Finding
	VerificationCommands []string
}

// BuildRepairPrompt assembles the same-worktree repair prompt. The prompt is
// intentionally policy-heavy: repair passes may add one commit, but must never
// rewrite the existing implementation history.
func BuildRepairPrompt(input RepairPromptInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repair bead %s in the current attempt worktree.\n\n", input.BeadID)
	fmt.Fprintf(&b, "base_rev: %s\n", input.BaseRev)
	fmt.Fprintf(&b, "failed_candidate_rev: %s\n", input.FailedCandidateRev)
	fmt.Fprintf(&b, "repair_cycle_index: %d\n\n", input.CycleIndex)

	b.WriteString("Review findings:\n")
	if strings.TrimSpace(input.ReviewRationale) != "" {
		fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(input.ReviewRationale))
	}
	for _, finding := range input.Findings {
		line := strings.TrimSpace(finding.Summary)
		if line == "" {
			continue
		}
		if loc := strings.TrimSpace(finding.Location); loc != "" {
			line = loc + ": " + line
		}
		if sev := strings.TrimSpace(finding.Severity); sev != "" {
			line = sev + ": " + line
		}
		fmt.Fprintf(&b, "- %s\n", line)
	}

	b.WriteString("\nPer-AC failures:\n")
	for _, ac := range input.PerAC {
		if strings.TrimSpace(ac.Evidence) == "" && strings.TrimSpace(ac.Grade) == "" {
			continue
		}
		fmt.Fprintf(&b, "- AC#%d %s %s %s\n", ac.Number, strings.TrimSpace(ac.Item), strings.TrimSpace(ac.Grade), strings.TrimSpace(ac.Evidence))
	}

	b.WriteString("\nVerification commands:\n")
	for _, cmd := range input.VerificationCommands {
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(cmd))
	}

	b.WriteString("\nRules:\n")
	b.WriteString("- Work in the existing attempt worktree only.\n")
	b.WriteString("- Make exactly one append-only repair commit on top of the failed candidate.\n")
	b.WriteString("- Do not reset, amend, squash, rebase, cherry-pick over, or otherwise rewrite history.\n")
	b.WriteString("- Limit changes to the reviewer findings and the bead acceptance criteria.\n")
	return b.String()
}

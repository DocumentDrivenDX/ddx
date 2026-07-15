package server

import (
	"fmt"
	"html"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// ReviewPromptFinding captures an unresolved finding that should be pinned
// to the tail of a review-session prompt.
type ReviewPromptFinding struct {
	Location string
	Summary  string
}

// ReviewPromptRenderInput describes the structured evidence envelope used to
// render a review-session prompt.
type ReviewPromptRenderInput struct {
	Session ReviewSession

	FirstUserIntent       string
	ExplicitUserDecisions []string
	SessionMemorySummary  string
	LastVerbatimTurns     int
	UnresolvedFindings    []ReviewPromptFinding
	MaxPromptBytes        int
	MaxPromptConfigured   bool
}

// ReviewPromptRenderResult is the structured output of RenderReviewPrompt.
type ReviewPromptRenderResult struct {
	Prompt   string
	Overflow bool
	Sections []evidence.EvidenceAssemblySection
}

// PromptBudgetExceededError is returned when the pinned floor cannot fit in
// the caller's prompt budget.
type PromptBudgetExceededError struct {
	ObservedBytes int
	CapBytes      int
}

func (e *PromptBudgetExceededError) Error() string {
	return fmt.Sprintf(
		"PROMPT_BUDGET_EXCEEDED: pinned floor observed %d bytes exceeds cap %d bytes",
		e.ObservedBytes,
		e.CapBytes,
	)
}

// RenderReviewPrompt assembles a structured review-session prompt with
// pinned sections first, then rolling session memory, then unresolved
// findings.
func RenderReviewPrompt(input ReviewPromptRenderInput) (ReviewPromptRenderResult, error) {
	max := input.MaxPromptBytes
	if !input.MaxPromptConfigured && max <= 0 {
		max = evidence.DefaultMaxPromptBytes
	}

	pinned := renderPinnedReviewPrompt(input)
	if len(pinned) > max {
		return ReviewPromptRenderResult{}, &PromptBudgetExceededError{
			ObservedBytes: len(pinned),
			CapBytes:      max,
		}
	}

	rolling := renderRollingReviewPrompt(input)
	unresolved := renderUnresolvedFindingsPrompt(input.UnresolvedFindings)

	assembled := evidence.AssembleInline([]evidence.SectionInput{
		{
			Name:     "pinned",
			Content:  pinned,
			MinFloor: true,
		},
		{
			Name:    "rolling",
			Content: rolling,
		},
		{
			Name:    "unresolved-findings",
			Content: unresolved,
		},
	}, max)

	return ReviewPromptRenderResult{
		Prompt:   assembled.Prompt,
		Overflow: assembled.Overflow,
		Sections: assembled.Sections,
	}, nil
}

func renderPinnedReviewPrompt(input ReviewPromptRenderInput) string {
	session := input.Session
	var sb strings.Builder
	sb.WriteString("<pinned>\n")

	sb.WriteString("  <artifact-identity>\n")
	fmt.Fprintf(&sb, "    <artifact-id>%s</artifact-id>\n", html.EscapeString(session.ArtifactID))
	fmt.Fprintf(&sb, "    <artifact-sha>%s</artifact-sha>\n", html.EscapeString(session.ArtifactSHA))
	fmt.Fprintf(&sb, "    <artifact-git-rev>%s</artifact-git-rev>\n", html.EscapeString(session.ArtifactGitRev))
	sb.WriteString("  </artifact-identity>\n")

	sb.WriteString("  <system-rubric>\n")
	if strings.TrimSpace(session.SystemRubric) == "" {
		sb.WriteString("    <empty/>\n")
	} else {
		sb.WriteString("    ")
		sb.WriteString(html.EscapeString(strings.TrimSpace(session.SystemRubric)))
		sb.WriteString("\n")
	}
	sb.WriteString("  </system-rubric>\n")

	sb.WriteString("  <story-17-template-prompt>\n")
	fmt.Fprintf(&sb, "    <template-ref>%s</template-ref>\n", html.EscapeString(session.TemplateRef))
	fmt.Fprintf(&sb, "    <prompt-ref>%s</prompt-ref>\n", html.EscapeString(session.PromptRef))
	sb.WriteString("  </story-17-template-prompt>\n")

	firstIntent := strings.TrimSpace(input.FirstUserIntent)
	if firstIntent == "" {
		firstIntent = firstUserTurnContent(session.Turns)
	}
	sb.WriteString("  <first-user-intent>\n")
	if firstIntent == "" {
		sb.WriteString("    <empty/>\n")
	} else {
		sb.WriteString("    ")
		sb.WriteString(html.EscapeString(firstIntent))
		sb.WriteString("\n")
	}
	sb.WriteString("  </first-user-intent>\n")

	decisions := input.ExplicitUserDecisions
	if len(decisions) == 0 {
		decisions = explicitUserDecisions(session.Turns)
	}
	sb.WriteString("  <explicit-user-decisions>\n")
	if len(decisions) == 0 {
		sb.WriteString("    <empty/>\n")
	} else {
		for _, decision := range decisions {
			decision = strings.TrimSpace(decision)
			if decision == "" {
				continue
			}
			sb.WriteString("    <decision>")
			sb.WriteString(html.EscapeString(decision))
			sb.WriteString("</decision>\n")
		}
	}
	sb.WriteString("  </explicit-user-decisions>\n")

	sb.WriteString("</pinned>\n")
	return sb.String()
}

func renderRollingReviewPrompt(input ReviewPromptRenderInput) string {
	var sb strings.Builder
	sb.WriteString("<rolling>\n")

	sb.WriteString("  <session-memory-summary>\n")
	summary := strings.TrimSpace(input.SessionMemorySummary)
	if summary == "" {
		summary = defaultSessionMemorySummary(input.Session)
	}
	if summary == "" {
		sb.WriteString("    <empty/>\n")
	} else {
		sb.WriteString("    ")
		sb.WriteString(html.EscapeString(summary))
		sb.WriteString("\n")
	}
	sb.WriteString("  </session-memory-summary>\n")

	turns := input.Session.Turns
	limit := input.LastVerbatimTurns
	if limit <= 0 || limit > len(turns) {
		limit = len(turns)
	}
	sb.WriteString("  <last-verbatim-turns>\n")
	if limit == 0 {
		sb.WriteString("    <empty/>\n")
	} else {
		start := len(turns) - limit
		for _, turn := range turns[start:] {
			fmt.Fprintf(&sb, "    <turn actor=%q>\n", html.EscapeString(turn.Actor))
			if strings.TrimSpace(turn.Content) == "" {
				sb.WriteString("      <empty/>\n")
			} else {
				sb.WriteString("      ")
				sb.WriteString(html.EscapeString(strings.TrimSpace(turn.Content)))
				sb.WriteString("\n")
			}
			sb.WriteString("    </turn>\n")
		}
	}
	sb.WriteString("  </last-verbatim-turns>\n")

	sb.WriteString("</rolling>\n")
	return sb.String()
}

func renderUnresolvedFindingsPrompt(findings []ReviewPromptFinding) string {
	var sb strings.Builder
	sb.WriteString("<unresolved-findings>\n")
	if len(findings) == 0 {
		sb.WriteString("  <empty/>\n")
	} else {
		for _, finding := range findings {
			sb.WriteString("  <finding>\n")
			if strings.TrimSpace(finding.Location) != "" {
				fmt.Fprintf(&sb, "    <location>%s</location>\n", html.EscapeString(strings.TrimSpace(finding.Location)))
			}
			if strings.TrimSpace(finding.Summary) != "" {
				fmt.Fprintf(&sb, "    <summary>%s</summary>\n", html.EscapeString(strings.TrimSpace(finding.Summary)))
			}
			sb.WriteString("  </finding>\n")
		}
	}
	sb.WriteString("</unresolved-findings>\n")
	return sb.String()
}

func firstUserTurnContent(turns []ReviewTurn) string {
	for _, turn := range turns {
		if strings.EqualFold(strings.TrimSpace(turn.Actor), "user") {
			return strings.TrimSpace(turn.Content)
		}
	}
	return ""
}

func explicitUserDecisions(turns []ReviewTurn) []string {
	seenFirst := false
	decisions := make([]string, 0)
	for _, turn := range turns {
		if !strings.EqualFold(strings.TrimSpace(turn.Actor), "user") {
			continue
		}
		if !seenFirst {
			seenFirst = true
			continue
		}
		content := strings.TrimSpace(turn.Content)
		if content != "" {
			decisions = append(decisions, content)
		}
	}
	return decisions
}

func defaultSessionMemorySummary(session ReviewSession) string {
	if len(session.Turns) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"%d turns across session %s (%s)",
		len(session.Turns),
		strings.TrimSpace(session.ID),
		strings.TrimSpace(session.Status),
	)
}

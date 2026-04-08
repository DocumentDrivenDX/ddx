package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GradeOptions configures a grading invocation.
type GradeOptions struct {
	Grader string // harness to use for grading
	Rubric string // custom rubric text (replaces default)
}

const defaultGradingRubric = `You are evaluating agent outputs for correctness, quality, and completeness.

For each arm, provide a JSON grade with:
- "arm": the harness name
- "score": integer 0-10
- "max_score": 10
- "pass": true if score >= 7
- "rationale": brief explanation

Respond with ONLY a JSON object: {"arms": [<grade>, ...]}`

// Grade sends a comparison record to a grading harness and returns
// structured grades per arm.
func (r *Runner) Grade(record *ComparisonRecord, opts GradeOptions) ([]ComparisonGrade, error) {
	if opts.Grader == "" {
		return nil, fmt.Errorf("agent: grader harness is required")
	}

	// Build the grading prompt
	prompt := buildGradingPrompt(record, opts.Rubric)

	// Run the grading harness
	result, err := r.Run(RunOptions{
		Harness: opts.Grader,
		Prompt:  prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: grading failed: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("agent: grader returned exit code %d: %s", result.ExitCode, result.Error)
	}

	// Parse the grading response
	grades, err := parseGrades(result.Output)
	if err != nil {
		return nil, err
	}

	return grades, nil
}

// buildGradingPrompt constructs the prompt sent to the grading harness.
func buildGradingPrompt(record *ComparisonRecord, customRubric string) string {
	var b strings.Builder

	// Rubric
	rubric := defaultGradingRubric
	if customRubric != "" {
		rubric = customRubric
	}
	b.WriteString(rubric)
	b.WriteString("\n\n")

	// Task
	b.WriteString("## Task\n\n")
	b.WriteString(record.Prompt)
	b.WriteString("\n\n")

	// Arms
	for i, arm := range record.Arms {
		b.WriteString(fmt.Sprintf("## Arm %d: %s\n\n", i+1, arm.Harness))

		b.WriteString("### Output\n\n")
		b.WriteString(arm.Output)
		b.WriteString("\n\n")

		if arm.Diff != "" {
			b.WriteString("### Changes (diff)\n\n```diff\n")
			b.WriteString(arm.Diff)
			b.WriteString("\n```\n\n")
		}
	}

	b.WriteString("Grade the following comparison arms. Respond with JSON only.")
	return b.String()
}

// parseGrades extracts ComparisonGrade values from the grader output.
func parseGrades(output string) ([]ComparisonGrade, error) {
	// Try to parse the whole output as the grade envelope
	var envelope struct {
		Arms []ComparisonGrade `json:"arms"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err == nil && len(envelope.Arms) > 0 {
		return envelope.Arms, nil
	}

	// Try to find JSON in the output (grader may include preamble)
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start >= 0 && end > start {
		substr := output[start : end+1]
		if err := json.Unmarshal([]byte(substr), &envelope); err == nil && len(envelope.Arms) > 0 {
			return envelope.Arms, nil
		}
	}

	return nil, fmt.Errorf("agent: failed to parse grading response as JSON")
}

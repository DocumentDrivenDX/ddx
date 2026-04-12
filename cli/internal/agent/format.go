package agent

import (
	"fmt"
	"strings"
	"text/tabwriter"
)

// FormatComparisonTable formats a ComparisonRecord as an ASCII table.
func FormatComparisonTable(record *ComparisonRecord) string {
	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)

	// Header
	fmt.Fprintln(w, "Arm\tStatus\tModel\tTokens (in/out)\tCost\tDuration")
	fmt.Fprintln(w, "---\t------\t-----\t---------------\t----\t--------")

	var totalInputTokens, totalOutputTokens int
	var totalCost float64
	var totalDurationMS int
	armCount := 0

	for _, arm := range record.Arms {
		status := "OK"
		if arm.ExitCode != 0 || arm.Error != "" {
			status = "FAIL"
		}

		armLabel := fmt.Sprintf("%s/%s", arm.Harness, arm.Model)
		tokensStr := fmt.Sprintf("%d/%d", arm.InputTokens, arm.OutputTokens)
		costStr := fmt.Sprintf("$%.6f", arm.CostUSD)
		durationStr := fmt.Sprintf("%dms", arm.DurationMS)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", armLabel, status, arm.Model, tokensStr, costStr, durationStr)

		totalInputTokens += arm.InputTokens
		totalOutputTokens += arm.OutputTokens
		totalCost += arm.CostUSD
		totalDurationMS += arm.DurationMS
		armCount++
	}

	// Total row
	if armCount > 0 {
		avgDuration := totalDurationMS / armCount
		tokensStr := fmt.Sprintf("%d/%d", totalInputTokens, totalOutputTokens)
		costStr := fmt.Sprintf("$%.6f", totalCost)
		durationStr := fmt.Sprintf("%dms (avg)", avgDuration)

		fmt.Fprintf(w, "TOTAL\t-\t-\t%s\t%s\t%s\n", tokensStr, costStr, durationStr)
	}

	_ = w.Flush()
	return sb.String()
}

// FormatGradeTable formats the grades from a ComparisonRecord as an ASCII table.
func FormatGradeTable(record *ComparisonRecord) string {
	if len(record.Grades) == 0 {
		return ""
	}

	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)

	// Header
	fmt.Fprintln(w, "Arm\tScore\tPass\tRationale")
	fmt.Fprintln(w, "---\t-----\t----\t---------")

	for _, grade := range record.Grades {
		scoreStr := fmt.Sprintf("%d/%d", grade.Score, grade.MaxScore)
		passStr := "no"
		if grade.Pass {
			passStr = "yes"
		}

		rationale := grade.Rationale
		if len(rationale) > 60 {
			rationale = rationale[:57] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", grade.Arm, scoreStr, passStr, rationale)
	}

	_ = w.Flush()
	return sb.String()
}

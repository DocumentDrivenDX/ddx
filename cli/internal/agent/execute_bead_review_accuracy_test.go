package agent

import (
	"strings"
	"testing"
)

// TestReviewAccuracy_ACCheckDisagreement_RecordedInMetrics verifies that
// countACGradeMismatches correctly detects when the reviewer's per-AC grade
// contradicts the ac-check.json mechanical result, and that the ReviewACOverride
// event kind constant is defined.
func TestReviewAccuracy_ACCheckDisagreement_RecordedInMetrics(t *testing.T) {
	t.Run("reviewer passes where ac-check fails", func(t *testing.T) {
		acCheckJSON := `{
			"schema_version": 1,
			"bead_id": "ddx-test",
			"items": [
				{"ac": 1, "kind": "test-name", "result": "fail", "evidence": "test not found"},
				{"ac": 2, "kind": "test-name", "result": "pass", "evidence": "TestFoo found"}
			]
		}`
		perAC := []ReviewAC{
			{Number: 1, Grade: "pass", Evidence: "TestFoo passed"},
			{Number: 2, Grade: "pass", Evidence: "TestBar passed"},
		}

		count, reasons := countACGradeMismatches(acCheckJSON, perAC)
		if count != 1 {
			t.Errorf("expected 1 mismatch (ac1), got %d", count)
		}
		if len(reasons) != 1 {
			t.Fatalf("expected 1 reason, got %d: %v", len(reasons), reasons)
		}
		if !strings.Contains(reasons[0], "ac1") {
			t.Errorf("expected reason to mention ac1, got: %s", reasons[0])
		}
		if !strings.Contains(reasons[0], "ac_check=fail") {
			t.Errorf("expected reason to contain ac_check=fail, got: %s", reasons[0])
		}
		if !strings.Contains(reasons[0], "reviewer=pass") {
			t.Errorf("expected reason to contain reviewer=pass, got: %s", reasons[0])
		}
	})

	t.Run("reviewer fails where ac-check passes", func(t *testing.T) {
		acCheckJSON := `{
			"items": [
				{"ac": 1, "result": "pass", "evidence": "TestFoo found"},
				{"ac": 2, "result": "pass", "evidence": "TestBar found"}
			]
		}`
		perAC := []ReviewAC{
			{Number: 1, Grade: "fail", Evidence: "test evidence looks fabricated"},
			{Number: 2, Grade: "pass", Evidence: "TestBar confirmed"},
		}

		count, reasons := countACGradeMismatches(acCheckJSON, perAC)
		if count != 1 {
			t.Errorf("expected 1 mismatch (ac1), got %d", count)
		}
		if len(reasons) == 0 {
			t.Fatal("expected at least one reason")
		}
		if !strings.Contains(reasons[0], "ac_check=pass") {
			t.Errorf("expected ac_check=pass in reason, got: %s", reasons[0])
		}
		if !strings.Contains(reasons[0], "reviewer=fail") {
			t.Errorf("expected reviewer=fail in reason, got: %s", reasons[0])
		}
	})

	t.Run("no mismatch when reviewer and ac-check agree", func(t *testing.T) {
		acCheckJSON := `{
			"items": [
				{"ac": 1, "result": "pass", "evidence": "TestFoo found"},
				{"ac": 2, "result": "fail", "evidence": "TestBar not found"}
			]
		}`
		perAC := []ReviewAC{
			{Number: 1, Grade: "pass", Evidence: "confirmed"},
			{Number: 2, Grade: "fail", Evidence: "missing"},
		}

		count, _ := countACGradeMismatches(acCheckJSON, perAC)
		if count != 0 {
			t.Errorf("expected 0 mismatches when verdicts agree, got %d", count)
		}
	})

	t.Run("needs_judgment mechanical result is not a mismatch", func(t *testing.T) {
		acCheckJSON := `{
			"items": [
				{"ac": 1, "result": "needs_judgment", "evidence": "cannot determine mechanically"}
			]
		}`
		perAC := []ReviewAC{
			{Number: 1, Grade: "pass", Evidence: "reviewer adjudicated"},
		}

		count, _ := countACGradeMismatches(acCheckJSON, perAC)
		if count != 0 {
			t.Errorf("expected 0 mismatches for needs_judgment ac-check result, got %d", count)
		}
	})

	t.Run("empty ac-check JSON returns no mismatches", func(t *testing.T) {
		count, reasons := countACGradeMismatches("", []ReviewAC{{Number: 1, Grade: "pass"}})
		if count != 0 || len(reasons) != 0 {
			t.Errorf("expected no mismatches for empty ac-check, got count=%d reasons=%v", count, reasons)
		}
	})

	t.Run("ReviewACOverrideEventKind constant is defined", func(t *testing.T) {
		if ReviewACOverrideEventKind == "" {
			t.Error("ReviewACOverrideEventKind must be a non-empty string")
		}
		if ReviewACOverrideEventKind != "review-ac-override" {
			t.Errorf("ReviewACOverrideEventKind = %q, want %q", ReviewACOverrideEventKind, "review-ac-override")
		}
	})

	t.Run("multiple mismatches returns all reasons", func(t *testing.T) {
		acCheckJSON := `{
			"items": [
				{"ac": 1, "result": "fail", "evidence": "not found"},
				{"ac": 2, "result": "fail", "evidence": "not found"},
				{"ac": 3, "result": "pass", "evidence": "found"}
			]
		}`
		perAC := []ReviewAC{
			{Number: 1, Grade: "pass"},
			{Number: 2, Grade: "pass"},
			{Number: 3, Grade: "pass"},
		}

		count, reasons := countACGradeMismatches(acCheckJSON, perAC)
		if count != 2 {
			t.Errorf("expected 2 mismatches, got %d", count)
		}
		if len(reasons) != 2 {
			t.Errorf("expected 2 reasons, got %d: %v", len(reasons), reasons)
		}
	})
}

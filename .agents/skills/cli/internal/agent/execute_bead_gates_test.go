package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// writeGateDoc writes an execution document with the flat execution: format.
func writeGateDoc(t *testing.T, dir, id, artifactID string, required bool, command []string) {
	t.Helper()
	path := filepath.Join(dir, "docs", "exec", id+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	cmdLines := ""
	for _, c := range command {
		cmdLines += "      - " + c + "\n"
	}
	reqStr := "false"
	if required {
		reqStr = "true"
	}
	content := "---\nddx:\n  id: " + id + "\n  depends_on:\n    - " + artifactID +
		"\n  execution:\n    kind: command\n    required: " + reqStr + "\n    command:\n" + cmdLines + "---\n# " + id + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeArtifactDoc writes a minimal governing artifact document.
func writeArtifactDoc(t *testing.T, dir, id string) {
	t.Helper()
	path := filepath.Join(dir, "docs", "specs", id+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nddx:\n  id: " + id + "\n---\n# " + id + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateRequiredGates_NoGates(t *testing.T) {
	dir := t.TempDir()
	writeArtifactDoc(t, dir, "FEAT-TEST")

	results, anyFailed, _, err := evaluateRequiredGates(dir, []string{"FEAT-TEST"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anyFailed {
		t.Error("expected no failure when no gates defined")
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestEvaluateRequiredGates_PassingGate(t *testing.T) {
	dir := t.TempDir()
	writeArtifactDoc(t, dir, "FEAT-PASS")
	writeGateDoc(t, dir, "exec.FEAT-PASS.smoke", "FEAT-PASS", true, []string{"sh", "-c", "exit 0"})

	results, anyFailed, _, err := evaluateRequiredGates(dir, []string{"FEAT-PASS"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anyFailed {
		t.Error("expected no failure for passing gate")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "pass" {
		t.Errorf("expected pass, got %q", results[0].Status)
	}
}

func TestEvaluateRequiredGates_FailingGate(t *testing.T) {
	dir := t.TempDir()
	writeArtifactDoc(t, dir, "FEAT-FAIL")
	writeGateDoc(t, dir, "exec.FEAT-FAIL.smoke", "FEAT-FAIL", true, []string{"sh", "-c", "exit 1"})

	results, anyFailed, _, err := evaluateRequiredGates(dir, []string{"FEAT-FAIL"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !anyFailed {
		t.Error("expected anyFailed=true for failing required gate")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "fail" {
		t.Errorf("expected fail, got %q", results[0].Status)
	}
}

func TestEvaluateRequiredGates_OptionalGateNotRun(t *testing.T) {
	dir := t.TempDir()
	writeArtifactDoc(t, dir, "FEAT-OPT")
	// required=false gate — should not be included in results
	writeGateDoc(t, dir, "exec.FEAT-OPT.optional", "FEAT-OPT", false, []string{"sh", "-c", "exit 1"})

	results, anyFailed, _, err := evaluateRequiredGates(dir, []string{"FEAT-OPT"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anyFailed {
		t.Error("optional gate should not cause failure")
	}
	if len(results) != 0 {
		t.Errorf("optional gate should not produce results, got %d", len(results))
	}
}

func TestEvaluateRequiredGates_UnrelatedGateIgnored(t *testing.T) {
	dir := t.TempDir()
	writeArtifactDoc(t, dir, "FEAT-A")
	writeArtifactDoc(t, dir, "FEAT-B")
	// Gate linked to FEAT-B only — should not run for FEAT-A
	writeGateDoc(t, dir, "exec.FEAT-B.smoke", "FEAT-B", true, []string{"sh", "-c", "exit 1"})

	results, anyFailed, _, err := evaluateRequiredGates(dir, []string{"FEAT-A"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anyFailed {
		t.Error("gate for different artifact should not affect FEAT-A")
	}
	if len(results) != 0 {
		t.Errorf("unrelated gate should not produce results, got %d", len(results))
	}
}

func TestEvaluateRequiredGates_EmptyGoverningIDs(t *testing.T) {
	dir := t.TempDir()
	results, anyFailed, _, err := evaluateRequiredGates(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anyFailed || len(results) != 0 {
		t.Error("empty governing IDs should produce no results")
	}
}

func TestSummarizeGates(t *testing.T) {
	tests := []struct {
		name     string
		results  []GateCheckResult
		failed   bool
		expected string
	}{
		{"no gates", nil, false, "skipped"},
		{"pass", []GateCheckResult{{Status: "pass"}}, false, "pass"},
		{"fail", []GateCheckResult{{Status: "fail"}}, true, "fail"},
		{"mixed fail", []GateCheckResult{{Status: "pass"}, {Status: "fail"}}, true, "fail"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeGates(tc.results, tc.failed)
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

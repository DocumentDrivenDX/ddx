package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRatchetGateDoc writes an execution document that declares a ratchet
// threshold. The gate command is expected to emit a numeric observation on
// stdout; the landing gate parses and compares against the ratchet before
// allowing the attempt to merge.
func writeRatchetGateDoc(t *testing.T, dir, id, artifactID string, command []string, comparison string, ratchet float64, unit, metricID string) {
	t.Helper()
	path := filepath.Join(dir, "docs", "exec", id+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("---\nddx:\n  id: ")
	sb.WriteString(id)
	sb.WriteString("\n  depends_on:\n    - ")
	sb.WriteString(artifactID)
	sb.WriteString("\n  execution:\n    kind: command\n    required: true\n    command:\n")
	for _, c := range command {
		sb.WriteString("      - ")
		sb.WriteString(c)
		sb.WriteString("\n")
	}
	if comparison != "" {
		sb.WriteString("    comparison: " + comparison + "\n")
	}
	sb.WriteString("    thresholds:\n")
	fmt.Fprintf(&sb, "      ratchet: %g\n", ratchet)
	if unit != "" {
		sb.WriteString("      unit: " + unit + "\n")
	}
	if metricID != "" {
		sb.WriteString("    metric:\n      metric_id: " + metricID + "\n")
		if unit != "" {
			sb.WriteString("      unit: " + unit + "\n")
		}
	}
	sb.WriteString("---\n# " + id + "\n")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEvaluateRequiredGates_RatchetPass verifies a gate whose observed value
// satisfies the ratchet returns status=pass with a machine-readable decision.
func TestEvaluateRequiredGates_RatchetPass(t *testing.T) {
	dir := t.TempDir()
	writeArtifactDoc(t, dir, "FEAT-RATCHET-PASS")
	writeRatchetGateDoc(t, dir, "exec.FEAT-RATCHET-PASS.latency", "FEAT-RATCHET-PASS",
		[]string{"sh", "-c", "echo 120"}, "lower-is-better", 250, "ms", "MET-LATENCY")

	results, anyFailed, anyRatchetFailed, err := evaluateRequiredGates(dir, []string{"FEAT-RATCHET-PASS"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anyFailed {
		t.Error("expected anyFailed=false for passing ratchet")
	}
	if anyRatchetFailed {
		t.Error("expected anyRatchetFailed=false when observed <= ratchet")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	gr := results[0]
	if gr.Status != "pass" {
		t.Errorf("expected status=pass, got %q", gr.Status)
	}
	if gr.Ratchet == nil {
		t.Fatal("expected ratchet evidence on passing gate")
	}
	if gr.Ratchet.Decision != "pass" {
		t.Errorf("expected ratchet decision=pass, got %q", gr.Ratchet.Decision)
	}
	if gr.Ratchet.Threshold != 250 {
		t.Errorf("expected ratchet threshold=250, got %v", gr.Ratchet.Threshold)
	}
	if gr.Ratchet.Observed != 120 {
		t.Errorf("expected observed=120, got %v", gr.Ratchet.Observed)
	}
	if gr.Ratchet.MetricID != "MET-LATENCY" {
		t.Errorf("expected metric_id=MET-LATENCY, got %q", gr.Ratchet.MetricID)
	}
	if gr.Ratchet.Comparison != "lower-is-better" {
		t.Errorf("expected comparison=lower-is-better, got %q", gr.Ratchet.Comparison)
	}
	if gr.Ratchet.Reason == "" {
		t.Error("expected non-empty reason for ratchet pass")
	}
}

// TestEvaluateRequiredGates_RatchetMiss verifies a gate whose observed value
// violates the ratchet is marked as a ratchet failure (not a generic gate fail).
func TestEvaluateRequiredGates_RatchetMiss(t *testing.T) {
	dir := t.TempDir()
	writeArtifactDoc(t, dir, "FEAT-RATCHET-MISS")
	writeRatchetGateDoc(t, dir, "exec.FEAT-RATCHET-MISS.latency", "FEAT-RATCHET-MISS",
		[]string{"sh", "-c", "echo 310"}, "lower-is-better", 250, "ms", "MET-LATENCY")

	results, anyFailed, anyRatchetFailed, err := evaluateRequiredGates(dir, []string{"FEAT-RATCHET-MISS"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !anyFailed {
		t.Error("expected anyFailed=true when ratchet missed")
	}
	if !anyRatchetFailed {
		t.Error("expected anyRatchetFailed=true when observed > ratchet")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	gr := results[0]
	if gr.Status != "fail" {
		t.Errorf("expected status=fail, got %q", gr.Status)
	}
	if gr.Ratchet == nil {
		t.Fatal("expected ratchet evidence on failing gate")
	}
	if gr.Ratchet.Decision != "fail" {
		t.Errorf("expected decision=fail, got %q", gr.Ratchet.Decision)
	}
	if gr.Ratchet.Threshold != 250 {
		t.Errorf("expected threshold=250, got %v", gr.Ratchet.Threshold)
	}
	if gr.Ratchet.Observed != 310 {
		t.Errorf("expected observed=310, got %v", gr.Ratchet.Observed)
	}
	if gr.Ratchet.Unit != "ms" {
		t.Errorf("expected unit=ms, got %q", gr.Ratchet.Unit)
	}
	if !strings.Contains(gr.Ratchet.Reason, "310") || !strings.Contains(gr.Ratchet.Reason, "250") {
		t.Errorf("expected reason to name observed and threshold, got %q", gr.Ratchet.Reason)
	}
}

// TestEvaluateRequiredGates_RatchetJSONValue verifies JSON stdout with a
// value field is parsed correctly. The JSON payload is written to a file so
// the shell invocation stays free of YAML-unsafe characters.
func TestEvaluateRequiredGates_RatchetJSONValue(t *testing.T) {
	dir := t.TempDir()
	writeArtifactDoc(t, dir, "FEAT-RATCHET-JSON")
	jsonPath := filepath.Join(dir, "observation.json")
	if err := os.WriteFile(jsonPath, []byte(`{"value": 900, "unit": "rps"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRatchetGateDoc(t, dir, "exec.FEAT-RATCHET-JSON.throughput", "FEAT-RATCHET-JSON",
		[]string{"cat", jsonPath}, "higher-is-better", 500, "", "MET-THROUGHPUT")

	results, anyFailed, anyRatchetFailed, err := evaluateRequiredGates(dir, []string{"FEAT-RATCHET-JSON"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anyFailed || anyRatchetFailed {
		t.Errorf("higher-is-better with observed 900 >= ratchet 500 should pass")
	}
	if len(results) != 1 || results[0].Ratchet == nil {
		t.Fatalf("expected 1 result with ratchet evidence, got %+v", results)
	}
	ev := results[0].Ratchet
	if ev.Observed != 900 {
		t.Errorf("expected observed=900, got %v", ev.Observed)
	}
	if ev.Unit != "rps" {
		t.Errorf("expected unit=rps (from JSON), got %q", ev.Unit)
	}
}

// TestEvaluateRequiredGates_RatchetHigherIsBetterMiss verifies higher-is-better
// misses when observed < threshold.
func TestEvaluateRequiredGates_RatchetHigherIsBetterMiss(t *testing.T) {
	dir := t.TempDir()
	writeArtifactDoc(t, dir, "FEAT-RATCHET-HIB")
	writeRatchetGateDoc(t, dir, "exec.FEAT-RATCHET-HIB.throughput", "FEAT-RATCHET-HIB",
		[]string{"sh", "-c", "echo 300"}, "higher-is-better", 500, "rps", "")

	_, anyFailed, anyRatchetFailed, err := evaluateRequiredGates(dir, []string{"FEAT-RATCHET-HIB"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !anyFailed || !anyRatchetFailed {
		t.Errorf("higher-is-better with observed 300 < ratchet 500 should fail: anyFailed=%v anyRatchetFailed=%v",
			anyFailed, anyRatchetFailed)
	}
}

// TestLandBeadResult_RatchetMiss_PreservesWithEvidence verifies the orchestrator
// preserves a result when a ratchet is missed, names the ratchet preserve
// reason, and attaches ratchet evidence for HELIX to read.
func TestLandBeadResult_RatchetMiss_PreservesWithEvidence(t *testing.T) {
	const beadID = "ddx-ratchet-miss-01"
	const specID = "FEAT-RATCHETMISS"

	projectRoot := setupGateTestProjectRoot(t)
	wtPath := t.TempDir()
	setupGateTestWorktree(t, wtPath, beadID, specID, false, 0)
	writeRatchetGateDoc(t, wtPath, "exec."+specID+".latency", specID,
		[]string{"sh", "-c", "echo 310"}, "lower-is-better", 250, "ms", "MET-API-LATENCY")

	res := &ExecuteBeadResult{
		BeadID:    beadID,
		BaseRev:   "aaa0000000000011",
		ResultRev: "bbb0000000000011",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
	}

	orch := &gateTestOrchestratorGitOps{}
	advancer := &gateTestLandingAdvancer{}

	tmpDir := t.TempDir()
	checksPath := filepath.Join(tmpDir, "checks.json")

	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		WtPath:             wtPath,
		GovernIDs:          []string{specID},
		LandingAdvancer:    advancer.advance,
		ChecksArtifactPath: checksPath,
		ChecksArtifactRel:  "checks.json",
	})
	if err != nil {
		t.Fatalf("LandBeadResult returned error: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "preserved" {
		t.Errorf("expected outcome=preserved when ratchet misses, got %q", res.Outcome)
	}
	if res.Reason != RatchetPreserveReason {
		t.Errorf("expected reason=%q, got %q", RatchetPreserveReason, res.Reason)
	}
	if res.Status != ExecuteBeadStatusRatchetFailed {
		t.Errorf("expected status=%q, got %q", ExecuteBeadStatusRatchetFailed, res.Status)
	}
	if res.FailureMode != FailureModeRatchetMiss {
		t.Errorf("expected failure_mode=%q, got %q", FailureModeRatchetMiss, res.FailureMode)
	}
	if advancer.called {
		t.Error("LandingAdvancer must not be called when ratchet misses")
	}
	if orch.preserveRef == "" {
		t.Error("expected preserve ref to be set on ratchet miss")
	}
	if res.RatchetSummary != "fail" {
		t.Errorf("expected ratchet_summary=fail, got %q", res.RatchetSummary)
	}
	if len(res.RatchetEvidence) != 1 {
		t.Fatalf("expected 1 ratchet evidence entry, got %d", len(res.RatchetEvidence))
	}
	ev := res.RatchetEvidence[0]
	if ev.Decision != "fail" || ev.Observed != 310 || ev.Threshold != 250 {
		t.Errorf("unexpected evidence: %+v", ev)
	}
	if ev.MetricID != "MET-API-LATENCY" {
		t.Errorf("expected metric_id=MET-API-LATENCY, got %q", ev.MetricID)
	}

	raw, err := os.ReadFile(checksPath)
	if err != nil {
		t.Fatalf("expected checks.json on ratchet miss: %v", err)
	}
	var checks struct {
		Summary         string            `json:"summary"`
		RatchetSummary  string            `json:"ratchet_summary"`
		RatchetEvidence []RatchetEvidence `json:"ratchet_evidence"`
	}
	if err := json.Unmarshal(raw, &checks); err != nil {
		t.Fatalf("parse checks.json: %v", err)
	}
	if checks.RatchetSummary != "fail" {
		t.Errorf("checks.json ratchet_summary=%q, want fail", checks.RatchetSummary)
	}
	if len(checks.RatchetEvidence) != 1 || checks.RatchetEvidence[0].Decision != "fail" {
		t.Errorf("checks.json ratchet_evidence missing/incorrect: %+v", checks.RatchetEvidence)
	}
}

// TestLandBeadResult_RatchetPass_MergesWithEvidence verifies a passing ratchet
// records machine-readable evidence AND allows landing.
func TestLandBeadResult_RatchetPass_MergesWithEvidence(t *testing.T) {
	const beadID = "ddx-ratchet-pass-01"
	const specID = "FEAT-RATCHETPASS"

	projectRoot := setupGateTestProjectRoot(t)
	wtPath := t.TempDir()
	setupGateTestWorktree(t, wtPath, beadID, specID, false, 0)
	writeRatchetGateDoc(t, wtPath, "exec."+specID+".latency", specID,
		[]string{"sh", "-c", "echo 180"}, "lower-is-better", 250, "ms", "MET-API-LATENCY")

	res := &ExecuteBeadResult{
		BeadID:    beadID,
		BaseRev:   "aaa0000000000012",
		ResultRev: "bbb0000000000012",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
	}

	orch := &gateTestOrchestratorGitOps{}
	advancer := &gateTestLandingAdvancer{}

	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		WtPath:          wtPath,
		GovernIDs:       []string{specID},
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult returned error: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "merged" {
		t.Errorf("expected outcome=merged when ratchet passes, got %q (reason=%q)", res.Outcome, res.Reason)
	}
	if res.Status != ExecuteBeadStatusSuccess {
		t.Errorf("expected status=success, got %q", res.Status)
	}
	if res.FailureMode != "" {
		t.Errorf("expected empty failure_mode on merged ratchet pass, got %q", res.FailureMode)
	}
	if !advancer.called {
		t.Error("LandingAdvancer must be called when ratchet passes")
	}
	if orch.preserveRef != "" {
		t.Errorf("expected no preserve ref on ratchet pass, got %q", orch.preserveRef)
	}
	if res.RatchetSummary != "pass" {
		t.Errorf("expected ratchet_summary=pass, got %q", res.RatchetSummary)
	}
	if len(res.RatchetEvidence) != 1 {
		t.Fatalf("expected 1 ratchet evidence entry, got %d", len(res.RatchetEvidence))
	}
	ev := res.RatchetEvidence[0]
	if ev.Decision != "pass" || ev.Observed != 180 || ev.Threshold != 250 {
		t.Errorf("unexpected passing evidence: %+v", ev)
	}
}

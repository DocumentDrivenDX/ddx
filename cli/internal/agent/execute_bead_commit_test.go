package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// minimalResult returns an ExecuteBeadResult with all required fields set to
// deterministic values for use in commit-message tests.
func minimalResult() *ExecuteBeadResult {
	return &ExecuteBeadResult{
		BeadID:       "ddx-abc12345",
		AttemptID:    "20260413T140544-6b4034a1",
		WorkerID:     "worker-testid",
		SessionID:    "eb-aabb1234",
		BaseRev:      "418a646def012345",
		ResultRev:    "63f71eeabc120000",
		Harness:      "agent",
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-6",
		Tokens:       12500,
		CostUSD:      0.05,
		ExitCode:     0,
		ExecutionDir: ".ddx/executions/20260413T140544-6b4034a1",
		Outcome:      ExecuteBeadOutcomeTaskSucceeded,
		Status:       ExecuteBeadStatusSuccess,
		StartedAt:    time.Date(2026, 4, 13, 14, 5, 44, 0, time.UTC),
		FinishedAt:   time.Date(2026, 4, 13, 14, 7, 44, 0, time.UTC),
	}
}

// TestBuildIterationCommitMessage_Deterministic verifies that two calls with
// identical inputs produce byte-for-byte identical commit messages.
func TestBuildIterationCommitMessage_Deterministic(t *testing.T) {
	res := minimalResult()
	msg1 := BuildIterationCommitMessage(res)
	msg2 := BuildIterationCommitMessage(res)
	if msg1 != msg2 {
		t.Errorf("BuildIterationCommitMessage is not deterministic:\nmsg1=%q\nmsg2=%q", msg1, msg2)
	}
}

// TestBuildIterationCommitMessage_Subject verifies the subject line format.
func TestBuildIterationCommitMessage_Subject(t *testing.T) {
	res := minimalResult()
	msg := BuildIterationCommitMessage(res)
	lines := strings.Split(msg, "\n")
	want := "chore: execute-bead iteration " + res.BeadID
	if lines[0] != want {
		t.Errorf("subject = %q, want %q", lines[0], want)
	}
}

// TestBuildIterationCommitMessage_CanonicalTrailers verifies that all five
// canonical Git trailers are present with the correct values.
func TestBuildIterationCommitMessage_CanonicalTrailers(t *testing.T) {
	res := minimalResult()
	msg := BuildIterationCommitMessage(res)

	cases := []struct{ trailer, want string }{
		{"Ddx-Attempt-Id", res.AttemptID},
		{"Ddx-Worker-Id", res.WorkerID},
		{"Ddx-Harness", res.Harness},
		{"Ddx-Model", res.Model},
		{"Ddx-Result-Status", res.Status},
	}
	for _, c := range cases {
		line := c.trailer + ": " + c.want
		if !strings.Contains(msg, line) {
			t.Errorf("message missing trailer line %q", line)
		}
	}
}

// TestBuildIterationCommitMessage_IterationBlock verifies that the ddx-iteration
// JSON block is present and well-formed with the correct field values.
func TestBuildIterationCommitMessage_IterationBlock(t *testing.T) {
	res := minimalResult()
	msg := BuildIterationCommitMessage(res)

	// Locate the ddx-iteration prefix.
	const prefix = "ddx-iteration: "
	idx := strings.Index(msg, prefix)
	if idx == -1 {
		t.Fatalf("message missing %q block", prefix)
	}

	// Extract the JSON portion: from '{' to the matching '}'.
	jsonStart := strings.Index(msg[idx:], "{")
	if jsonStart == -1 {
		t.Fatalf("ddx-iteration block has no JSON object")
	}
	jsonStart += idx
	// Find the closing brace of the top-level object.
	jsonEnd := strings.Index(msg[jsonStart:], "\n\n")
	var raw string
	if jsonEnd == -1 {
		raw = msg[jsonStart:]
		// Trim any trailing trailer lines after the JSON.
		if i := strings.Index(raw, "\nDdx-"); i != -1 {
			raw = raw[:i]
		}
	} else {
		raw = msg[jsonStart : jsonStart+jsonEnd]
	}
	raw = strings.TrimSpace(raw)

	var block ExecuteBeadIterationBlock
	if err := json.Unmarshal([]byte(raw), &block); err != nil {
		t.Fatalf("ddx-iteration JSON parse error: %v\nraw=%q", err, raw)
	}

	if block.BeadID != res.BeadID {
		t.Errorf("block.bead_id = %q, want %q", block.BeadID, res.BeadID)
	}
	if block.AttemptID != res.AttemptID {
		t.Errorf("block.attempt_id = %q, want %q", block.AttemptID, res.AttemptID)
	}
	if block.SessionID != res.SessionID {
		t.Errorf("block.session_id = %q, want %q", block.SessionID, res.SessionID)
	}
	if block.Harness != res.Harness {
		t.Errorf("block.harness = %q, want %q", block.Harness, res.Harness)
	}
	if block.Model != res.Model {
		t.Errorf("block.model = %q, want %q", block.Model, res.Model)
	}
	if block.TotalTokens != res.Tokens {
		t.Errorf("block.total_tokens = %d, want %d", block.TotalTokens, res.Tokens)
	}
	if block.BaseRev != res.BaseRev {
		t.Errorf("block.base_rev = %q, want %q", block.BaseRev, res.BaseRev)
	}
	if block.ResultRev != res.ResultRev {
		t.Errorf("block.result_rev = %q, want %q", block.ResultRev, res.ResultRev)
	}
	if block.ExecutionBundle != res.ExecutionDir {
		t.Errorf("block.execution_bundle = %q, want %q", block.ExecutionBundle, res.ExecutionDir)
	}
}

// TestBuildIterationCommitMessage_WorkerIDFallback verifies that WorkerID falls
// back to SessionID when no distinct worker identity is provided.
func TestBuildIterationCommitMessage_WorkerIDFallback(t *testing.T) {
	res := minimalResult()
	res.WorkerID = "" // no distinct worker ID
	msg := BuildIterationCommitMessage(res)

	want := "Ddx-Worker-Id: " + res.SessionID
	if !strings.Contains(msg, want) {
		t.Errorf("WorkerID fallback to SessionID failed: %q not in message", want)
	}
}

// TestBuildIterationCommitMessage_EmptyResultRev verifies that an empty
// ResultRev (pre-commit state) is handled deterministically — the field is
// omitted from the JSON block rather than emitting a null or placeholder.
func TestBuildIterationCommitMessage_EmptyResultRev(t *testing.T) {
	res := minimalResult()
	res.ResultRev = ""
	msg := BuildIterationCommitMessage(res)

	// The message must still be a valid commit message with all trailers.
	if !strings.Contains(msg, "Ddx-Attempt-Id: "+res.AttemptID) {
		t.Error("trailers missing when ResultRev is empty")
	}
	// result_rev should be absent from the JSON when empty (omitempty).
	if strings.Contains(msg, `"result_rev"`) {
		t.Error(`"result_rev" key should be omitted when ResultRev is empty`)
	}
}

// TestBuildIterationCommitMessage_RequiredExecSummaryDefault verifies that
// RequiredExecSummary defaults to "skipped" when unset (pre-landing state).
func TestBuildIterationCommitMessage_RequiredExecSummaryDefault(t *testing.T) {
	res := minimalResult()
	res.RequiredExecSummary = ""
	msg := BuildIterationCommitMessage(res)

	if !strings.Contains(msg, `"required_exec_summary": "skipped"`) {
		t.Errorf("expected required_exec_summary=skipped when unset, message: %q", msg)
	}
}

// TestBuildIterationCommitMessage_RequiredExecSummaryPreserved verifies that
// a non-empty RequiredExecSummary is passed through unchanged.
func TestBuildIterationCommitMessage_RequiredExecSummaryPreserved(t *testing.T) {
	for _, summary := range []string{"pass", "fail", "skipped"} {
		res := minimalResult()
		res.RequiredExecSummary = summary
		msg := BuildIterationCommitMessage(res)
		want := `"required_exec_summary": "` + summary + `"`
		if !strings.Contains(msg, want) {
			t.Errorf("summary=%q not found in message", summary)
		}
	}
}

// TestBuildCommitMessageFromResultFile_FileSourced verifies that
// BuildCommitMessageFromResultFile reads result.json and produces a message
// identical to BuildIterationCommitMessage called with the same data.
func TestBuildCommitMessageFromResultFile_FileSourced(t *testing.T) {
	res := minimalResult()

	dir := t.TempDir()
	resultPath := filepath.Join(dir, "result.json")
	raw, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		t.Fatalf("marshalling result: %v", err)
	}
	if err := os.WriteFile(resultPath, raw, 0o644); err != nil {
		t.Fatalf("writing result.json: %v", err)
	}

	// Build message via direct struct call.
	wantMsg := BuildIterationCommitMessage(res)

	// Build message via file-sourced call.
	gotMsg, err := BuildCommitMessageFromResultFile(resultPath)
	if err != nil {
		t.Fatalf("BuildCommitMessageFromResultFile: %v", err)
	}

	if gotMsg != wantMsg {
		t.Errorf("file-sourced message differs from struct-sourced message:\ngot =%q\nwant=%q", gotMsg, wantMsg)
	}
}

// TestBuildCommitMessageFromResultFile_MissingFile verifies that an error is
// returned when the result file does not exist.
func TestBuildCommitMessageFromResultFile_MissingFile(t *testing.T) {
	_, err := BuildCommitMessageFromResultFile("/nonexistent/result.json")
	if err == nil {
		t.Error("expected error for missing result file, got nil")
	}
}

// TestBuildCommitMessageFromResultFile_CorruptFile verifies that an error is
// returned when the result file contains invalid JSON.
func TestBuildCommitMessageFromResultFile_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "result.json")
	if err := os.WriteFile(p, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := BuildCommitMessageFromResultFile(p)
	if err == nil {
		t.Error("expected error for corrupt result file, got nil")
	}
}

// TestSynthesizeCommitMessage_InExecuteBead verifies that when ExecuteBead
// synthesizes a commit (agent left dirty files), the SynthesizeCommit call
// receives a commit message containing the canonical trailers and the
// ddx-iteration block sourced from result.json.
func TestSynthesizeCommitMessage_InExecuteBead(t *testing.T) {
	const beadID = "ddx-commit-msg-01"
	const baseRev = "aaaa111100000001"
	const synthRev = "aaaa111100000002"

	var capturedMsg string
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, ".ddx"), 0o755); err != nil {
		t.Fatal(err)
	}

	gitOps := &commitMsgCapturingGitOps{
		projectRoot: projectRoot,
		baseRev:     baseRev,
		synthRev:    synthRev,
		captureMsg:  func(msg string) { capturedMsg = msg },
		wtSetupFn: func(wtPath string) {
			setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
		},
	}

	_, err := ExecuteBead(context.Background(), projectRoot, beadID, ExecuteBeadOptions{
		Harness:     "test-harness",
		Model:       "test-model",
		AgentRunner: &artifactTestAgentRunner{},
	}, gitOps)
	if err != nil {
		t.Fatalf("ExecuteBead: %v", err)
	}

	if capturedMsg == "" {
		t.Fatal("SynthesizeCommit was not called (no dirty worktree?)")
	}

	// Verify canonical trailers are present.
	for _, trailer := range []string{
		"Ddx-Attempt-Id:",
		"Ddx-Worker-Id:",
		"Ddx-Harness: test-harness",
		"Ddx-Model: test-model",
		"Ddx-Result-Status:",
	} {
		if !strings.Contains(capturedMsg, trailer) {
			t.Errorf("commit message missing trailer %q\nmessage:\n%s", trailer, capturedMsg)
		}
	}

	// Verify the ddx-iteration block is present.
	if !strings.Contains(capturedMsg, "ddx-iteration:") {
		t.Errorf("commit message missing ddx-iteration block\nmessage:\n%s", capturedMsg)
	}

	// Verify the execution bundle pointer is present.
	if !strings.Contains(capturedMsg, ".ddx/executions/") {
		t.Errorf("commit message missing execution bundle pointer\nmessage:\n%s", capturedMsg)
	}
}

// commitMsgCapturingGitOps is a GitOps mock that captures the commit message
// passed to SynthesizeCommit and simulates a dirty worktree.
type commitMsgCapturingGitOps struct {
	projectRoot string
	baseRev     string
	synthRev    string
	captureMsg  func(string)
	wtSetupFn   func(wtPath string)
}

func (m *commitMsgCapturingGitOps) HeadRev(dir string) (string, error) {
	if filepath.Clean(dir) == filepath.Clean(m.projectRoot) {
		return m.baseRev, nil
	}
	// Worktree: return baseRev initially (no agent commits), then synthRev
	// after SynthesizeCommit is called (tracked via a pointer swap).
	return m.baseRev, nil
}

func (m *commitMsgCapturingGitOps) ResolveRev(dir, rev string) (string, error) {
	return m.baseRev, nil
}

func (m *commitMsgCapturingGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		return err
	}
	if m.wtSetupFn != nil {
		m.wtSetupFn(wtPath)
	}
	return nil
}

func (m *commitMsgCapturingGitOps) WorktreeRemove(dir, wtPath string) error { return nil }
func (m *commitMsgCapturingGitOps) WorktreeList(dir string) ([]string, error) {
	return nil, nil
}
func (m *commitMsgCapturingGitOps) WorktreePrune(dir string) error { return nil }

// IsDirty returns true so ExecuteBead enters the SynthesizeCommit path.
func (m *commitMsgCapturingGitOps) IsDirty(dir string) (bool, error) { return true, nil }

func (m *commitMsgCapturingGitOps) SynthesizeCommit(dir, msg string) (bool, error) {
	if m.captureMsg != nil {
		m.captureMsg(msg)
	}
	// Simulate the commit by advancing the effective HEAD to synthRev.
	// The test's HeadRev still returns baseRev so resultRev won't change,
	// but we confirm the message was received.
	return true, nil
}

func (m *commitMsgCapturingGitOps) UpdateRef(dir, ref, sha string) error { return nil }
func (m *commitMsgCapturingGitOps) DeleteRef(dir, ref string) error      { return nil }

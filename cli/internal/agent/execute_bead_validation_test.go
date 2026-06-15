package agent

import (
	"strings"
	"testing"
)

// TestParseHeadReflog_OrdersOldestFirst verifies the reflog parser turns
// git's newest-first output into oldest-first CommitEvents and splits the
// action verb from the subject.
func TestParseHeadReflog_OrdersOldestFirst(t *testing.T) {
	lines := []string{
		"66beb39ba598dfe12301dec195f0673e750e1c98 HEAD@{0}: commit (amend): fix shard_id init [niflheim-bc47ed66]",
		"e7ae0424aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa HEAD@{1}: commit (amend): fix compile errors [niflheim-bc47ed66]",
		"1551674daaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa HEAD@{2}: commit: implement shard router [niflheim-bc47ed66]",
		"bc8d9bac5b1ee1794245ae2784c2a285700fb2c3 HEAD@{3}: checkout: moving from main to bc8d9bac",
	}
	events := ParseHeadReflog(lines)
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d: %+v", len(events), events)
	}
	if events[0].Action != "checkout" {
		t.Errorf("oldest event should be checkout, got %q", events[0].Action)
	}
	if events[1].Action != "commit" || !strings.HasPrefix(events[1].SHA, "1551674d") {
		t.Errorf("expected first commit at index 1, got %+v", events[1])
	}
	if events[3].Action != "commit (amend)" || !strings.HasPrefix(events[3].SHA, "66beb39b") {
		t.Errorf("expected final amend at index 3, got %+v", events[3])
	}
	if events[1].Subject != "implement shard router [niflheim-bc47ed66]" {
		t.Errorf("unexpected subject: %q", events[1].Subject)
	}
}

// TestValidateAttemptIntegrity_PostCommitMutationRejected (AC1) simulates an
// execute-bead transcript where the agent makes an initial implementation
// commit and then amends it twice (the niflheim-bc47ed66 / attempt
// 20260518T021035-da282d6e failure), and asserts the attempt is rejected.
func TestValidateAttemptIntegrity_PostCommitMutationRejected(t *testing.T) {
	reflog := []string{
		"66beb39ba598dfe12301dec195f0673e750e1c98 HEAD@{0}: commit (amend): fix more shard_id [niflheim-bc47ed66]",
		"e7ae0424aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa HEAD@{1}: commit (amend): fix compile errors [niflheim-bc47ed66]",
		"1551674daaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa HEAD@{2}: commit: implement shard router [niflheim-bc47ed66]",
	}
	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "bc8d9bac5b1ee1794245ae2784c2a285700fb2c3",
		ImplementationRev: "66beb39ba598dfe12301dec195f0673e750e1c98",
		CommitEvents:      ParseHeadReflog(reflog),
		CodeChanging:      true,
	})
	if verdict.OK {
		t.Fatal("expected post-commit mutation to be rejected, got OK")
	}
	if verdict.Reason != IntegrityReasonPostCommitMutation {
		t.Errorf("expected reason %q, got %q", IntegrityReasonPostCommitMutation, verdict.Reason)
	}
	// AC4: the detail must make clear this is a DDx validation rejection, not
	// an implementation failure, so an operator can tell them apart.
	if !strings.Contains(strings.ToLower(verdict.Detail), "ddx validation") ||
		!strings.Contains(strings.ToLower(verdict.Detail), "not an implementation failure") {
		t.Errorf("detail should distinguish DDx validation from implementation failure, got %q", verdict.Detail)
	}
}

// TestValidateAttemptIntegrity_EmptyGateEvidenceRejected (AC2) simulates a
// lefthook run where every hook skips because there are no staged files, and
// asserts it is not accepted as pre-commit gate evidence for a code-changing
// bead.
func TestValidateAttemptIntegrity_EmptyGateEvidenceRejected(t *testing.T) {
	noStagedOutput := strings.Join([]string{
		"╭──────────────────────────────────────╮",
		"│ 🥊 lefthook v1.7.0  hook: pre-commit  │",
		"╰──────────────────────────────────────╯",
		"summary: (skip) no files for inspection",
	}, "\n")

	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "aaa0000",
		ImplementationRev: "bbb0000",
		CommitEvents: []CommitEvent{
			{SHA: "bbb0000", Action: "commit", Subject: "do work [x]"},
		},
		CodeChanging: true,
		GateRuns: []PreCommitGateRun{
			{Command: "lefthook run pre-commit", Output: noStagedOutput},
		},
	})
	if verdict.OK {
		t.Fatal("expected empty gate evidence to be rejected, got OK")
	}
	if verdict.Reason != IntegrityReasonEmptyGateEvidence {
		t.Errorf("expected reason %q, got %q", IntegrityReasonEmptyGateEvidence, verdict.Reason)
	}
	if !strings.Contains(strings.ToLower(verdict.Detail), "ddx validation") {
		t.Errorf("detail should mark the rejection as DDx validation, got %q", verdict.Detail)
	}
}

// TestValidateAttemptIntegrity_MeaningfulGateRunAccepted verifies a lefthook
// run that actually executed hooks against staged files is accepted, even when
// an earlier no-staged-files run is also present.
func TestValidateAttemptIntegrity_MeaningfulGateRunAccepted(t *testing.T) {
	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "aaa0000",
		ImplementationRev: "bbb0000",
		CommitEvents: []CommitEvent{
			{SHA: "bbb0000", Action: "commit", Subject: "do work [x]"},
		},
		CodeChanging: true,
		GateRuns: []PreCommitGateRun{
			{Command: "lefthook run pre-commit", Output: "summary: (skip) no files for inspection"},
			{Command: "lefthook run pre-commit", Output: "go-test\n✔ go-test (executed in 4.21s)\nsummary: (done in 4.3s)"},
		},
	})
	if !verdict.OK {
		t.Fatalf("expected a meaningful staged gate run to be accepted, got reason=%q detail=%q", verdict.Reason, verdict.Detail)
	}
}

// TestValidateAttemptIntegrity_CleanAttemptPasses (AC3) verifies that an
// attempt which stages files, runs the staged gate meaningfully, creates
// exactly one implementation commit, and leaves a clean worktree passes.
func TestValidateAttemptIntegrity_CleanAttemptPasses(t *testing.T) {
	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "aaa0000000000000000000000000000000000000",
		ImplementationRev: "bbb0000000000000000000000000000000000000",
		CommitEvents: []CommitEvent{
			{SHA: "aaa0000000000000000000000000000000000000", Action: "checkout", Subject: "moving from main"},
			{SHA: "bbb0000000000000000000000000000000000000", Action: "commit", Subject: "implement feature [x]"},
		},
		DirtyPaths:   nil,
		CodeChanging: true,
		GateRuns: []PreCommitGateRun{
			{Command: "lefthook run pre-commit", Output: "go-test\n✔ go-test (executed in 2.0s)\nsummary: (done in 2.1s)"},
		},
	})
	if !verdict.OK {
		t.Fatalf("expected clean single-commit attempt to pass, got reason=%q detail=%q", verdict.Reason, verdict.Detail)
	}
}

// TestValidateAttemptIntegrity_DirtyAfterCommitRejected verifies tracked files
// left uncommitted after the implementation commit are rejected.
func TestValidateAttemptIntegrity_DirtyAfterCommitRejected(t *testing.T) {
	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "aaa0000",
		ImplementationRev: "bbb0000",
		CommitEvents: []CommitEvent{
			{SHA: "bbb0000", Action: "commit", Subject: "do work [x]"},
		},
		CodeChanging: true,
		DirtyPaths:   []string{"cli/internal/agent/foo.go"},
	})
	if verdict.OK {
		t.Fatal("expected dirty-after-commit to be rejected, got OK")
	}
	if verdict.Reason != IntegrityReasonDirtyAfterCommit {
		t.Errorf("expected reason %q, got %q", IntegrityReasonDirtyAfterCommit, verdict.Reason)
	}
}

// TestValidateAttemptIntegrity_SingleCommitNoEvidenceSkips verifies the
// validator never rejects checks it could not observe: a single matching
// commit with no captured gate runs passes (the gate check is skipped, not
// failed).
func TestValidateAttemptIntegrity_SingleCommitNoEvidenceSkips(t *testing.T) {
	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "aaa0000",
		ImplementationRev: "bbb0000",
		CommitEvents: []CommitEvent{
			{SHA: "bbb0000", Action: "commit", Subject: "do work [x]"},
		},
		CodeChanging: true,
	})
	if !verdict.OK {
		t.Fatalf("expected single-commit/no-evidence to pass (skip), got reason=%q", verdict.Reason)
	}
}

// TestAttemptIntegrityRejectsVerificationWrongCWD (AC1) feeds a transcript
// where a required Go verification command returned `go.mod file not found`,
// and asserts DDx classifies it as wrong_cwd_verification and does not accept
// it as passing evidence.
func TestAttemptIntegrityRejectsVerificationWrongCWD(t *testing.T) {
	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "aaa0000",
		ImplementationRev: "bbb0000",
		CommitEvents: []CommitEvent{
			{SHA: "bbb0000", Action: "commit", Subject: "do work [x]"},
		},
		CodeChanging: true,
		TranscriptCommands: []TranscriptCommand{
			{
				Command: "go test ./internal/agent/...",
				Output:  "go: go.mod file not found in current directory or any parent directory; see 'go help modules'",
			},
		},
	})
	if verdict.OK {
		t.Fatal("expected wrong-cwd verification to be rejected, got OK")
	}
	if verdict.Reason != IntegrityReasonWrongCWDVerification {
		t.Errorf("expected reason %q, got %q", IntegrityReasonWrongCWDVerification, verdict.Reason)
	}
	if !strings.Contains(strings.ToLower(verdict.Detail), "ddx validation") ||
		!strings.Contains(strings.ToLower(verdict.Detail), "not an implementation failure") {
		t.Errorf("detail should mark this as a DDx validation rejection, got %q", verdict.Detail)
	}
}

// TestAttemptIntegrityRejectsMissingExecutionBundleLookup (AC2) feeds a
// command/output pair that tries `.ddx/executions/` from the wrong cwd and
// asserts DDx records evidence_path_wrong_cwd.
func TestAttemptIntegrityRejectsMissingExecutionBundleLookup(t *testing.T) {
	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "aaa0000",
		ImplementationRev: "bbb0000",
		CommitEvents: []CommitEvent{
			{SHA: "bbb0000", Action: "commit", Subject: "do work [x]"},
		},
		CodeChanging: true,
		TranscriptCommands: []TranscriptCommand{
			{
				Command: "ls .ddx/executions/20260615T140747-d0d32f0e",
				Output:  "ls: cannot access '.ddx/executions/20260615T140747-d0d32f0e': No such file or directory",
			},
		},
	})
	if verdict.OK {
		t.Fatal("expected missing-bundle lookup to be rejected, got OK")
	}
	if verdict.Reason != IntegrityReasonEvidencePathWrongCWD {
		t.Errorf("expected reason %q, got %q", IntegrityReasonEvidencePathWrongCWD, verdict.Reason)
	}
	if !strings.Contains(strings.ToLower(verdict.Detail), "ddx validation") {
		t.Errorf("detail should mark this as a DDx validation rejection, got %q", verdict.Detail)
	}
}

// TestAttemptIntegrityRejectsTmpRoundTripForSourceFiles (AC3) feeds transcript
// commands that move a tracked source file through /tmp and asserts DDx records
// tmp_roundtrip_tracked_file and rejects the attempt.
func TestAttemptIntegrityRejectsTmpRoundTripForSourceFiles(t *testing.T) {
	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "aaa0000",
		ImplementationRev: "bbb0000",
		CommitEvents: []CommitEvent{
			{SHA: "bbb0000", Action: "commit", Subject: "do work [x]"},
		},
		CodeChanging: true,
		TranscriptCommands: []TranscriptCommand{
			{Command: "mv cli/internal/agent/execute_bead.go /tmp/execute_bead.go"},
			{Command: "mv /tmp/execute_bead.go cli/internal/agent/execute_bead.go"},
		},
	})
	if verdict.OK {
		t.Fatal("expected /tmp round-trip of a tracked source file to be rejected, got OK")
	}
	if verdict.Reason != IntegrityReasonTmpRoundtripTrackedFile {
		t.Errorf("expected reason %q, got %q", IntegrityReasonTmpRoundtripTrackedFile, verdict.Reason)
	}
	if !strings.Contains(strings.ToLower(verdict.Detail), "ddx validation") {
		t.Errorf("detail should mark this as a DDx validation rejection, got %q", verdict.Detail)
	}
}

// TestAttemptIntegrityAllowsBenignBuildToolTempUsage (AC4) proves build
// cache/temp paths that do not move repo source or evidence are not rejected.
func TestAttemptIntegrityAllowsBenignBuildToolTempUsage(t *testing.T) {
	verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
		BaseRev:           "aaa0000",
		ImplementationRev: "bbb0000",
		CommitEvents: []CommitEvent{
			{SHA: "bbb0000", Action: "commit", Subject: "do work [x]"},
		},
		CodeChanging: true,
		GateRuns: []PreCommitGateRun{
			{Command: "lefthook run pre-commit", Output: "go-test\n✔ go-test (executed in 2.0s)"},
		},
		TranscriptCommands: []TranscriptCommand{
			// Build cache + binary written to /tmp — no tracked source moved.
			{Command: "GOCACHE=/tmp/go-cache go build -o /tmp/ddx-bin ./cmd/ddx", Output: ""},
			// A normal in-module verification run that actually succeeded.
			{Command: "go test ./internal/agent/...", Output: "ok  github.com/DocumentDrivenDX/ddx/internal/agent  1.21s"},
			// Reading a temp scratch note that is not a source/evidence file.
			{Command: "cat /tmp/scratch-notes.txt", Output: "reminder: run gofmt"},
		},
	})
	if !verdict.OK {
		t.Fatalf("expected benign build-tool temp usage to pass, got reason=%q detail=%q", verdict.Reason, verdict.Detail)
	}
}

// TestLandBeadResult_AttemptIntegrityPreserved (AC1 + AC4 at the orchestrator
// boundary) verifies a worker result flagged with FailureModeAttemptIntegrity
// and commits is preserved for review — not merged — and that the final result
// distinguishes the DDx validation failure from an implementation failure.
func TestLandBeadResult_AttemptIntegrityPreserved(t *testing.T) {
	projectRoot := t.TempDir()
	res := makeWorkerResult("ddx-integrity-01", "aaa0001", "bbb0001", 0)
	res.Outcome = ExecuteBeadOutcomeTaskFailed
	res.FailureMode = FailureModeAttemptIntegrity
	res.Reason = AttemptIntegrityPreserveReason
	res.Error = "DDx validation: the implementation commit was rewritten after the first commit. Detected by DDx, not an implementation failure."
	orch := &orchTestGitOps{}

	advancer := &fakeLandingAdvancer{}
	landing, err := LandBeadResult(projectRoot, res, orch, BeadLandingOptions{
		LandingAdvancer: advancer.advance,
	})
	if err != nil {
		t.Fatalf("LandBeadResult: %v", err)
	}
	ApplyLandingToResult(res, landing)

	if res.Outcome != "preserved" {
		t.Errorf("expected outcome=preserved, got %q", res.Outcome)
	}
	if advancer.calls != 0 {
		t.Errorf("expected 0 advancer calls (must not merge), got %d", advancer.calls)
	}
	if orch.preserveRef == "" {
		t.Error("expected the integrity-rejected commit to be preserved under an iteration ref")
	}
	if res.Status != ExecuteBeadStatusPreservedNeedsReview {
		t.Errorf("expected status=preserved_needs_review, got %q", res.Status)
	}
	if res.FailureMode != FailureModeAttemptIntegrity {
		t.Errorf("expected failure_mode=attempt_integrity (distinct from execution_failed), got %q", res.FailureMode)
	}
	if !strings.Contains(strings.ToLower(res.Detail), "ddx validation") {
		t.Errorf("result detail should explain the DDx validation failure, got %q", res.Detail)
	}
}

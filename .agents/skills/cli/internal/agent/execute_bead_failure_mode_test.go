package agent

import "testing"

// TestFailureModeClassifyWorker exercises ClassifyFailureMode against the
// known worker-level signals. The acceptance contract enumerates the full
// set of modes: context_overflow, merge_conflict, test_failure,
// build_failure, timeout, auth_error, no_changes, unknown (plus "" on
// success). Each case names its input so a pattern drift is easy to spot
// in test failures.
func TestFailureModeClassifyWorker(t *testing.T) {
	cases := []struct {
		name     string
		outcome  string
		exitCode int
		errMsg   string
		want     string
	}{
		{"success_clean", ExecuteBeadOutcomeTaskSucceeded, 0, "", ""},
		{"no_changes", ExecuteBeadOutcomeTaskNoChanges, 0, "", FailureModeNoChanges},
		{"no_changes_ignores_error_text", ExecuteBeadOutcomeTaskNoChanges, 0, "build failed", FailureModeNoChanges},

		// Context / long-context hangs.
		{"context_length_exceeded", ExecuteBeadOutcomeTaskFailed, 1,
			"context_length_exceeded: prompt exceeds 200000 tokens", FailureModeContextOverflow},
		{"prompt_too_long", ExecuteBeadOutcomeTaskFailed, 1,
			"Error: prompt is too long for model", FailureModeContextOverflow},
		{"maximum_context", ExecuteBeadOutcomeTaskFailed, 1,
			"request exceeds the maximum context length", FailureModeContextOverflow},

		// Auth / quota.
		{"unauthorized_401", ExecuteBeadOutcomeTaskFailed, 1,
			"401 Unauthorized: invalid API key", FailureModeAuthError},
		{"insufficient_quota", ExecuteBeadOutcomeTaskFailed, 1,
			"insufficient_quota: you have exceeded your current quota", FailureModeAuthError},
		{"rate_limit", ExecuteBeadOutcomeTaskFailed, 1,
			"429 Too Many Requests: rate limit reached", FailureModeAuthError},

		// Timeout.
		{"context_deadline_exceeded", ExecuteBeadOutcomeTaskFailed, 1,
			"context deadline exceeded", FailureModeTimeout},
		{"timed_out", ExecuteBeadOutcomeTaskFailed, 1,
			"agent call timed out after 2h", FailureModeTimeout},

		// Merge conflict (worker-visible when orchestrator surfaces it via
		// Reason/Error). The landing classifier refines this; the worker
		// classifier recognises the text so a pre-landing result is still
		// tagged correctly.
		{"merge_conflict", ExecuteBeadOutcomeTaskFailed, 1,
			"merge conflict in cli/cmd/agent_metrics.go", FailureModeMergeConflict},

		// Build failure: shipped code that does not compile.
		{"go_compile_error", ExecuteBeadOutcomeTaskFailed, 1,
			"undefined: foo.Bar", FailureModeBuildFailure},
		{"build_failed", ExecuteBeadOutcomeTaskFailed, 1,
			"build failed with 3 errors", FailureModeBuildFailure},

		// Test failure.
		{"go_test_fail_marker", ExecuteBeadOutcomeTaskFailed, 1,
			"--- FAIL: TestX (0.01s)", FailureModeTestFailure},
		{"tests_failed", ExecuteBeadOutcomeTaskFailed, 1,
			"tests failed: 3 of 12 assertions did not pass", FailureModeTestFailure},

		// Unknown fallback: non-zero exit with no recognised pattern.
		{"unknown_error", ExecuteBeadOutcomeTaskFailed, 1,
			"something exploded", FailureModeUnknown},
		{"no_error_text_non_zero", ExecuteBeadOutcomeTaskFailed, 137, "",
			FailureModeUnknown},

		// Ordering: timeout wins over test/build keywords when both appear.
		{"timeout_beats_test_failure", ExecuteBeadOutcomeTaskFailed, 1,
			"test timed out after 5m", FailureModeTimeout},
		// Ordering: auth wins over generic keywords.
		{"auth_beats_build", ExecuteBeadOutcomeTaskFailed, 1,
			"401 unauthorized: build failed", FailureModeAuthError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyFailureMode(tc.outcome, tc.exitCode, tc.errMsg)
			if got != tc.want {
				t.Fatalf("ClassifyFailureMode(%q, %d, %q) = %q, want %q",
					tc.outcome, tc.exitCode, tc.errMsg, got, tc.want)
			}
		})
	}
}

// TestFailureModeLandingRefines exercises classifyLandingFailureMode, which
// is how ApplyLandingToResult folds landing-level signals (merge conflict,
// gate failure) into the final failure_mode. A clean merge clears the
// worker's mode; merge conflict and gate failure override it; other
// preserved reasons defer to the worker's classification.
func TestFailureModeLandingRefines(t *testing.T) {
	gateFailTest := []GateCheckResult{{
		DefinitionID: "go-test",
		Required:     true,
		Status:       "fail",
		Stderr:       "--- FAIL: TestX",
	}}
	gateFailBuild := []GateCheckResult{{
		DefinitionID: "go-build",
		Required:     true,
		Status:       "fail",
		Stderr:       "undefined: foo.Bar",
	}}

	cases := []struct {
		name       string
		outcome    string
		reason     string
		gates      []GateCheckResult
		workerMode string
		want       string
	}{
		{"merged_clears_worker_mode", "merged", "", nil, FailureModeUnknown, ""},
		{"no_changes_outcome", "no-changes", "agent made no commits", nil, "", FailureModeNoChanges},

		{"preserved_merge_conflict", "preserved", "merge conflict", nil, "", FailureModeMergeConflict},
		{"preserved_ff_not_possible", "preserved", "ff-merge not possible", nil, "", FailureModeMergeConflict},
		{"preserved_merge_failed", "preserved", "merge failed", nil, "", FailureModeMergeConflict},

		{"preserved_post_run_build", "preserved", "post-run checks failed", gateFailBuild, "", FailureModeBuildFailure},
		{"preserved_post_run_test", "preserved", "post-run checks failed", gateFailTest, "", FailureModeTestFailure},
		{"preserved_post_run_empty_defaults_to_test", "preserved", "post-run checks failed", nil, "", FailureModeTestFailure},

		{"preserved_agent_execution_failed_keeps_worker", "preserved", "agent execution failed", nil, FailureModeContextOverflow, FailureModeContextOverflow},
		{"preserved_no_merge_keeps_worker", "preserved", "--no-merge specified", nil, FailureModeTimeout, FailureModeTimeout},
		{"preserved_unknown_reason_falls_back_to_unknown", "preserved", "something else", nil, "", FailureModeUnknown},

		{"error_keeps_worker_mode", "error", "agent execution failed", nil, FailureModeAuthError, FailureModeAuthError},
		{"error_unknown_when_worker_empty", "error", "agent execution failed", nil, "", FailureModeUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyLandingFailureMode(tc.outcome, tc.reason, tc.gates, tc.workerMode)
			if got != tc.want {
				t.Fatalf("classifyLandingFailureMode(%q, %q, %d-gates, %q) = %q, want %q",
					tc.outcome, tc.reason, len(tc.gates), tc.workerMode, got, tc.want)
			}
		})
	}
}

// TestFailureModeApplyLandingToResult verifies that ApplyLandingToResult
// writes the refined failure_mode onto the result, giving callers a single
// unified record. This is the contract callers of result.json depend on.
func TestFailureModeApplyLandingToResult(t *testing.T) {
	// Worker recorded a context overflow; landing then merged the result
	// cleanly (unlikely but possible if the worker bounced mid-run and a
	// retry landed). The landing decision wins: failure_mode clears.
	res := &ExecuteBeadResult{
		BeadID:      "ddx-test",
		BaseRev:     "a",
		ResultRev:   "b",
		ExitCode:    0,
		Outcome:     ExecuteBeadOutcomeTaskSucceeded,
		FailureMode: FailureModeContextOverflow, // carried from a prior attempt
	}
	ApplyLandingToResult(res, &BeadLandingResult{Outcome: "merged"})
	if res.FailureMode != "" {
		t.Fatalf("merged landing should clear FailureMode, got %q", res.FailureMode)
	}

	// Worker succeeded; landing could not merge. FailureMode becomes
	// merge_conflict so measurement can bucket the failure.
	res = &ExecuteBeadResult{
		BeadID:      "ddx-test",
		BaseRev:     "a",
		ResultRev:   "b",
		ExitCode:    0,
		Outcome:     ExecuteBeadOutcomeTaskSucceeded,
		FailureMode: "",
	}
	ApplyLandingToResult(res, &BeadLandingResult{Outcome: "preserved", Reason: "merge conflict"})
	if res.FailureMode != FailureModeMergeConflict {
		t.Fatalf("merge conflict landing should set merge_conflict, got %q", res.FailureMode)
	}

	// Worker succeeded; gate failed with a build diagnostic in stderr.
	res = &ExecuteBeadResult{
		BeadID:    "ddx-test",
		BaseRev:   "a",
		ResultRev: "b",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
	}
	ApplyLandingToResult(res, &BeadLandingResult{
		Outcome: "preserved",
		Reason:  "post-run checks failed",
		GateResults: []GateCheckResult{{
			DefinitionID: "go-build",
			Required:     true,
			Status:       "fail",
			Stderr:       "undefined: foo.Bar",
		}},
	})
	if res.FailureMode != FailureModeBuildFailure {
		t.Fatalf("build-diagnostic gate failure should set build_failure, got %q", res.FailureMode)
	}

	// No-changes landing outcome always maps to no_changes regardless of
	// what the worker recorded.
	res = &ExecuteBeadResult{
		BeadID:      "ddx-test",
		BaseRev:     "a",
		ResultRev:   "a",
		ExitCode:    0,
		Outcome:     ExecuteBeadOutcomeTaskNoChanges,
		FailureMode: FailureModeNoChanges,
	}
	ApplyLandingToResult(res, &BeadLandingResult{Outcome: "no-changes", Reason: "agent made no commits"})
	if res.FailureMode != FailureModeNoChanges {
		t.Fatalf("no-changes landing should set no_changes, got %q", res.FailureMode)
	}
}

---
ddx:
  id: TP-019
  depends_on:
    - FEAT-019
    - FEAT-006
    - SD-023
---
# Test Plan: Agent Evaluation and Prompt Comparison (FEAT-019)

**Design authority:** [`SD-023`](../../02-design/solution-designs/SD-023-agent-evaluation.md)
defines the comparison isolation, grading, benchmark, and replay architecture
validated by this plan.

## Test Layers

### Layer 1 — DDx Consumer Of Fizeau (unit, in-process)

These tests use a fake of the pinned public Fizeau service contract. The fake
implements `Execute(context.Context, ServiceExecuteRequest)
(<-chan ServiceEvent, error)` and emits a public final event carrying
`ServiceFinalData`. DDx does not embed a virtual provider, concrete harness,
tool loop, or session logger. No subprocess, git operation, or network call
occurs in this layer.

| ID | Test | What It Proves |
|----|------|----------------|
| F-01 | `TestEvaluationRequestUsesFizeauContract` | DDx sends prompt, worktree, correlation, and abstract power facts through one Fizeau request |
| F-02 | `TestEvaluationToolAndSessionArtifactsRemainOpaque` | DDx retains the Fizeau session-log reference without executing tools or parsing session/tool logs |
| F-03 | `TestEvaluationReviewRaisesMinPowerWithoutRoutePins` | Stronger grading/review intent raises `MinPower` while harness/provider/model/policy remain unset |
| F-04 | `TestEvaluationCancelUsesExecuteContext` | DDx cancels the context supplied to `Execute`; Fizeau owns the session/process tree and terminates the stream according to the pinned contract |
| F-05 | `TestEvaluationFinalFailurePreserved` | A public Fizeau final failure is stored separately from the DDx comparison/grade outcome |
| F-06 | `TestEvaluationSessionEvidenceIsEnvelopeOnly` | DDx stores request/correlation fields plus opaque Fizeau session refs, not a second session-log schema |
| F-07 | `TestEvaluationDoesNotOriginateHarnessProviderModel` | With no explicit operator pins, DDx leaves concrete routing fields empty |
| F-08 | `TestEvaluationUsageComesFromFizeauOutcome` | Usage/cost and audit-only actual model are copied from public final fields without provider-log parsing |
| F-09 | `TestEvaluationImmediateExecuteErrorPreserved` | An immediate typed `Execute` error is preserved without inventing a final event or routing fallback |
| F-10 | `TestEvaluationGenericFinalErrorRemainsUnclassified` | Generic final `Error` text is retained but never parsed into cause, stage, retry, or escalation policy |
| F-11 | `TestEvaluationHasNoContinuationOrSessionQueryAPI` | Evaluation exposes only new `Execute` operations and context cancellation under v0.14.50 |

**Test fixture:** a Fizeau contract fake records requests and emits arbitrary
opaque non-terminal payloads followed by the real public final shape:

```go
fakeFizeau.ExecuteFn = func(
    ctx context.Context,
    req fizeau.ServiceExecuteRequest,
) (<-chan fizeau.ServiceEvent, error) {
    input, output := 100, 25
    payload, _ := json.Marshal(fizeau.ServiceFinalData{
        Status: "success",
        Usage: &fizeau.ServiceFinalUsage{
            InputTokens: &input, OutputTokens: &output,
        },
        SessionLogPath: "sessions/session-1.jsonl",
    })
    events := make(chan fizeau.ServiceEvent, 1)
    events <- fizeau.ServiceEvent{
        Type: fizeau.ServiceEventTypeFinal, Data: payload,
    }
    close(events)
    return events, nil
}
```

The fake does not model Fizeau's tool/session loop. That runtime behavior is
covered by Fizeau's CONTRACT-003 conformance tests, not duplicated in DDx.

### Layer 2 — Comparison Dispatch (needs temp git repos)

These tests create real git repos in `t.TempDir()`, exercise worktree
lifecycle, and verify side-effect capture. Moderate speed (git operations).

| ID | Test | What It Proves |
|----|------|----------------|
| C-01 | `TestCompareCreatesWorktrees` | The comparison workflow creates one DDx-owned worktree per arm under an arm-id-keyed path, independent of Fizeau's selected route |
| C-02 | `TestCompareArmsIsolated` | File written by arm A does not appear in arm B's worktree |
| C-03 | `TestCompareCapturesDiff` | After a Fizeau session changes the arm worktree, DDx captures the expected repository diff |
| C-04 | `TestCompareEmptyDiff` | Arm that produces no file changes records empty diff string |
| C-05 | `TestCompareCleansUpWorktrees` | After comparison, worktrees are removed (default behavior) |
| C-06 | `TestCompareKeepSandbox` | --keep-sandbox preserves worktrees; they exist after the run |
| C-07 | `TestCompareParallelExecution` | Two arms run concurrently (verify via timing or sync primitives) |
| C-08 | `TestCompareRecordSchema` | `ComparisonRecord` contains arm id, DDx request facts, explicit operator passthrough if any, Fizeau-returned route/usage/final facts, opaque session-log reference, diff, and grade |
| C-09 | `TestCompareArmFailure` | If one Fizeau session fails, comparison still completes with the public final outcome in that arm's record |
| C-10 | `TestComparePostRun` | --post-run command executes in each worktree; pass/fail captured |
| C-11 | `TestComparePostRunFailure` | Post-run failure recorded but doesn't abort the comparison |
| C-12 | `TestCompareArmIdentityIgnoresRoutingActual` | Different returned routes cannot relabel, merge, split, rank, or filter logical arms |
| C-13 | `TestCompareOperatorPinsAreIdenticalAndExcluded` | One explicit operator envelope is copied unchanged to every arm and excluded from arm identity, grouping, grading, and comparison policy |

**Test scaffold — temp git repo:**

```go
func setupTestRepo(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    run(t, dir, "git", "init")
    run(t, dir, "git", "commit", "--allow-empty", "-m", "init")
    // Write a seed file so diffs are meaningful
    os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
    run(t, dir, "git", "add", ".")
    run(t, dir, "git", "commit", "-m", "seed")
    return dir
}
```

All comparison tests use the Fizeau contract fake. They do not install a DDx
virtual harness, mock a concrete harness executor, or encode a provider stream.
When a test needs a repository side effect, the fake's test callback edits the
arm worktree as the service boundary's observable effect.

### Layer 3 — Grading (Fizeau contract fake, canned grades)

Grading sends a comparison record through the same Fizeau contract and consumes
the application-level grader response from `ServiceFinalData.FinalText`. DDx
may parse that declared result as the grade while keeping the native session log
and non-terminal service events opaque. DDx may request a stronger abstract
`MinPower`, but it never selects a grader harness/provider/model. Tests use the
Fizeau contract fake with canned final events.

| ID | Test | What It Proves |
|----|------|----------------|
| G-01 | `TestGradeConstructsPrompt` | Grading prompt includes original task, each arm's output, each arm's diff |
| G-02 | `TestGradeParsesFinalText` | Fizeau `ServiceFinalData.FinalText` contains the requested JSON grade → parsed into per-arm score/pass/rationale |
| G-03 | `TestGradeAttachesToRecord` | Grade is written to the DDx comparison record while the Fizeau session log remains opaque |
| G-04 | `TestGradeCustomRubric` | --rubric file content replaces the default grading template |
| G-05 | `TestGradeMalformedResponse` | Non-JSON grader output → graceful error, comparison record not corrupted |
| G-06 | `TestGradeGraderFailure` | Fizeau returns a public final failure → error recorded, existing arms untouched |
| G-07 | `TestGradePromptExcludesRoutingActual` | Concrete harness/provider/model and native session-log content never enter a grading prompt or score |

**Test fixture — canned grade:**

```go
final := fizeau.ServiceFinalData{
    Status: "success",
    FinalText: `{"arms":[{"arm":"arm-1","score":8,"max_score":10,` +
        `"pass":true,"rationale":"Correct"}]}`,
}
```

### Layer 4 — Integration (real Fizeau service, skip-if-unavailable)

These tests call a configured real Fizeau service and are slow. DDx never
connects to a provider or concrete harness directly. They validate the consumer
boundary end to end but are not required for CI.

| ID | Test | What It Proves |
|----|------|----------------|
| I-01 | `TestIntegration_EvaluationThroughFizeau` | DDx request → Fizeau → public final outcome/usage and opaque session-log reference |
| I-02 | `TestIntegration_CompareArmsThroughFizeau` | Multiple arm requests go through Fizeau, produce isolated diffs, and record returned route facts without DDx selecting them |
| I-03 | `TestIntegration_GradeThroughFizeau` | Grading uses a stronger abstract `MinPower` with no DDx-originated harness/provider/model |

```go
func TestIntegration_EvaluationThroughFizeau(t *testing.T) {
    if !fizeauTestServiceAvailable() {
        t.Skip("Fizeau integration service not available")
    }
    // ...
}
```

### Layer 5 — Replay And Benchmark Policy

Replay and benchmark tests operate on preserved DDx request, revision,
`FinalText`, repository, check, and grade evidence. They may carry an opaque
`SessionLogPath` as a reference but never open it to reconstruct an input.

| ID | Test | What It Proves |
|----|------|----------------|
| R-01 | `TestReplayReconstructsFromDDxEvidence` | Replay builds a new request from DDx-owned evidence and calls `Execute` once |
| R-02 | `TestReplayDoesNotParseSessionLog` | Missing or unreadable native logs do not change the replay request |
| R-03 | `TestReplayIgnoresPriorRoutingActual` | A prior concrete route is absent from replay inputs unless it was an explicit unchanged operator pin |
| B-01 | `TestBenchmarkGroupsByDeclaredInput` | Benchmark keys use prompt/rubric/work facts/`MinPower`, never returned route identity |
| B-02 | `TestRoutingActualCannotDriveRankingOrWarning` | Changing only returned harness/provider/model cannot change aggregate grade, rank, warning, or policy |
| B-03 | `TestStrongerReviewRaisesOnlyMinPower` | Reviewer intent raises abstract `MinPower` while operator `MaxPower` and pins remain unchanged |

## Side-Effect Capture: What to Test

The boundary exposes two ownership classes:

| Signal | Owner | DDx behavior |
|--------|-------|--------------|
| Git diff / result revision | DDx | Compute from the arm worktree and store in comparison evidence |
| Repository gates | DDx | Execute and store structured pass/fail evidence |
| Final session outcome and usage | Fizeau | Copy public `ServiceFinalData` fields into the DDx envelope |
| Tool/session/progress logs | Fizeau | Retain live public display events only while observed and store the opaque `SessionLogPath`; do not query or interpret native history |

Tests verify DDx captures repository effects for every arm (C-03, C-04) and
does not parse a Fizeau session log to reconstruct tool, bash, or file-read
events (F-02). Unknown non-terminal event payloads pass through unchanged.

## Sandboxing Edge Cases

| Case | Expected Behavior | Test |
|------|-------------------|------|
| Arm deletes a file | Diff shows deletion; other arm still has the file | C-02 |
| Arm creates files in subdirectory | Diff captures new directory + files | C-03 |
| Arm runs `git commit` | Diff is empty (changes committed); output captures the commit | C-04 variant |
| Worktree creation fails (dirty repo) | Clear error before arms start | C-01 variant |
| Arm panics/crashes | Worktree still cleaned up; arm marked as error | C-09 |
| Two comparisons run simultaneously | Each gets unique worktree names (compare-<id>-) | C-01 |

## Test Data: Prompts for Comparison Tests

Rather than using trivial prompts, comparison tests should use prompts
that produce predictable side effects with the Fizeau contract fake:

```
prompt: "Create a file called result.txt containing 'hello world'"
```

Configure the Fizeau contract fake's side-effect callback to write the file in
the arm worktree, then return an opaque session-log reference and public final
event. DDx observes the repository diff without knowing which harness or tool
produced it.

This keeps tests deterministic while exercising realistic diff capture.

## Dependencies on Unbuilt Code

Tests in layers 2-3 depend on code that doesn't exist yet:

- `Runner.RunCompare(opts CompareOptions) (*ComparisonRecord, error)`
- `ComparisonRecord` type with DDx arm evidence, Fizeau final envelopes,
  opaque session-log references, diffs, and grades
- `Runner.Grade(comparisonID string, minPower int, rubric string) error` with
  optional explicit operator passthrough carried separately
- Worktree creation/cleanup for comparison arms
- Diff capture utility: `captureWorktreeDiff(worktreePath string) (string, error)`

Layer 1 tests (F-01 through F-09) should target the FEAT-006 Fizeau consumer
adapter. Layers 2-3 should be written alongside the comparison implementation.

## Running the Tests

```bash
# Unit/contract-fake tests only (fast, no external deps)
cd cli && go test ./internal/agent/... -run 'TestEvaluation|TestCompare|TestGrade|TestReplay|TestBenchmark|TestRoutingActual|TestStrongerReview' -count=1

# Real Fizeau consumer integration; skips when the configured service is absent
cd cli && go test ./internal/agent/... -run 'TestIntegration_.*ThroughFizeau' -v -timeout 120s

# Owning package and repository gates
cd cli && go test ./internal/agent/...
lefthook run pre-commit
```

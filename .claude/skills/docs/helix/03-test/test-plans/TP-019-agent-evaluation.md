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

### Layer 1 — DDx Agent Executor (unit, in-process)

These tests use DDx Agent's virtual provider for deterministic replay. No
subprocess, no git, no network. All run in `internal/agent/`.

| ID | Test | What It Proves |
|----|------|----------------|
| F-01 | `TestAgentRunVirtualProvider` | RunAgent dispatches to the embedded agent library with virtual provider, returns typed Result with tokens/cost/model |
| F-02 | `TestAgentRunToolExecution` | DDx Agent tools (read/write/edit/bash) execute in WorkDir and tool calls appear in session log |
| F-03 | `TestAgentRunIterationLimit` | MaxIterations caps the loop; Result.Status = iteration_limit, ExitCode = 1 |
| F-04 | `TestAgentRunTimeout` | Context timeout cancels the run; Result maps to timeout error |
| F-05 | `TestAgentRunProviderError` | Provider returns error → Result.Error populated, ExitCode = 1 |
| F-06 | `TestAgentRunSessionLogging` | Session entry written to sessions.jsonl with correct harness, tokens, cost |
| F-07 | `TestAgentRunModelResolution` | Model from opts > config > env > provider default, in that priority |
| F-08 | `TestAgentRunCostMapping` | CostUSD = 0 for local models, -1 mapped to 0 in Result (unknown model) |

**Test fixture:** The virtual provider can be configured with inline
responses that include tool calls, allowing deterministic multi-turn
tests without any LLM:

```go
virtual.New(virtual.Config{
    InlineResponses: []virtual.InlineResponse{{
        PromptMatch: "create hello.txt",
        Response: agentlib.Response{
            Content: "",
            ToolCalls: []agentlib.ToolCall{{
                Name: "write", Arguments: `{"path":"hello.txt","content":"hello"}`,
            }},
        },
    }},
})
```

This exercises the full DDx Agent loop (prompt → tool call → tool result →
next LLM turn → final response) without network or cost.

### Layer 2 — Comparison Dispatch (needs temp git repos)

These tests create real git repos in `t.TempDir()`, exercise worktree
lifecycle, and verify side-effect capture. Moderate speed (git operations).

| ID | Test | What It Proves |
|----|------|----------------|
| C-01 | `TestCompareCreatesWorktrees` | --compare creates one worktree per harness arm under `.worktrees/compare-<id>-<harness>/` |
| C-02 | `TestCompareArmsIsolated` | File written by arm A does not appear in arm B's worktree |
| C-03 | `TestCompareCapturesDiff` | After DDx Agent writes a file, the effect diff contains the expected unified diff |
| C-04 | `TestCompareEmptyDiff` | Arm that produces no file changes records empty diff string |
| C-05 | `TestCompareCleansUpWorktrees` | After comparison, worktrees are removed (default behavior) |
| C-06 | `TestCompareKeepSandbox` | --keep-sandbox preserves worktrees; they exist after the run |
| C-07 | `TestCompareParallelExecution` | Two arms run concurrently (verify via timing or sync primitives) |
| C-08 | `TestCompareRecordSchema` | ComparisonRecord contains all expected fields: id, timestamp, prompt, arms[] with harness/model/output/diff/tokens/cost |
| C-09 | `TestCompareArmFailure` | If one arm fails, comparison still completes with error in that arm's record |
| C-10 | `TestComparePostRun` | --post-run command executes in each worktree; pass/fail captured |
| C-11 | `TestComparePostRunFailure` | Post-run failure recorded but doesn't abort the comparison |

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

All comparison tests use the virtual provider (DDx Agent side) and
mockExecutor (subprocess side) — no real LLM calls.

### Layer 3 — Grading (virtual harness, canned grades)

Grading sends a comparison record to a harness and parses the structured
response. Tests use the DDx virtual harness (not DDx Agent virtual provider)
with inline responses.

| ID | Test | What It Proves |
|----|------|----------------|
| G-01 | `TestGradeConstructsPrompt` | Grading prompt includes original task, each arm's output, each arm's diff |
| G-02 | `TestGradeParsesResponse` | Virtual harness returns JSON grade → parsed into per-arm score/pass/rationale |
| G-03 | `TestGradeAttachesToRecord` | Grade is written back to the comparison record in session log |
| G-04 | `TestGradeCustomRubric` | --rubric file content replaces the default grading template |
| G-05 | `TestGradeMalformedResponse` | Non-JSON grader output → graceful error, comparison record not corrupted |
| G-06 | `TestGradeGraderFailure` | Grading harness returns exit_code=1 → error recorded, existing arms untouched |

**Test fixture — canned grade:**

```go
t.Setenv("DDX_VIRTUAL_RESPONSES", `[{
    "prompt_match": "Grade the following",
    "response": "{\"arms\":[{\"arm\":\"agent\",\"score\":8,\"max_score\":10,\"pass\":true,\"rationale\":\"Correct\"}]}"
}]`)
```

### Layer 4 — Integration (real models, skip-if-unavailable)

These tests hit real providers and are slow. They validate end-to-end
but are not required for CI.

| ID | Test | What It Proves |
|----|------|----------------|
| I-01 | `TestIntegration_AgentLocalModel` | DDx Agent → LM Studio (localhost:1234) → real model response with tokens |
| I-02 | `TestIntegration_CompareAgentVsClaude` | Full comparison: agent arm + claude arm, both produce diffs, comparison record complete |
| I-03 | `TestIntegration_GradeWithClaude` | Grade a comparison using real claude; structured grade returned |

```go
func TestIntegration_AgentLocalModel(t *testing.T) {
    // Skip if LM Studio not reachable
    if _, err := net.DialTimeout("tcp", "localhost:1234", 2*time.Second); err != nil {
        t.Skip("LM Studio not available on localhost:1234")
    }
    // ...
}
```

## Side-Effect Capture: What to Test

The key insight is that **DDx Agent gives us two levels of side-effect data**
while subprocess harnesses give us only one:

| Signal | DDx Agent | subprocess (codex/claude/opencode) |
|--------|-------|-----------------------------------|
| Git diff (after) | ✓ | ✓ |
| Tool call log (during) | ✓ (typed ToolCallLog[]) | ✗ |
| Bash command output | ✓ (in tool call log) | ✗ |
| Files read | ✓ (in tool call log) | ✗ |

Tests should verify:
- **Diff capture** works for both DDx Agent and subprocess arms (C-03, C-04)
- **Tool call log** is populated for DDx Agent arms only (F-02)
- **Missing tool log** for subprocess arms is nil, not empty (C-08)

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
that produce predictable side effects with the virtual/mock providers:

```
prompt: "Create a file called result.txt containing 'hello world'"
```

For DDx Agent (virtual provider): configure response with a write tool call.
For subprocess (mockExecutor): configure output string, manually seed the
file in the worktree to simulate the effect.

This keeps tests deterministic while exercising realistic diff capture.

## Dependencies on Unbuilt Code

Tests in layers 2-3 depend on code that doesn't exist yet:

- `Runner.RunCompare(opts CompareOptions) (*ComparisonRecord, error)`
- `ComparisonRecord` type with arms, diffs, grades
- `Runner.Grade(comparisonID string, graderHarness string, rubric string) error`
- Worktree creation/cleanup for comparison arms
- Diff capture utility: `captureWorktreeDiff(worktreePath string) (string, error)`

Layer 1 tests (F-01 through F-08) can be written now against the
existing `RunAgent` method. Layers 2-3 should be written alongside
the implementation.

## Running the Tests

```bash
# Unit tests only (fast, no external deps)
cd cli && go test ./internal/agent/ -run "^Test[^I]" -count=1

# Include DDx Agent virtual provider tests
cd cli && go test ./internal/agent/ -run "TestAgentRun" -v

# Integration tests (needs LM Studio on localhost:1234)
cd cli && go test ./internal/agent/ -run "TestIntegration_Agent" -v -timeout 120s

# Integration tests (needs LM Studio on vidar:1234)
AGENT_BASE_URL=http://vidar:1234/v1 go test ./internal/agent/ -run "TestIntegration_Agent" -v
```

<bead-review>
  <bead id="ddx-4a751095" iter=1>
    <title>execute-loop: tighten already_satisfied gate to require AC structural test name match</title>
    <description>
Observed across multiple beads in agent v0.9.9 / v0.9.10 work (2026-04-25): worker classifies bead as already_satisfied when the regression test suite passes, even when the bead's acceptance criteria explicitly name structural test functions or properties that the implementation has not exercised.

## Concrete instances

- agent-9d120ece (route resolution): AC #1 was structural ('cmd/agent no longer contains provider-building helpers or route-provider wrapper types that depend on internal/core'). Regression tests passed because the AC also named a regression command. Bead closed already_satisfied — but cmd/agent/routing_provider.go and cmd/agent/provider_build.go still contained the forbidden helpers. False close.
- agent-1023f072 (boundary tightening): same pattern earlier — AC structural property unmet, regression command passing, close.
- agent-d9c358ba (smart-routing wiring): worker analysis correctly identified the bead was too broad and split it. Status still classified already_satisfied which is misleading — 'split deferred' is a different outcome from 'AC was already met.'

## Principle

already_satisfied should mean: 'the AC's specific test functions or named structural properties were exercised and produced the asserted outcome without any code change in this attempt.' It should NOT mean: 'some unrelated regression command passed.'

If the AC names test functions (e.g. 'go test . -run TestX'), those tests must have run during the worker's verification step. If the AC names a structural property (e.g. 'no field X', 'file Y deleted', 'ProviderEntry.SupportsTools is OR-permissive'), the property must be checked.

## Suggested fix

Parse the bead's acceptance criteria during worker verification. Extract:
- Named test function references ('TestFoo', 'go test . -run TestFoo')
- File-existence claims ('file X deleted', 'no longer contains')
- AST-level claims ('field X removed', 'no import of Y')

For already_satisfied to be valid:
- All named test functions must exist and pass.
- All file-existence claims must hold via filesystem check.
- AST-level claims must hold via parser pass.

If any check fails, classify as no_changes (or a new outcome class like ac_unmet_no_changes) so the loop unclaims and tries again rather than closing the bead.

## Out of scope

- Changes to the success / land_conflict / execution_failed / review_request_changes outcome classes.
- Adding new structural-claim parsers beyond what the AC explicitly names.
- Automatic AC-claim extraction beyond test names + file paths + simple AST patterns. Free-form prose stays operator-judgment territory.

## Cross-reference

Discipline rule documented in agent repo AGENTS.md 'Review and Verification Discipline' section. Local mitigation (verify before accept, reopen with notes) caught three false closes in this session.
    </description>
    <acceptance>
1. New test cd cli &amp;&amp; go test ./internal/agent/... -run TestAlreadySatisfiedRequiresAcStructuralCheck covers: bead with AC naming TestX; worker run does not exercise TestX; classification is NOT already_satisfied (instead no_changes or new ac_unmet class).
2. Test covers: bead with AC naming a deleted file path; file still exists; classification is NOT already_satisfied.
3. Test covers: bead with AC asserting a struct does not have field Y; field still exists; classification is NOT already_satisfied.
4. Test covers: positive case — bead with AC naming TestX; worker run exercises TestX and TestX passes; AC structural property holds; classification IS already_satisfied (no regression).
5. cd cli &amp;&amp; go test ./... -count=1 passes.
    </acceptance>
    <labels>ddx, kind:bug, area:agent, area:execute-loop, area:harness</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260426T111547-9d695f8c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5c8d9944ba65f86202c18b34a6b0dcc510c1f530">
diff --git a/.ddx/executions/20260426T111547-9d695f8c/result.json b/.ddx/executions/20260426T111547-9d695f8c/result.json
new file mode 100644
index 00000000..d93b7c09
--- /dev/null
+++ b/.ddx/executions/20260426T111547-9d695f8c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-4a751095",
+  "attempt_id": "20260426T111547-9d695f8c",
+  "base_rev": "332e9f6038879649b52c7a7af0a5e5c56187df65",
+  "result_rev": "28077ad8375a6824168f62d4e1bfc899a75c1f8d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-05bd87e1",
+  "duration_ms": 633475,
+  "tokens": 28365,
+  "cost_usd": 3.303562250000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260426T111547-9d695f8c",
+  "prompt_file": ".ddx/executions/20260426T111547-9d695f8c/prompt.md",
+  "manifest_file": ".ddx/executions/20260426T111547-9d695f8c/manifest.json",
+  "result_file": ".ddx/executions/20260426T111547-9d695f8c/result.json",
+  "usage_file": ".ddx/executions/20260426T111547-9d695f8c/usage.json",
+  "started_at": "2026-04-26T11:15:48.016541879Z",
+  "finished_at": "2026-04-26T11:26:21.492121652Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>

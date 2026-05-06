<bead-review>
  <bead id="ddx-958b8fc3" iter=1>
    <title>bead/axon: parameterize conformance suites across JSONL and Axon</title>
    <description>
PROBLEM
The package-level conformance coverage still needs a single matrix that proves both backends satisfy the same behavioral contract. The current chaos harness already names jsonl and axon in cli/internal/bead/chaos_test.go:23-36, but the rest of the package still relies on ad hoc store construction and has no package-wide backend matrix to prove the new Axon path behaves identically.

ROOT CAUSE
cli/internal/bead/chaos_test.go:23-36 only defines the backend table; there is no end-to-end package test matrix yet that runs the conformance suite through both backends and keeps the existing package tests honest. The rest of the test package still defaults to newTestStore(t) in many places, so the Axon path is not yet a first-class test target.

PROPOSED FIX
- Extend the backend test helpers so the package conformance suite can run under both jsonl and axon.
- Keep TestChaos_ConcurrentAppendSafety, TestChaos_AtomicStatusTransitions, TestChaos_ConcurrentCloseAndAppend, and TestChaos_ConcurrentCloseNotLost as explicit backend matrix coverage.
- Make the package test harness clearly fail if either backend diverges.

NON-SCOPE
- GraphQL client generation.
- Backend selection/config plumbing.
- Subscription integration smoke tests.
    </description>
    <acceptance>
1. TestChaos_ConcurrentAppendSafety, TestChaos_AtomicStatusTransitions, TestChaos_ConcurrentCloseAndAppend, and TestChaos_ConcurrentCloseNotLost run under both jsonl and axon subtests.
2. The package conformance helpers make the backend selector explicit instead of silently defaulting in only one path.
3. cd cli &amp;&amp; go test ./internal/bead/... passes.
4. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:feature, blocked-on-upstream:axon-05c1019d, blocked-on-upstream:axon-82b6f7b2</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T193917-468fb55f/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T193917-468fb55f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ab4c765abea1c30c32ff594a2cf94a2fba949b90">
<untrusted-data>
diff --git a/.ddx/executions/20260506T193917-468fb55f/checks/production-reachability.json b/.ddx/executions/20260506T193917-468fb55f/checks/production-reachability.json
new file mode 100644
index 000000000..89e732516
--- /dev/null
+++ b/.ddx/executions/20260506T193917-468fb55f/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no non-test Go files changed"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T193917-468fb55f/result.json b/.ddx/executions/20260506T193917-468fb55f/result.json
new file mode 100644
index 000000000..5cd884f88
--- /dev/null
+++ b/.ddx/executions/20260506T193917-468fb55f/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-958b8fc3",
+  "attempt_id": "20260506T193917-468fb55f",
+  "base_rev": "71b48b6be6af9e828c5eacf04b5f6de8d719a092",
+  "result_rev": "73f5fed5626a7ec8959d9ce9fe455259086148d6",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-9e0335ed",
+  "duration_ms": 190625,
+  "tokens": 2234390,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T193917-468fb55f",
+  "prompt_file": ".ddx/executions/20260506T193917-468fb55f/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T193917-468fb55f/manifest.json",
+  "result_file": ".ddx/executions/20260506T193917-468fb55f/result.json",
+  "usage_file": ".ddx/executions/20260506T193917-468fb55f/usage.json",
+  "started_at": "2026-05-06T19:39:20.732087703Z",
+  "finished_at": "2026-05-06T19:42:31.357921034Z"
+}
\ No newline at end of file
</untrusted-data>
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

<bead-review>
  <bead id="ddx-a458af7c" iter=1>
    <title>execute-loop: push_failed should NOT set a 1-year cooldown — race-with-remote is recoverable</title>
    <description>
Today, when execute-bead lands locally but push fails (e.g. non-fast-forward because remote advanced), the loop sets execute-loop-retry-after to ONE YEAR in the future. This is wrong on multiple axes:

1. Push race is fundamentally recoverable: pull/merge/push is the standard fix and takes seconds.
2. A 1-year cooldown means the bead never re-runs in any reasonable working horizon.
3. The work is already done locally — the bead is functionally complete; reopening it for a year is misleading.

Observed in this session on ddx-98e6e9ef (routing-preflight-gate): worker landed commit 978131c6, push rejected because origin/main had advanced (687fb4ca + 4a6c6e8a pushed remotely between the local land and the push). Loop set retry_after=2027-04-29T03:43:59Z. Operator had to manually merge, push, and clear the cooldown.

What is needed:

1. push_failed should auto-pull/merge/retry-push at least once before giving up. The merge case where origin advanced with non-overlapping work is the common case in a multi-worker / multi-operator setup.

2. If the auto-merge has conflicts that the loop cannot resolve, push_failed should set a SHORT cooldown (5-15 min), not 1 year. The operator just needs time to resolve the conflict.

3. push_failed-with-conflicts should mark the bead as needing-human-review (a structured outcome similar to declined_needs_decomposition from ddx-fba752b9), not as a generic execution failure.

4. The loop's cooldown duration should NEVER exceed some reasonable cap (e.g. 24h) for any outcome. Year-scale cooldowns mean 'never' and that should be a deliberate operator choice via 'ddx bead update --set execute-loop-retry-after=...', not an automatic loop decision.
    </description>
    <acceptance>
1. push_failed flow attempts auto pull --rebase / merge / retry-push at least once before exiting with push_failed. 2. push_failed cooldown caps at 24h regardless of underlying error. 3. push_failed-with-conflicts (auto-merge fails) emits a structured outcome distinct from generic execution_failed; bead is parked for human review with the conflict context recorded as a structured event (kind:push-conflict). 4. Regression test: a fake git that returns non-fast-forward on first push, then fast-forward after pull/merge, exercises the new flow and asserts the bead lands cleanly without operator intervention. 5. Regression test: a fake git that produces an unresolvable merge conflict exercises the structured-outcome path and asserts cooldown &lt;= 24h. 6. CHANGELOG entry. 7. Update ddx-fba752b9's CHANGELOG section (or a new section) noting that the 24h cap applies to all loop-set cooldowns, not just push failures.
    </acceptance>
    <labels>execute-loop, beads, quality-of-life</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T065532-bc1fd37e/manifest.json</file>
    <file>.ddx/executions/20260429T065532-bc1fd37e/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="ae1ab304e7da3abdae214b2fe3abd0e95788b4ac">
diff --git a/.ddx/executions/20260429T065532-bc1fd37e/manifest.json b/.ddx/executions/20260429T065532-bc1fd37e/manifest.json
new file mode 100644
index 00000000..a30f8a1d
--- /dev/null
+++ b/.ddx/executions/20260429T065532-bc1fd37e/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T065532-bc1fd37e",
+  "bead_id": "ddx-a458af7c",
+  "base_rev": "62a923eefbf74b007e2882d0b3f2242d798b2717",
+  "created_at": "2026-04-29T06:55:32.958685431Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-a458af7c",
+    "title": "execute-loop: push_failed should NOT set a 1-year cooldown — race-with-remote is recoverable",
+    "description": "Today, when execute-bead lands locally but push fails (e.g. non-fast-forward because remote advanced), the loop sets execute-loop-retry-after to ONE YEAR in the future. This is wrong on multiple axes:\n\n1. Push race is fundamentally recoverable: pull/merge/push is the standard fix and takes seconds.\n2. A 1-year cooldown means the bead never re-runs in any reasonable working horizon.\n3. The work is already done locally — the bead is functionally complete; reopening it for a year is misleading.\n\nObserved in this session on ddx-98e6e9ef (routing-preflight-gate): worker landed commit 978131c6, push rejected because origin/main had advanced (687fb4ca + 4a6c6e8a pushed remotely between the local land and the push). Loop set retry_after=2027-04-29T03:43:59Z. Operator had to manually merge, push, and clear the cooldown.\n\nWhat is needed:\n\n1. push_failed should auto-pull/merge/retry-push at least once before giving up. The merge case where origin advanced with non-overlapping work is the common case in a multi-worker / multi-operator setup.\n\n2. If the auto-merge has conflicts that the loop cannot resolve, push_failed should set a SHORT cooldown (5-15 min), not 1 year. The operator just needs time to resolve the conflict.\n\n3. push_failed-with-conflicts should mark the bead as needing-human-review (a structured outcome similar to declined_needs_decomposition from ddx-fba752b9), not as a generic execution failure.\n\n4. The loop's cooldown duration should NEVER exceed some reasonable cap (e.g. 24h) for any outcome. Year-scale cooldowns mean 'never' and that should be a deliberate operator choice via 'ddx bead update --set execute-loop-retry-after=...', not an automatic loop decision.",
+    "acceptance": "1. push_failed flow attempts auto pull --rebase / merge / retry-push at least once before exiting with push_failed. 2. push_failed cooldown caps at 24h regardless of underlying error. 3. push_failed-with-conflicts (auto-merge fails) emits a structured outcome distinct from generic execution_failed; bead is parked for human review with the conflict context recorded as a structured event (kind:push-conflict). 4. Regression test: a fake git that returns non-fast-forward on first push, then fast-forward after pull/merge, exercises the new flow and asserts the bead lands cleanly without operator intervention. 5. Regression test: a fake git that produces an unresolvable merge conflict exercises the structured-outcome path and asserts cooldown \u003c= 24h. 6. CHANGELOG entry. 7. Update ddx-fba752b9's CHANGELOG section (or a new section) noting that the 24h cap applies to all loop-set cooldowns, not just push failures.",
+    "labels": [
+      "execute-loop",
+      "beads",
+      "quality-of-life"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T06:55:32Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T06:55:32.429721528Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T065532-bc1fd37e",
+    "prompt": ".ddx/executions/20260429T065532-bc1fd37e/prompt.md",
+    "manifest": ".ddx/executions/20260429T065532-bc1fd37e/manifest.json",
+    "result": ".ddx/executions/20260429T065532-bc1fd37e/result.json",
+    "checks": ".ddx/executions/20260429T065532-bc1fd37e/checks.json",
+    "usage": ".ddx/executions/20260429T065532-bc1fd37e/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-a458af7c-20260429T065532-bc1fd37e"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T065532-bc1fd37e/result.json b/.ddx/executions/20260429T065532-bc1fd37e/result.json
new file mode 100644
index 00000000..177d7458
--- /dev/null
+++ b/.ddx/executions/20260429T065532-bc1fd37e/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-a458af7c",
+  "attempt_id": "20260429T065532-bc1fd37e",
+  "base_rev": "62a923eefbf74b007e2882d0b3f2242d798b2717",
+  "result_rev": "b12af94207b2d37e03d79b55fa84abae829b71b4",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-d220ff36",
+  "duration_ms": 1313175,
+  "tokens": 33833,
+  "cost_usd": 6.04103525,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T065532-bc1fd37e",
+  "prompt_file": ".ddx/executions/20260429T065532-bc1fd37e/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T065532-bc1fd37e/manifest.json",
+  "result_file": ".ddx/executions/20260429T065532-bc1fd37e/result.json",
+  "usage_file": ".ddx/executions/20260429T065532-bc1fd37e/usage.json",
+  "started_at": "2026-04-29T06:55:32.958973306Z",
+  "finished_at": "2026-04-29T07:17:26.134271251Z"
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

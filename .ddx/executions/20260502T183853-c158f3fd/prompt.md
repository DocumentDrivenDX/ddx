<bead-review>
  <bead id="ddx-13e19750" iter=1>
    <title>B14.8a: Backend chaos/integration suite for federation</title>
    <description>
Go integration + chaos test suite covering: hub crash mid-fan-out (caller sees partial-result not panic), stale spoke during merged query (marked stale, not blocking), duplicate registration (idempotent + conflict path), newer-schema registration (degraded), slow-spoke timeout (per-node timeout enforced, others succeed), hub restart + immediate query (registry rebuilt from disk + repopulated by next heartbeat). Each scenario asserts both behavior and observable status transitions. Run under cli/internal/federation/ and/or cli/internal/server/ test packages.
    </description>
    <acceptance>
Integration tests exist for: hub crash mid-fan-out, stale-spoke-during-query, duplicate-registration (id-match + id-conflict), newer-schema-registration, slow-spoke-timeout, hub-restart-rebuild. All pass deterministically (no flake). Status transitions asserted at each scenario boundary.
    </acceptance>
    <labels>phase:2, story:14</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T182934-a951835d/manifest.json</file>
    <file>.ddx/executions/20260502T182934-a951835d/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2372bd02a04588444cb32a8c9d4a402d6b093a76">
diff --git a/.ddx/executions/20260502T182934-a951835d/manifest.json b/.ddx/executions/20260502T182934-a951835d/manifest.json
new file mode 100644
index 00000000..7820f990
--- /dev/null
+++ b/.ddx/executions/20260502T182934-a951835d/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T182934-a951835d",
+  "bead_id": "ddx-13e19750",
+  "base_rev": "90df2eebdadabcc354fcb34b4cb7bd776ecb9e74",
+  "created_at": "2026-05-02T18:29:35.883289226Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-13e19750",
+    "title": "B14.8a: Backend chaos/integration suite for federation",
+    "description": "Go integration + chaos test suite covering: hub crash mid-fan-out (caller sees partial-result not panic), stale spoke during merged query (marked stale, not blocking), duplicate registration (idempotent + conflict path), newer-schema registration (degraded), slow-spoke timeout (per-node timeout enforced, others succeed), hub restart + immediate query (registry rebuilt from disk + repopulated by next heartbeat). Each scenario asserts both behavior and observable status transitions. Run under cli/internal/federation/ and/or cli/internal/server/ test packages.",
+    "acceptance": "Integration tests exist for: hub crash mid-fan-out, stale-spoke-during-query, duplicate-registration (id-match + id-conflict), newer-schema-registration, slow-spoke-timeout, hub-restart-rebuild. All pass deterministically (no flake). Status transitions asserted at each scenario boundary.",
+    "parent": "ddx-a038a090",
+    "labels": [
+      "phase:2",
+      "story:14"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T18:29:34Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3358605",
+      "execute-loop-heartbeat-at": "2026-05-02T18:29:34.438325065Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T182934-a951835d",
+    "prompt": ".ddx/executions/20260502T182934-a951835d/prompt.md",
+    "manifest": ".ddx/executions/20260502T182934-a951835d/manifest.json",
+    "result": ".ddx/executions/20260502T182934-a951835d/result.json",
+    "checks": ".ddx/executions/20260502T182934-a951835d/checks.json",
+    "usage": ".ddx/executions/20260502T182934-a951835d/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-13e19750-20260502T182934-a951835d"
+  },
+  "prompt_sha": "62322c7553a8ed04e7da7360d90d3ed99e1588e331bffe59da16958c0a28d096"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T182934-a951835d/result.json b/.ddx/executions/20260502T182934-a951835d/result.json
new file mode 100644
index 00000000..df49f719
--- /dev/null
+++ b/.ddx/executions/20260502T182934-a951835d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-13e19750",
+  "attempt_id": "20260502T182934-a951835d",
+  "base_rev": "90df2eebdadabcc354fcb34b4cb7bd776ecb9e74",
+  "result_rev": "8c37523ea93e81793f10c16519fbed6c48c93e96",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-fe25403a",
+  "duration_ms": 552000,
+  "tokens": 25074,
+  "cost_usd": 2.7303847500000007,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T182934-a951835d",
+  "prompt_file": ".ddx/executions/20260502T182934-a951835d/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T182934-a951835d/manifest.json",
+  "result_file": ".ddx/executions/20260502T182934-a951835d/result.json",
+  "usage_file": ".ddx/executions/20260502T182934-a951835d/usage.json",
+  "started_at": "2026-05-02T18:29:35.88357481Z",
+  "finished_at": "2026-05-02T18:38:47.883919659Z"
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

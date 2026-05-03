<bead-review>
  <bead id="ddx-cb63cdfc" iter=1>
    <title>Migration tool: .ddx/beads.jsonl + archive → axon backend</title>
    <description>
Add a one-shot migration command (e.g. ddx bead migrate --to axon) that reads .ddx/beads.jsonl and .ddx/beads-archive.jsonl and writes them losslessly into the axon backend. Idempotent; safe to re-run. Does not delete the source files (operator removes after verification).
    </description>
    <acceptance>
1. ddx bead migrate --to axon command exists and is documented in --help. 2. After migration, every bead (including events and archive entries) round-trips: ddx bead export | diff against pre-migration export = empty. 3. Idempotent: running twice produces no duplicates. 4. Test fixture under cli/internal/bead/testdata covers a representative beads.jsonl + archive.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:tool, backend-migration</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T221716-47fdb078/manifest.json</file>
    <file>.ddx/executions/20260503T221716-47fdb078/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="58b460018431ae6beb881f50f9085acc7dae3814">
diff --git a/.ddx/executions/20260503T221716-47fdb078/manifest.json b/.ddx/executions/20260503T221716-47fdb078/manifest.json
new file mode 100644
index 00000000..d327ef11
--- /dev/null
+++ b/.ddx/executions/20260503T221716-47fdb078/manifest.json
@@ -0,0 +1,40 @@
+{
+  "attempt_id": "20260503T221716-47fdb078",
+  "bead_id": "ddx-cb63cdfc",
+  "base_rev": "823ced625935a40a12172a0a073a221ae18e6520",
+  "created_at": "2026-05-03T22:17:18.383060941Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-cb63cdfc",
+    "title": "Migration tool: .ddx/beads.jsonl + archive → axon backend",
+    "description": "Add a one-shot migration command (e.g. ddx bead migrate --to axon) that reads .ddx/beads.jsonl and .ddx/beads-archive.jsonl and writes them losslessly into the axon backend. Idempotent; safe to re-run. Does not delete the source files (operator removes after verification).",
+    "acceptance": "1. ddx bead migrate --to axon command exists and is documented in --help. 2. After migration, every bead (including events and archive entries) round-trips: ddx bead export | diff against pre-migration export = empty. 3. Idempotent: running twice produces no duplicates. 4. Test fixture under cli/internal/bead/testdata covers a representative beads.jsonl + archive.",
+    "parent": "ddx-5d49b14e",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:storage",
+      "kind:tool",
+      "backend-migration"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T22:17:16Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "execute-loop-heartbeat-at": "2026-05-03T22:17:16.754896824Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T221716-47fdb078",
+    "prompt": ".ddx/executions/20260503T221716-47fdb078/prompt.md",
+    "manifest": ".ddx/executions/20260503T221716-47fdb078/manifest.json",
+    "result": ".ddx/executions/20260503T221716-47fdb078/result.json",
+    "checks": ".ddx/executions/20260503T221716-47fdb078/checks.json",
+    "usage": ".ddx/executions/20260503T221716-47fdb078/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-cb63cdfc-20260503T221716-47fdb078"
+  },
+  "prompt_sha": "c38d2c44458a5d5179f285dc319c4b9af96641b940e337512e7eb0437d18b53f"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T221716-47fdb078/result.json b/.ddx/executions/20260503T221716-47fdb078/result.json
new file mode 100644
index 00000000..7efbe1c4
--- /dev/null
+++ b/.ddx/executions/20260503T221716-47fdb078/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-cb63cdfc",
+  "attempt_id": "20260503T221716-47fdb078",
+  "base_rev": "823ced625935a40a12172a0a073a221ae18e6520",
+  "result_rev": "4024c5c2df5fce1ee2f1f082fcf31d62eb72fc02",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-28d401e0",
+  "duration_ms": 789960,
+  "tokens": 38072,
+  "cost_usd": 4.720472749999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T221716-47fdb078",
+  "prompt_file": ".ddx/executions/20260503T221716-47fdb078/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T221716-47fdb078/manifest.json",
+  "result_file": ".ddx/executions/20260503T221716-47fdb078/result.json",
+  "usage_file": ".ddx/executions/20260503T221716-47fdb078/usage.json",
+  "started_at": "2026-05-03T22:17:18.383393857Z",
+  "finished_at": "2026-05-03T22:30:28.344088864Z"
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

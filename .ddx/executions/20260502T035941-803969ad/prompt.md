<bead-review>
  <bead id="ddx-9cc3abad" iter=1>
    <title>Refactor bead store: extract storage engine from beads.jsonl coupling</title>
    <description>
Per the TD authored in C1, refactor cli/internal/bead/ to support multiple named collections behind one storage engine. The current code assumes .ddx/beads.jsonl as the single backing file; introduce a collection registry that maps logical names ('beads', 'beads-archive', etc) to concrete file paths and locks. Preserve all existing behavior (bd/br interchange, atomic rename, repair, claim semantics). No archive logic yet — that's C3.
    </description>
    <acceptance>
1. cli/internal/bead/ has a Collection or Store type that owns named collections. 2. Existing 'beads' collection still backed by .ddx/beads.jsonl with identical semantics. 3. All existing bead store tests pass: 'cd cli &amp;&amp; go test ./internal/bead/...'. 4. schema_compat_test.go still green (bd/br round-trip). 5. No new collections instantiated yet (deferred to C3).
    </acceptance>
    <labels>area:beads, area:storage, kind:refactor</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T035603-d584f3bc/manifest.json</file>
    <file>.ddx/executions/20260502T035603-d584f3bc/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b5a5a6f6dae043c245ed7493baf108e0f220d56e">
diff --git a/.ddx/executions/20260502T035603-d584f3bc/manifest.json b/.ddx/executions/20260502T035603-d584f3bc/manifest.json
new file mode 100644
index 00000000..74ce3b06
--- /dev/null
+++ b/.ddx/executions/20260502T035603-d584f3bc/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T035603-d584f3bc",
+  "bead_id": "ddx-9cc3abad",
+  "base_rev": "f3f56c2819adceb91afa755debdc4eb8d3232d10",
+  "created_at": "2026-05-02T03:56:04.946638596Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9cc3abad",
+    "title": "Refactor bead store: extract storage engine from beads.jsonl coupling",
+    "description": "Per the TD authored in C1, refactor cli/internal/bead/ to support multiple named collections behind one storage engine. The current code assumes .ddx/beads.jsonl as the single backing file; introduce a collection registry that maps logical names ('beads', 'beads-archive', etc) to concrete file paths and locks. Preserve all existing behavior (bd/br interchange, atomic rename, repair, claim semantics). No archive logic yet — that's C3.",
+    "acceptance": "1. cli/internal/bead/ has a Collection or Store type that owns named collections. 2. Existing 'beads' collection still backed by .ddx/beads.jsonl with identical semantics. 3. All existing bead store tests pass: 'cd cli \u0026\u0026 go test ./internal/bead/...'. 4. schema_compat_test.go still green (bd/br round-trip). 5. No new collections instantiated yet (deferred to C3).",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "area:storage",
+      "kind:refactor"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T03:56:03Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T03:56:03.520343603Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T035603-d584f3bc",
+    "prompt": ".ddx/executions/20260502T035603-d584f3bc/prompt.md",
+    "manifest": ".ddx/executions/20260502T035603-d584f3bc/manifest.json",
+    "result": ".ddx/executions/20260502T035603-d584f3bc/result.json",
+    "checks": ".ddx/executions/20260502T035603-d584f3bc/checks.json",
+    "usage": ".ddx/executions/20260502T035603-d584f3bc/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9cc3abad-20260502T035603-d584f3bc"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T035603-d584f3bc/result.json b/.ddx/executions/20260502T035603-d584f3bc/result.json
new file mode 100644
index 00000000..0376162f
--- /dev/null
+++ b/.ddx/executions/20260502T035603-d584f3bc/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9cc3abad",
+  "attempt_id": "20260502T035603-d584f3bc",
+  "base_rev": "f3f56c2819adceb91afa755debdc4eb8d3232d10",
+  "result_rev": "84c1881d002deacd58e19a1c9b3a67140bba11a8",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-63bc76dc",
+  "duration_ms": 212080,
+  "tokens": 10257,
+  "cost_usd": 1.7075715000000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T035603-d584f3bc",
+  "prompt_file": ".ddx/executions/20260502T035603-d584f3bc/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T035603-d584f3bc/manifest.json",
+  "result_file": ".ddx/executions/20260502T035603-d584f3bc/result.json",
+  "usage_file": ".ddx/executions/20260502T035603-d584f3bc/usage.json",
+  "started_at": "2026-05-02T03:56:04.948801926Z",
+  "finished_at": "2026-05-02T03:59:37.029209373Z"
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

<bead-review>
  <bead id="ddx-c4476899" iter=1>
    <title>Migration command: split existing beads.jsonl into beads + beads-archive + attachments</title>
    <description>
Add 'ddx bead migrate' (or similar) that takes the existing .ddx/beads.jsonl and splits closed beads (per archival trigger from C3) into beads-archive, plus moves event arrays into attachment store per C4. Idempotent. Preserves all data. Documented in CLAUDE.md and in the TD from C1.
    </description>
    <acceptance>
1. ddx bead migrate runs cleanly on the current 5.4MB beads.jsonl. 2. After migration, .ddx/beads.jsonl &lt; 1MB, .ddx/beads-archive.jsonl exists, .ddx/attachments/ populated. 3. ddx bead status shows same totals before and after migration. 4. All existing bead operations (show, list, ready, blocked, dep tree) return identical results pre/post migration. 5. Tests: 'cd cli &amp;&amp; go test -run TestMigrate ./cmd/ ./internal/bead/'. 6. Idempotent — running migrate twice leaves state unchanged.
    </acceptance>
    <labels>area:beads, area:storage, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T041925-603fbd0a/manifest.json</file>
    <file>.ddx/executions/20260502T041925-603fbd0a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="fc49593fbc746f694f45b4f4b2846cfae0213cdc">
diff --git a/.ddx/executions/20260502T041925-603fbd0a/manifest.json b/.ddx/executions/20260502T041925-603fbd0a/manifest.json
new file mode 100644
index 00000000..90980961
--- /dev/null
+++ b/.ddx/executions/20260502T041925-603fbd0a/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T041925-603fbd0a",
+  "bead_id": "ddx-c4476899",
+  "base_rev": "02ce8c2f93c1cb7cc3fbb719efd8da3082637d50",
+  "created_at": "2026-05-02T04:19:27.299667399Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c4476899",
+    "title": "Migration command: split existing beads.jsonl into beads + beads-archive + attachments",
+    "description": "Add 'ddx bead migrate' (or similar) that takes the existing .ddx/beads.jsonl and splits closed beads (per archival trigger from C3) into beads-archive, plus moves event arrays into attachment store per C4. Idempotent. Preserves all data. Documented in CLAUDE.md and in the TD from C1.",
+    "acceptance": "1. ddx bead migrate runs cleanly on the current 5.4MB beads.jsonl. 2. After migration, .ddx/beads.jsonl \u003c 1MB, .ddx/beads-archive.jsonl exists, .ddx/attachments/ populated. 3. ddx bead status shows same totals before and after migration. 4. All existing bead operations (show, list, ready, blocked, dep tree) return identical results pre/post migration. 5. Tests: 'cd cli \u0026\u0026 go test -run TestMigrate ./cmd/ ./internal/bead/'. 6. Idempotent — running migrate twice leaves state unchanged.",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "area:storage",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T04:19:25Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T04:19:25.770689506Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T041925-603fbd0a",
+    "prompt": ".ddx/executions/20260502T041925-603fbd0a/prompt.md",
+    "manifest": ".ddx/executions/20260502T041925-603fbd0a/manifest.json",
+    "result": ".ddx/executions/20260502T041925-603fbd0a/result.json",
+    "checks": ".ddx/executions/20260502T041925-603fbd0a/checks.json",
+    "usage": ".ddx/executions/20260502T041925-603fbd0a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c4476899-20260502T041925-603fbd0a"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T041925-603fbd0a/result.json b/.ddx/executions/20260502T041925-603fbd0a/result.json
new file mode 100644
index 00000000..7c064267
--- /dev/null
+++ b/.ddx/executions/20260502T041925-603fbd0a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-c4476899",
+  "attempt_id": "20260502T041925-603fbd0a",
+  "base_rev": "02ce8c2f93c1cb7cc3fbb719efd8da3082637d50",
+  "result_rev": "a16114f1bb4489789c338d8ebb07e308b29e13ed",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-956b8061",
+  "duration_ms": 597241,
+  "tokens": 29757,
+  "cost_usd": 4.2466532500000005,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T041925-603fbd0a",
+  "prompt_file": ".ddx/executions/20260502T041925-603fbd0a/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T041925-603fbd0a/manifest.json",
+  "result_file": ".ddx/executions/20260502T041925-603fbd0a/result.json",
+  "usage_file": ".ddx/executions/20260502T041925-603fbd0a/usage.json",
+  "started_at": "2026-05-02T04:19:27.300921149Z",
+  "finished_at": "2026-05-02T04:29:24.542909807Z"
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

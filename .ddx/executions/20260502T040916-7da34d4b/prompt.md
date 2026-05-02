<bead-review>
  <bead id="ddx-9aecc66c" iter=1>
    <title>Add beads-archive collection with archival policy</title>
    <description>
Per TD-NNN from C1, add the beads-archive collection backed by .ddx/beads-archive.jsonl. Implement the archival trigger chosen in the TD. Move closed beads from beads → beads-archive when the trigger fires. Keep ddx bead show/list semantics transparent across both collections.
    </description>
    <acceptance>
1. .ddx/beads-archive.jsonl is written when archival trigger fires. 2. ddx bead show &lt;id&gt; finds beads in either collection. 3. ddx bead list (default) shows active + recently closed; --all flag includes archive. 4. ddx bead ready/blocked queries only active. 5. Tests cover the archival flow: 'cd cli &amp;&amp; go test -run TestArchive ./internal/bead/'. 6. Migration of existing beads.jsonl is via separate command (deferred to C5).
    </acceptance>
    <labels>area:beads, area:storage, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T040017-7246ee65/manifest.json</file>
    <file>.ddx/executions/20260502T040017-7246ee65/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="cd1f0a936eb650e1255c1777609f56812a0575ac">
diff --git a/.ddx/executions/20260502T040017-7246ee65/manifest.json b/.ddx/executions/20260502T040017-7246ee65/manifest.json
new file mode 100644
index 00000000..ea071fd8
--- /dev/null
+++ b/.ddx/executions/20260502T040017-7246ee65/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T040017-7246ee65",
+  "bead_id": "ddx-9aecc66c",
+  "base_rev": "b0d23d62c62b5868fd389d1e5468b34f4bb4ba61",
+  "created_at": "2026-05-02T04:00:18.59375396Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9aecc66c",
+    "title": "Add beads-archive collection with archival policy",
+    "description": "Per TD-NNN from C1, add the beads-archive collection backed by .ddx/beads-archive.jsonl. Implement the archival trigger chosen in the TD. Move closed beads from beads → beads-archive when the trigger fires. Keep ddx bead show/list semantics transparent across both collections.",
+    "acceptance": "1. .ddx/beads-archive.jsonl is written when archival trigger fires. 2. ddx bead show \u003cid\u003e finds beads in either collection. 3. ddx bead list (default) shows active + recently closed; --all flag includes archive. 4. ddx bead ready/blocked queries only active. 5. Tests cover the archival flow: 'cd cli \u0026\u0026 go test -run TestArchive ./internal/bead/'. 6. Migration of existing beads.jsonl is via separate command (deferred to C5).",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "area:storage",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T04:00:17Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T04:00:17.031874131Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T040017-7246ee65",
+    "prompt": ".ddx/executions/20260502T040017-7246ee65/prompt.md",
+    "manifest": ".ddx/executions/20260502T040017-7246ee65/manifest.json",
+    "result": ".ddx/executions/20260502T040017-7246ee65/result.json",
+    "checks": ".ddx/executions/20260502T040017-7246ee65/checks.json",
+    "usage": ".ddx/executions/20260502T040017-7246ee65/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9aecc66c-20260502T040017-7246ee65"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T040017-7246ee65/result.json b/.ddx/executions/20260502T040017-7246ee65/result.json
new file mode 100644
index 00000000..5ec7b6b0
--- /dev/null
+++ b/.ddx/executions/20260502T040017-7246ee65/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9aecc66c",
+  "attempt_id": "20260502T040017-7246ee65",
+  "base_rev": "b0d23d62c62b5868fd389d1e5468b34f4bb4ba61",
+  "result_rev": "6b997f399f49285cac3b34a5b007c291bbe1bb87",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-c5cc2a17",
+  "duration_ms": 530210,
+  "tokens": 23000,
+  "cost_usd": 3.12385775,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T040017-7246ee65",
+  "prompt_file": ".ddx/executions/20260502T040017-7246ee65/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T040017-7246ee65/manifest.json",
+  "result_file": ".ddx/executions/20260502T040017-7246ee65/result.json",
+  "usage_file": ".ddx/executions/20260502T040017-7246ee65/usage.json",
+  "started_at": "2026-05-02T04:00:18.594050877Z",
+  "finished_at": "2026-05-02T04:09:08.804192235Z"
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

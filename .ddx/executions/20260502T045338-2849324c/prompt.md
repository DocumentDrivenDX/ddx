<bead-review>
  <bead id="ddx-cd1f0f7e" iter=1>
    <title>Sidecar attachment storage for closed-bead events (ADR-004 step 3)</title>
    <description>
Implement attachment sidecar storage at .ddx/attachments/&lt;bead-id&gt;/events.jsonl. The bead record in beads-archive carries a reference (preserved-extra field, e.g. 'attachments': {'events': '&lt;relative-path&gt;'}) instead of an inline events array, so the archive row stays small. ddx bead show lazy-loads events from the sidecar when present and merges them into the displayed history. Active beads keep events inline (no behavior change) — the move to sidecar only happens when a bead is archived (handled in the archive-command bead).

References: ADR-004 §Attachment Model. Depends on ddx-2f453147.
    </description>
    <acceptance>
1. New attachment helper API in cli/internal/bead reads/writes .ddx/attachments/&lt;bead-id&gt;/events.jsonl. 2. Bead schema preserves an 'attachments' extras field round-trip without losing it. 3. ddx bead show &lt;id&gt; displays events sourced from the sidecar when the bead row uses an attachment reference, identically to inline-events display. 4. schema_compat_test.go still green; bd/br round-trip preserves the attachments extra. 5. Tests cover sidecar read, missing-sidecar fallback, and round-trip preservation.
    </acceptance>
    <labels>area:beads, area:storage, kind:feature, adr:004</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T045058-fd9e7a8c/manifest.json</file>
    <file>.ddx/executions/20260502T045058-fd9e7a8c/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a8cf295c63ee08c7c8bd42ebe98e832124591a7e">
diff --git a/.ddx/executions/20260502T045058-fd9e7a8c/manifest.json b/.ddx/executions/20260502T045058-fd9e7a8c/manifest.json
new file mode 100644
index 00000000..6504c55a
--- /dev/null
+++ b/.ddx/executions/20260502T045058-fd9e7a8c/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T045058-fd9e7a8c",
+  "bead_id": "ddx-cd1f0f7e",
+  "base_rev": "3e7413b07069bd178dc8bd5ae185b6b0a915e3b2",
+  "created_at": "2026-05-02T04:50:59.390036111Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-cd1f0f7e",
+    "title": "Sidecar attachment storage for closed-bead events (ADR-004 step 3)",
+    "description": "Implement attachment sidecar storage at .ddx/attachments/\u003cbead-id\u003e/events.jsonl. The bead record in beads-archive carries a reference (preserved-extra field, e.g. 'attachments': {'events': '\u003crelative-path\u003e'}) instead of an inline events array, so the archive row stays small. ddx bead show lazy-loads events from the sidecar when present and merges them into the displayed history. Active beads keep events inline (no behavior change) — the move to sidecar only happens when a bead is archived (handled in the archive-command bead).\n\nReferences: ADR-004 §Attachment Model. Depends on ddx-2f453147.",
+    "acceptance": "1. New attachment helper API in cli/internal/bead reads/writes .ddx/attachments/\u003cbead-id\u003e/events.jsonl. 2. Bead schema preserves an 'attachments' extras field round-trip without losing it. 3. ddx bead show \u003cid\u003e displays events sourced from the sidecar when the bead row uses an attachment reference, identically to inline-events display. 4. schema_compat_test.go still green; bd/br round-trip preserves the attachments extra. 5. Tests cover sidecar read, missing-sidecar fallback, and round-trip preservation.",
+    "parent": "ddx-0c0565f3",
+    "labels": [
+      "area:beads",
+      "area:storage",
+      "kind:feature",
+      "adr:004"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T04:50:58Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "execute-loop-heartbeat-at": "2026-05-02T04:50:58.022899668Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T045058-fd9e7a8c",
+    "prompt": ".ddx/executions/20260502T045058-fd9e7a8c/prompt.md",
+    "manifest": ".ddx/executions/20260502T045058-fd9e7a8c/manifest.json",
+    "result": ".ddx/executions/20260502T045058-fd9e7a8c/result.json",
+    "checks": ".ddx/executions/20260502T045058-fd9e7a8c/checks.json",
+    "usage": ".ddx/executions/20260502T045058-fd9e7a8c/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-cd1f0f7e-20260502T045058-fd9e7a8c"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T045058-fd9e7a8c/result.json b/.ddx/executions/20260502T045058-fd9e7a8c/result.json
new file mode 100644
index 00000000..70dc337e
--- /dev/null
+++ b/.ddx/executions/20260502T045058-fd9e7a8c/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-cd1f0f7e",
+  "attempt_id": "20260502T045058-fd9e7a8c",
+  "base_rev": "3e7413b07069bd178dc8bd5ae185b6b0a915e3b2",
+  "result_rev": "ca1240ae3f053b814cbf7fcdfed8d7fd32078bed",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-d415b402",
+  "duration_ms": 155364,
+  "tokens": 6256,
+  "cost_usd": 0.94798225,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T045058-fd9e7a8c",
+  "prompt_file": ".ddx/executions/20260502T045058-fd9e7a8c/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T045058-fd9e7a8c/manifest.json",
+  "result_file": ".ddx/executions/20260502T045058-fd9e7a8c/result.json",
+  "usage_file": ".ddx/executions/20260502T045058-fd9e7a8c/usage.json",
+  "started_at": "2026-05-02T04:50:59.390338069Z",
+  "finished_at": "2026-05-02T04:53:34.754977821Z"
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

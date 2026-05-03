<bead-review>
  <bead id="ddx-88507ed5" iter=1>
    <title>metric: author MET-001 (exec) and MET-002 (external) artifacts</title>
    <description>
Concrete examples: MET-001 (source: exec, computed from execution attempts) and MET-002 (source: external, prose-defined precedent for human-supplied metrics). Live under docs/helix/&lt;phase&gt;/metrics/ or library examples.
    </description>
    <acceptance>
1. MET-001 + MET-002 exist with full frontmatter per the schema in B1a. 2. ddx doc audit shows them with no errors. 3. Both serve as canonical references for plugin authors.
    </acceptance>
    <labels>phase:2, story:13, area:specs, kind:doc</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T161949-e7c7f3c2/manifest.json</file>
    <file>.ddx/executions/20260503T161949-e7c7f3c2/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9d397a83a6d4c9a6093b8df6e3b2e8543e2314fe">
diff --git a/.ddx/executions/20260503T161949-e7c7f3c2/manifest.json b/.ddx/executions/20260503T161949-e7c7f3c2/manifest.json
new file mode 100644
index 00000000..ba27b7fc
--- /dev/null
+++ b/.ddx/executions/20260503T161949-e7c7f3c2/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260503T161949-e7c7f3c2",
+  "bead_id": "ddx-88507ed5",
+  "base_rev": "86eff75dd6e93b03632a2d1865df10abbb3b000e",
+  "created_at": "2026-05-03T16:19:50.501624627Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-88507ed5",
+    "title": "metric: author MET-001 (exec) and MET-002 (external) artifacts",
+    "description": "Concrete examples: MET-001 (source: exec, computed from execution attempts) and MET-002 (source: external, prose-defined precedent for human-supplied metrics). Live under docs/helix/\u003cphase\u003e/metrics/ or library examples.",
+    "acceptance": "1. MET-001 + MET-002 exist with full frontmatter per the schema in B1a. 2. ddx doc audit shows them with no errors. 3. Both serve as canonical references for plugin authors.",
+    "parent": "ddx-921616ea",
+    "labels": [
+      "phase:2",
+      "story:13",
+      "area:specs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T16:19:49Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "execute-loop-heartbeat-at": "2026-05-03T16:19:49.018353661Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T161949-e7c7f3c2",
+    "prompt": ".ddx/executions/20260503T161949-e7c7f3c2/prompt.md",
+    "manifest": ".ddx/executions/20260503T161949-e7c7f3c2/manifest.json",
+    "result": ".ddx/executions/20260503T161949-e7c7f3c2/result.json",
+    "checks": ".ddx/executions/20260503T161949-e7c7f3c2/checks.json",
+    "usage": ".ddx/executions/20260503T161949-e7c7f3c2/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-88507ed5-20260503T161949-e7c7f3c2"
+  },
+  "prompt_sha": "501d99d811d44459c531488b05cac55454c36f6af8411169eb5a62c82d39c869"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T161949-e7c7f3c2/result.json b/.ddx/executions/20260503T161949-e7c7f3c2/result.json
new file mode 100644
index 00000000..ba04d06b
--- /dev/null
+++ b/.ddx/executions/20260503T161949-e7c7f3c2/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-88507ed5",
+  "attempt_id": "20260503T161949-e7c7f3c2",
+  "base_rev": "86eff75dd6e93b03632a2d1865df10abbb3b000e",
+  "result_rev": "b4ad54d8993b7f997a0f3bd4d4f2a8d72e9f6795",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-8513648c",
+  "duration_ms": 194272,
+  "tokens": 8268,
+  "cost_usd": 0.9480930000000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T161949-e7c7f3c2",
+  "prompt_file": ".ddx/executions/20260503T161949-e7c7f3c2/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T161949-e7c7f3c2/manifest.json",
+  "result_file": ".ddx/executions/20260503T161949-e7c7f3c2/result.json",
+  "usage_file": ".ddx/executions/20260503T161949-e7c7f3c2/usage.json",
+  "started_at": "2026-05-03T16:19:50.501916793Z",
+  "finished_at": "2026-05-03T16:23:04.774896098Z"
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

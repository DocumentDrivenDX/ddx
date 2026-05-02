<bead-review>
  <bead id="ddx-58566cbf" iter=1>
    <title>Test evidence path: write hello.md with go version</title>
    <description>
Verification bead for execute-bead /tmp evidence pattern fix. The agent should write a file named hello.md under the per-attempt evidence directory describing the current Go version (output of 'go version'). This bead exists solely to confirm that v0.6.2-alpha4 steers agents to in-repo evidence dir rather than /tmp.
    </description>
    <acceptance>
Write a one-line report named hello.md describing the current Go version (e.g. 'go version go1.23.x linux/amd64'). The file MUST be located under .ddx/executions/&lt;run-id&gt;/hello.md (NOT /tmp). Stage and commit it as part of the bead's commit.
    </acceptance>
    <labels>area:agent, kind:test, validation</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T044406-19ebbeb6/manifest.json</file>
    <file>.ddx/executions/20260502T044406-19ebbeb6/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="d58cc235cb29652da58d11991f2a7ddfbba9a3a7">
diff --git a/.ddx/executions/20260502T044406-19ebbeb6/manifest.json b/.ddx/executions/20260502T044406-19ebbeb6/manifest.json
new file mode 100644
index 00000000..2868546a
--- /dev/null
+++ b/.ddx/executions/20260502T044406-19ebbeb6/manifest.json
@@ -0,0 +1,54 @@
+{
+  "attempt_id": "20260502T044406-19ebbeb6",
+  "bead_id": "ddx-58566cbf",
+  "base_rev": "98c8117c094aa7d4761a6d0e8949ea168cf481ca",
+  "created_at": "2026-05-02T04:44:08.527223253Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-58566cbf",
+    "title": "Test evidence path: write hello.md with go version",
+    "description": "Verification bead for execute-bead /tmp evidence pattern fix. The agent should write a file named hello.md under the per-attempt evidence directory describing the current Go version (output of 'go version'). This bead exists solely to confirm that v0.6.2-alpha4 steers agents to in-repo evidence dir rather than /tmp.",
+    "acceptance": "Write a one-line report named hello.md describing the current Go version (e.g. 'go version go1.23.x linux/amd64'). The file MUST be located under .ddx/executions/\u003crun-id\u003e/hello.md (NOT /tmp). Stage and commit it as part of the bead's commit.",
+    "labels": [
+      "area:agent",
+      "kind:test",
+      "validation"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T04:44:06Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1329405",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[],\"requested_harness\":\"claude\"}",
+          "created_at": "2026-05-02T03:44:20.282549252Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260502T034358-5cb371cb\",\"harness\":\"claude\",\"input_tokens\":9,\"output_tokens\":732,\"total_tokens\":741,\"cost_usd\":0.21468250000000003,\"duration_ms\":19787,\"exit_code\":0}",
+          "created_at": "2026-05-02T03:44:20.403917391Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=741 cost_usd=0.2147"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T04:44:06.134353763Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T044406-19ebbeb6",
+    "prompt": ".ddx/executions/20260502T044406-19ebbeb6/prompt.md",
+    "manifest": ".ddx/executions/20260502T044406-19ebbeb6/manifest.json",
+    "result": ".ddx/executions/20260502T044406-19ebbeb6/result.json",
+    "checks": ".ddx/executions/20260502T044406-19ebbeb6/checks.json",
+    "usage": ".ddx/executions/20260502T044406-19ebbeb6/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-58566cbf-20260502T044406-19ebbeb6"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T044406-19ebbeb6/result.json b/.ddx/executions/20260502T044406-19ebbeb6/result.json
new file mode 100644
index 00000000..d079db85
--- /dev/null
+++ b/.ddx/executions/20260502T044406-19ebbeb6/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-58566cbf",
+  "attempt_id": "20260502T044406-19ebbeb6",
+  "base_rev": "98c8117c094aa7d4761a6d0e8949ea168cf481ca",
+  "result_rev": "2212ed7c1444d7b0ac3f6655fe8eb4a5d95d2e2e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-65c913cc",
+  "duration_ms": 18101,
+  "tokens": 662,
+  "cost_usd": 0.21363200000000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T044406-19ebbeb6",
+  "prompt_file": ".ddx/executions/20260502T044406-19ebbeb6/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T044406-19ebbeb6/manifest.json",
+  "result_file": ".ddx/executions/20260502T044406-19ebbeb6/result.json",
+  "usage_file": ".ddx/executions/20260502T044406-19ebbeb6/usage.json",
+  "started_at": "2026-05-02T04:44:08.527491253Z",
+  "finished_at": "2026-05-02T04:44:26.628513067Z"
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

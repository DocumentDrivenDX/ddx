<bead-review>
  <bead id="ddx-a7e7a1a1" iter=1>
    <title>S15-3: operatorPromptSubmit GraphQL mutation w/ CSRF, idempotency, audit</title>
    <description>
Implement GraphQL mutation operatorPromptSubmit(projectId, prompt, tier?, harness?, idempotencyKey) → Bead. Gated by requireTrusted. Default status=proposed. Cap prompt at serverPromptCapBytes. Add strict Origin/Host validation, per-session CSRF token, idempotency-key dedupe, identity-bound first-event audit (peer identity, origin node ID, build SHA, approval-mode, request ID, prompt SHA-256). See /tmp/story-15-final.md §Implementation #2-3 and §Additional security controls.
    </description>
    <acceptance>
Mutation creates bead with issueType=operator-prompt, status=proposed, prompt body verbatim; non-trusted origin returns 403; oversize returns 400 with cap-error envelope; cross-origin browser POST rejected; duplicate idempotencyKey returns same bead without creating new; first bead event records structured audit fields enumerated above; integration tests for each branch including multibyte oversize and GraphQL-variable oversize.
    </acceptance>
    <labels>phase:2, story:15, kind:graphql</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T224933-b7b99343/manifest.json</file>
    <file>.ddx/executions/20260502T224933-b7b99343/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="e926516a5cb58d0a62faa0a3a7429eed6d388d88">
diff --git a/.ddx/executions/20260502T224933-b7b99343/manifest.json b/.ddx/executions/20260502T224933-b7b99343/manifest.json
new file mode 100644
index 00000000..93626819
--- /dev/null
+++ b/.ddx/executions/20260502T224933-b7b99343/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260502T224933-b7b99343",
+  "bead_id": "ddx-a7e7a1a1",
+  "base_rev": "91de2e0b6edd16a3814be4a8117a112114bc2803",
+  "created_at": "2026-05-02T22:49:34.751089566Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-a7e7a1a1",
+    "title": "S15-3: operatorPromptSubmit GraphQL mutation w/ CSRF, idempotency, audit",
+    "description": "Implement GraphQL mutation operatorPromptSubmit(projectId, prompt, tier?, harness?, idempotencyKey) → Bead. Gated by requireTrusted. Default status=proposed. Cap prompt at serverPromptCapBytes. Add strict Origin/Host validation, per-session CSRF token, idempotency-key dedupe, identity-bound first-event audit (peer identity, origin node ID, build SHA, approval-mode, request ID, prompt SHA-256). See /tmp/story-15-final.md §Implementation #2-3 and §Additional security controls.",
+    "acceptance": "Mutation creates bead with issueType=operator-prompt, status=proposed, prompt body verbatim; non-trusted origin returns 403; oversize returns 400 with cap-error envelope; cross-origin browser POST rejected; duplicate idempotencyKey returns same bead without creating new; first bead event records structured audit fields enumerated above; integration tests for each branch including multibyte oversize and GraphQL-variable oversize.",
+    "parent": "ddx-1d85c927",
+    "labels": [
+      "phase:2",
+      "story:15",
+      "kind:graphql"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T22:49:33Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3924346",
+      "execute-loop-heartbeat-at": "2026-05-02T22:49:33.383439191Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T224933-b7b99343",
+    "prompt": ".ddx/executions/20260502T224933-b7b99343/prompt.md",
+    "manifest": ".ddx/executions/20260502T224933-b7b99343/manifest.json",
+    "result": ".ddx/executions/20260502T224933-b7b99343/result.json",
+    "checks": ".ddx/executions/20260502T224933-b7b99343/checks.json",
+    "usage": ".ddx/executions/20260502T224933-b7b99343/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-a7e7a1a1-20260502T224933-b7b99343"
+  },
+  "prompt_sha": "fa40eb2500ec1f051bc33d3940aba1fe5288b396a9b35e14f5c8f6c7587bb616"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T224933-b7b99343/result.json b/.ddx/executions/20260502T224933-b7b99343/result.json
new file mode 100644
index 00000000..74bc3373
--- /dev/null
+++ b/.ddx/executions/20260502T224933-b7b99343/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-a7e7a1a1",
+  "attempt_id": "20260502T224933-b7b99343",
+  "base_rev": "91de2e0b6edd16a3814be4a8117a112114bc2803",
+  "result_rev": "5ebab96db36728c5801801aca743e87cd79db08b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-19995e09",
+  "duration_ms": 744349,
+  "tokens": 34401,
+  "cost_usd": 5.23767575,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T224933-b7b99343",
+  "prompt_file": ".ddx/executions/20260502T224933-b7b99343/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T224933-b7b99343/manifest.json",
+  "result_file": ".ddx/executions/20260502T224933-b7b99343/result.json",
+  "usage_file": ".ddx/executions/20260502T224933-b7b99343/usage.json",
+  "started_at": "2026-05-02T22:49:34.751382191Z",
+  "finished_at": "2026-05-02T23:01:59.100398681Z"
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

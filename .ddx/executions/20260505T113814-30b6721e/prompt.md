<bead-review>
  <bead id="ddx-0e5c3005" iter=1>
    <title>review: structured prompt assembly + FEAT-022 telemetry + injection delimiters</title>
    <description>
Structured prompt assembly per turn (NOT naive verbatim+oldest-first truncation). Pinned: system rubric, artifact identity (path/sha/git_rev), Story 17 template+prompt, first-user-intent + explicit user decisions. Rolling: session memory summary, last K verbatim turns, unresolved findings. FEAT-022 byte accounting. Refuses with PROMPT_BUDGET_EXCEEDED if minimum pinned set won't fit. Untrusted-data delimiters (artifact body wrapped) for prompt-injection boundary.
    </description>
    <acceptance>
1. Assembly function exists with structured pinned + summary + recent + unresolved. 2. FEAT-022 byte accounting per assembly. 3. PROMPT_BUDGET_EXCEEDED refusal with clear error. 4. Untrusted-data delimiters wrap artifact body. 5. Tests cover all four.
    </acceptance>
    <notes>
decomposed into .execute-bead-wt-ddx-0e5c3005-20260505T113434-3317d134-f25a64d9, .execute-bead-wt-ddx-0e5c3005-20260505T113434-3317d134-7fb75345
    </notes>
    <labels>phase:2, story:18, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T113434-3317d134/manifest.json</file>
    <file>.ddx/executions/20260505T113434-3317d134/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b1a8133af2994880807c9bac40893cb16352bb25">
diff --git a/.ddx/executions/20260505T113434-3317d134/manifest.json b/.ddx/executions/20260505T113434-3317d134/manifest.json
new file mode 100644
index 00000000..17ea6b3b
--- /dev/null
+++ b/.ddx/executions/20260505T113434-3317d134/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260505T113434-3317d134",
+  "bead_id": "ddx-0e5c3005",
+  "base_rev": "bd7ab90c9db971dff5453d7d4cb67bb254cec26f",
+  "created_at": "2026-05-05T11:34:37.337073785Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-0e5c3005",
+    "title": "review: structured prompt assembly + FEAT-022 telemetry + injection delimiters",
+    "description": "Structured prompt assembly per turn (NOT naive verbatim+oldest-first truncation). Pinned: system rubric, artifact identity (path/sha/git_rev), Story 17 template+prompt, first-user-intent + explicit user decisions. Rolling: session memory summary, last K verbatim turns, unresolved findings. FEAT-022 byte accounting. Refuses with PROMPT_BUDGET_EXCEEDED if minimum pinned set won't fit. Untrusted-data delimiters (artifact body wrapped) for prompt-injection boundary.",
+    "acceptance": "1. Assembly function exists with structured pinned + summary + recent + unresolved. 2. FEAT-022 byte accounting per assembly. 3. PROMPT_BUDGET_EXCEEDED refusal with clear error. 4. Untrusted-data delimiters wrap artifact body. 5. Tests cover all four.",
+    "parent": "ddx-42b917fe",
+    "labels": [
+      "phase:2",
+      "story:18",
+      "area:server",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T11:34:33Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "execute-loop-heartbeat-at": "2026-05-05T11:34:33.949448476Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T113434-3317d134",
+    "prompt": ".ddx/executions/20260505T113434-3317d134/prompt.md",
+    "manifest": ".ddx/executions/20260505T113434-3317d134/manifest.json",
+    "result": ".ddx/executions/20260505T113434-3317d134/result.json",
+    "checks": ".ddx/executions/20260505T113434-3317d134/checks.json",
+    "usage": ".ddx/executions/20260505T113434-3317d134/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-0e5c3005-20260505T113434-3317d134"
+  },
+  "prompt_sha": "24e00fb22be3f92d1947276e6b04d9d1baff9496484a2e8624789e884869e03b"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T113434-3317d134/result.json b/.ddx/executions/20260505T113434-3317d134/result.json
new file mode 100644
index 00000000..d71fbe21
--- /dev/null
+++ b/.ddx/executions/20260505T113434-3317d134/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-0e5c3005",
+  "attempt_id": "20260505T113434-3317d134",
+  "base_rev": "bd7ab90c9db971dff5453d7d4cb67bb254cec26f",
+  "result_rev": "6d3234178aee667f4e199254596bb498b3eb68e5",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-64053758",
+  "duration_ms": 211007,
+  "tokens": 2881756,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T113434-3317d134",
+  "prompt_file": ".ddx/executions/20260505T113434-3317d134/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T113434-3317d134/manifest.json",
+  "result_file": ".ddx/executions/20260505T113434-3317d134/result.json",
+  "usage_file": ".ddx/executions/20260505T113434-3317d134/usage.json",
+  "started_at": "2026-05-05T11:34:37.337370285Z",
+  "finished_at": "2026-05-05T11:38:08.344571967Z"
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

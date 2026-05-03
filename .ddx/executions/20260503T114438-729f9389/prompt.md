<bead-review>
  <bead id="ddx-6db5d14a" iter=1>
    <title>C2: try.Attempt function shell wrapping ExecuteBeadWithConfig + LandBeadResult</title>
    <description>
Create cli/internal/agent/try/attempt.go that wraps current agent.ExecuteBeadWithConfig + agent.LandBeadResult + optional agent.DefaultBeadReviewer. Returns try.Outcome via the C1 adapter. Loop continues using old executor; this shell is unused at end of C2 (caller migration in C3+).
    </description>
    <acceptance>
1. cli/internal/agent/try/attempt.go exposes try.Attempt(ctx, store, beadID, opts) (Outcome, error). 2. Internally calls existing ExecuteBeadWithConfig + LandBeadResult + DefaultBeadReviewer. 3. Returns Outcome via ToOutcome adapter (C1). 4. Unit test TestAttempt_WrapsLegacyExecutor with stubbed executor. 5. cd cli &amp;&amp; go test ./internal/agent/try/ green. 6. No production callers yet.
    </acceptance>
    <labels>phase:2, refactor, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T113858-be0a8400/manifest.json</file>
    <file>.ddx/executions/20260503T113858-be0a8400/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9366bd6c5310c7781189acd2e0daf7499c086063">
diff --git a/.ddx/executions/20260503T113858-be0a8400/manifest.json b/.ddx/executions/20260503T113858-be0a8400/manifest.json
new file mode 100644
index 00000000..f270d745
--- /dev/null
+++ b/.ddx/executions/20260503T113858-be0a8400/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260503T113858-be0a8400",
+  "bead_id": "ddx-6db5d14a",
+  "base_rev": "e57c42e82b6abfda52fba673f548e593aa542f91",
+  "created_at": "2026-05-03T11:39:00.553837649Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-6db5d14a",
+    "title": "C2: try.Attempt function shell wrapping ExecuteBeadWithConfig + LandBeadResult",
+    "description": "Create cli/internal/agent/try/attempt.go that wraps current agent.ExecuteBeadWithConfig + agent.LandBeadResult + optional agent.DefaultBeadReviewer. Returns try.Outcome via the C1 adapter. Loop continues using old executor; this shell is unused at end of C2 (caller migration in C3+).",
+    "acceptance": "1. cli/internal/agent/try/attempt.go exposes try.Attempt(ctx, store, beadID, opts) (Outcome, error). 2. Internally calls existing ExecuteBeadWithConfig + LandBeadResult + DefaultBeadReviewer. 3. Returns Outcome via ToOutcome adapter (C1). 4. Unit test TestAttempt_WrapsLegacyExecutor with stubbed executor. 5. cd cli \u0026\u0026 go test ./internal/agent/try/ green. 6. No production callers yet.",
+    "parent": "ddx-5cb6e6cd",
+    "labels": [
+      "phase:2",
+      "refactor",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T11:38:58Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "execute-loop-heartbeat-at": "2026-05-03T11:38:58.896117975Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T113858-be0a8400",
+    "prompt": ".ddx/executions/20260503T113858-be0a8400/prompt.md",
+    "manifest": ".ddx/executions/20260503T113858-be0a8400/manifest.json",
+    "result": ".ddx/executions/20260503T113858-be0a8400/result.json",
+    "checks": ".ddx/executions/20260503T113858-be0a8400/checks.json",
+    "usage": ".ddx/executions/20260503T113858-be0a8400/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-6db5d14a-20260503T113858-be0a8400"
+  },
+  "prompt_sha": "d15b2488394927bac1bbefb8d1c016296c7e69869d1ea04e17e4c4ca1a1baffb"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T113858-be0a8400/result.json b/.ddx/executions/20260503T113858-be0a8400/result.json
new file mode 100644
index 00000000..217788b1
--- /dev/null
+++ b/.ddx/executions/20260503T113858-be0a8400/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-6db5d14a",
+  "attempt_id": "20260503T113858-be0a8400",
+  "base_rev": "e57c42e82b6abfda52fba673f548e593aa542f91",
+  "result_rev": "0a268e89773bd65e2edbaab14c999eb42e2ce80c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-58e48fcc",
+  "duration_ms": 333641,
+  "tokens": 14750,
+  "cost_usd": 2.03636225,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T113858-be0a8400",
+  "prompt_file": ".ddx/executions/20260503T113858-be0a8400/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T113858-be0a8400/manifest.json",
+  "result_file": ".ddx/executions/20260503T113858-be0a8400/result.json",
+  "usage_file": ".ddx/executions/20260503T113858-be0a8400/usage.json",
+  "started_at": "2026-05-03T11:39:00.554159982Z",
+  "finished_at": "2026-05-03T11:44:34.195630091Z"
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

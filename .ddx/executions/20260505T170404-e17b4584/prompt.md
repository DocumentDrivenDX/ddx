<bead-review>
  <bead id="ddx-e36d7959" iter=1>
    <title>try.Attempt: relocate rate-limit retry policy</title>
    <description>
PROBLEM\nThe per-bead HTTP 429 retry policy still lives in execute_bead.go, so the attempt path does not own Retry-After parsing, exponential backoff selection, or the per-bead retry budget. That makes rate-limit behavior depend on loop wiring instead of the attempt boundary that actually issues the dispatch.\n\nROOT CAUSE\n- cli/internal/agent/execute_bead.go:800-851 builds RateLimitRetryConfig, installs the RecordRouteAttempt / bead-event hooks, and wraps dispatchAgentRun in RunWithRateLimitRetry.\n- cli/internal/agent/try/attempt.go:99-137 still delegates only to legacy conflict recovery and never classifies or retries rate-limited attempts itself.\n\nPROPOSED FIX\n- Move the Retry-After / exponential-backoff / budget-cap decision into cli/internal/agent/try/attempt.go on the rate-limit detection path.\n- Keep the per-bead retry budget, per-wait cap, and routing-engine transparency hooks intact, but make them part of the attempt outcome contract rather than loop-local policy.\n- Preserve the existing stable backoff schedule and budget-exhausted reason string.\n- Reduce execute_bead.go to a generic caller of the structured attempt result.\n\nNON-SCOPE\n- No_changes adjudication.\n- Decomposition, push_failed, or push_conflict routing.\n- Any change to the published backoff schedule or budget constants.\n\nINTERSECTIONS\n- Absorbs ddx-c6e3db02 behavior.\n
    </description>
    <acceptance>
1. TestAttempt_RateLimit_Retry_HonorsRetryAfter verifies the attempt layer honors Retry-After exactly when present.\n2. TestAttempt_RateLimit_Retry_UsesExponentialBackoff verifies the attempt layer falls back to the published backoff schedule when Retry-After is absent.\n3. TestAttempt_RateLimit_WiresEvaluateRateLimitWait proves the rate-limit detection path calls EvaluateRateLimitWait from try.Attempt rather than only defining it.\n4. TestRunWithRateLimitRetry_RetryAfterRespected and TestRunWithRateLimitRetry_ExponentialBackoffWhenRetryAfterAbsent remain green after the move.\n5. cd cli &amp;&amp; go test ./internal/agent/... green.\n6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, kind:refactor, story:10, observed-failure</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T165234-0c0b5dec/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T165234-0c0b5dec/manifest.json</file>
    <file>.ddx/executions/20260505T165234-0c0b5dec/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="05da24bcba900e17fb5b52969dc199268db0fae4">
diff --git a/.ddx/executions/20260505T165234-0c0b5dec/checks/production-reachability.json b/.ddx/executions/20260505T165234-0c0b5dec/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T165234-0c0b5dec/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T165234-0c0b5dec/manifest.json b/.ddx/executions/20260505T165234-0c0b5dec/manifest.json
new file mode 100644
index 00000000..e4509be0
--- /dev/null
+++ b/.ddx/executions/20260505T165234-0c0b5dec/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260505T165234-0c0b5dec",
+  "bead_id": "ddx-e36d7959",
+  "base_rev": "0bf19d753f954388fe4ed5a973330afee0af0b0d",
+  "created_at": "2026-05-05T16:52:37.048621424Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-e36d7959",
+    "title": "try.Attempt: relocate rate-limit retry policy",
+    "description": "PROBLEM\\nThe per-bead HTTP 429 retry policy still lives in execute_bead.go, so the attempt path does not own Retry-After parsing, exponential backoff selection, or the per-bead retry budget. That makes rate-limit behavior depend on loop wiring instead of the attempt boundary that actually issues the dispatch.\\n\\nROOT CAUSE\\n- cli/internal/agent/execute_bead.go:800-851 builds RateLimitRetryConfig, installs the RecordRouteAttempt / bead-event hooks, and wraps dispatchAgentRun in RunWithRateLimitRetry.\\n- cli/internal/agent/try/attempt.go:99-137 still delegates only to legacy conflict recovery and never classifies or retries rate-limited attempts itself.\\n\\nPROPOSED FIX\\n- Move the Retry-After / exponential-backoff / budget-cap decision into cli/internal/agent/try/attempt.go on the rate-limit detection path.\\n- Keep the per-bead retry budget, per-wait cap, and routing-engine transparency hooks intact, but make them part of the attempt outcome contract rather than loop-local policy.\\n- Preserve the existing stable backoff schedule and budget-exhausted reason string.\\n- Reduce execute_bead.go to a generic caller of the structured attempt result.\\n\\nNON-SCOPE\\n- No_changes adjudication.\\n- Decomposition, push_failed, or push_conflict routing.\\n- Any change to the published backoff schedule or budget constants.\\n\\nINTERSECTIONS\\n- Absorbs ddx-c6e3db02 behavior.\\n",
+    "acceptance": "1. TestAttempt_RateLimit_Retry_HonorsRetryAfter verifies the attempt layer honors Retry-After exactly when present.\\n2. TestAttempt_RateLimit_Retry_UsesExponentialBackoff verifies the attempt layer falls back to the published backoff schedule when Retry-After is absent.\\n3. TestAttempt_RateLimit_WiresEvaluateRateLimitWait proves the rate-limit detection path calls EvaluateRateLimitWait from try.Attempt rather than only defining it.\\n4. TestRunWithRateLimitRetry_RetryAfterRespected and TestRunWithRateLimitRetry_ExponentialBackoffWhenRetryAfterAbsent remain green after the move.\\n5. cd cli \u0026\u0026 go test ./internal/agent/... green.\\n6. lefthook run pre-commit passes.",
+    "parent": "ddx-c8f79963",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "kind:refactor",
+      "story:10",
+      "observed-failure"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T16:52:34Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2464742",
+      "execute-loop-heartbeat-at": "2026-05-05T16:52:34.210796253Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T165234-0c0b5dec",
+    "prompt": ".ddx/executions/20260505T165234-0c0b5dec/prompt.md",
+    "manifest": ".ddx/executions/20260505T165234-0c0b5dec/manifest.json",
+    "result": ".ddx/executions/20260505T165234-0c0b5dec/result.json",
+    "checks": ".ddx/executions/20260505T165234-0c0b5dec/checks.json",
+    "usage": ".ddx/executions/20260505T165234-0c0b5dec/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-e36d7959-20260505T165234-0c0b5dec"
+  },
+  "prompt_sha": "364c0c6080d8d06139b214456520e90cc99b8c5698dc919ee53ea41e7ec731a2"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T165234-0c0b5dec/result.json b/.ddx/executions/20260505T165234-0c0b5dec/result.json
new file mode 100644
index 00000000..cf04407e
--- /dev/null
+++ b/.ddx/executions/20260505T165234-0c0b5dec/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-e36d7959",
+  "attempt_id": "20260505T165234-0c0b5dec",
+  "base_rev": "0bf19d753f954388fe4ed5a973330afee0af0b0d",
+  "result_rev": "c7a2770a01cccc5fe428bd52bd4d0220728c7a38",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-fe9385dd",
+  "duration_ms": 678687,
+  "tokens": 11063312,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T165234-0c0b5dec",
+  "prompt_file": ".ddx/executions/20260505T165234-0c0b5dec/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T165234-0c0b5dec/manifest.json",
+  "result_file": ".ddx/executions/20260505T165234-0c0b5dec/result.json",
+  "usage_file": ".ddx/executions/20260505T165234-0c0b5dec/usage.json",
+  "started_at": "2026-05-05T16:52:37.049055591Z",
+  "finished_at": "2026-05-05T17:03:55.73615871Z"
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

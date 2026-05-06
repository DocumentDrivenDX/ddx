<bead-review>
  <bead id="ddx-848069a3" iter=1>
    <title>C8: routing preflight moves to Drain startup (runs once, not per iteration)</title>
    <description>
execute_bead_loop.go:483-510 routing preflight runs per-iteration today AND treats its rejection as a worker-terminal exit (the user's original bug #2). Move to Drain startup: run once before the for-loop. If it fails, return one worker-level execution_failed record and exit (legitimate startup failure). Never run again per iteration.

INTERSECTIONS
- P1: routing preflight should fail open when its own check rejects.
- P2: preflight only owns startup advisory checks, not eligibility or provider availability.
- P3: skipped preflight should surface as an explicit event.
- P4: a preflight failure on one bead must not affect later beads.
    </description>
    <acceptance>
1. RoutePreflight invocation removed from per-iteration loop body in cli/internal/agent/execute_bead_loop.go (pre-refactor lines 483-510).
2. Called once in work.Worker.Drain bootstrap before for-loop starts.
3. Startup failure surfaces as Disposition=LoopError + ParkReason=preflight_failed message (not iteration-level execution_failed).
4. TestDrain_RoutingPreflightRunsOnce verifies preflight called exactly once even if Drain iterates multiple beads.
5. cd cli &amp;&amp; go test ./internal/agent/... green.
6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, refactor, kind:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T090443-7e445bee/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T090443-7e445bee/manifest.json</file>
    <file>.ddx/executions/20260506T090443-7e445bee/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="dbfdd522ce049a584ad3a3e1e3db6345cebc0a53">
<untrusted-data>
diff --git a/.ddx/executions/20260506T090443-7e445bee/checks/production-reachability.json b/.ddx/executions/20260506T090443-7e445bee/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T090443-7e445bee/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T090443-7e445bee/manifest.json b/.ddx/executions/20260506T090443-7e445bee/manifest.json
new file mode 100644
index 00000000..7552e0b6
--- /dev/null
+++ b/.ddx/executions/20260506T090443-7e445bee/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260506T090443-7e445bee",
+  "bead_id": "ddx-848069a3",
+  "base_rev": "76735fd34e5f5bac1d956b18cfd4c7c07ed1dc73",
+  "created_at": "2026-05-06T09:04:46.092900914Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-848069a3",
+    "title": "C8: routing preflight moves to Drain startup (runs once, not per iteration)",
+    "description": "execute_bead_loop.go:483-510 routing preflight runs per-iteration today AND treats its rejection as a worker-terminal exit (the user's original bug #2). Move to Drain startup: run once before the for-loop. If it fails, return one worker-level execution_failed record and exit (legitimate startup failure). Never run again per iteration.\n\nINTERSECTIONS\n- P1: routing preflight should fail open when its own check rejects.\n- P2: preflight only owns startup advisory checks, not eligibility or provider availability.\n- P3: skipped preflight should surface as an explicit event.\n- P4: a preflight failure on one bead must not affect later beads.",
+    "acceptance": "1. RoutePreflight invocation removed from per-iteration loop body in cli/internal/agent/execute_bead_loop.go (pre-refactor lines 483-510).\n2. Called once in work.Worker.Drain bootstrap before for-loop starts.\n3. Startup failure surfaces as Disposition=LoopError + ParkReason=preflight_failed message (not iteration-level execution_failed).\n4. TestDrain_RoutingPreflightRunsOnce verifies preflight called exactly once even if Drain iterates multiple beads.\n5. cd cli \u0026\u0026 go test ./internal/agent/... green.\n6. lefthook run pre-commit passes.",
+    "parent": "ddx-5cb6e6cd",
+    "labels": [
+      "phase:2",
+      "refactor",
+      "kind:fix"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T09:04:43Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T09:04:43.431136252Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T090443-7e445bee",
+    "prompt": ".ddx/executions/20260506T090443-7e445bee/prompt.md",
+    "manifest": ".ddx/executions/20260506T090443-7e445bee/manifest.json",
+    "result": ".ddx/executions/20260506T090443-7e445bee/result.json",
+    "checks": ".ddx/executions/20260506T090443-7e445bee/checks.json",
+    "usage": ".ddx/executions/20260506T090443-7e445bee/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-848069a3-20260506T090443-7e445bee"
+  },
+  "prompt_sha": "2dd82718e2faf877f383d3bccc55721b7a5616bcb7ad5bc1977b9eb3a02f7d46"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T090443-7e445bee/result.json b/.ddx/executions/20260506T090443-7e445bee/result.json
new file mode 100644
index 00000000..e2e97ef5
--- /dev/null
+++ b/.ddx/executions/20260506T090443-7e445bee/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-848069a3",
+  "attempt_id": "20260506T090443-7e445bee",
+  "base_rev": "76735fd34e5f5bac1d956b18cfd4c7c07ed1dc73",
+  "result_rev": "13071366e12282de5c541bb7cef50750aa8ffc2f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-cce0f5eb",
+  "duration_ms": 464166,
+  "tokens": 7863753,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T090443-7e445bee",
+  "prompt_file": ".ddx/executions/20260506T090443-7e445bee/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T090443-7e445bee/manifest.json",
+  "result_file": ".ddx/executions/20260506T090443-7e445bee/result.json",
+  "usage_file": ".ddx/executions/20260506T090443-7e445bee/usage.json",
+  "started_at": "2026-05-06T09:04:46.093227622Z",
+  "finished_at": "2026-05-06T09:12:30.259893725Z"
+}
\ No newline at end of file
</untrusted-data>
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

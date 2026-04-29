<bead-review>
  <bead id="ddx-87fb72c2" iter=1>
    <title>routing-config-deprecation: hard error on default_harness; opt-in for ladders/overrides</title>
    <description>
Cover D4 from ddx-fdd3ea36. Deliberate breaking change. - agent.routing.default_harness: field DELETED from config schema. Loading a config with it set produces a HARD ERROR at startup with a migration pointer. - agent.routing.profile_ladders: kept but gated behind explicit --escalate only; one-time process warning at config-load if set without --escalate ever being invoked. - agent.routing.model_overrides: same treatment as profile_ladders — explicit-opt-in via --override-model only; one-time warning otherwise. - agent.harness (top-level): kept, clarified in comments/docs as a tie-break preference, NOT a default override. Migration note in DDx CHANGELOG and docs/migrations/.
    </description>
    <acceptance>
1. Migration test: config containing agent.routing.default_harness fails at startup with migration message; exit code != 0. 2. Migration test: config containing agent.routing.profile_ladders + model_overrides loads with one-time warning; default path does NOT consult them; --escalate / --override-model do consult them. 3. Config schema: default_harness field removed (Go struct + JSON schema + sample config). 4. CHANGELOG entry under 'Breaking changes' with migration pointer. 5. docs/migrations/ (or equivalent) gains a routing-config migration guide.
    </acceptance>
    <labels>feat-006, routing, breaking-change</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T034538-9aa875d6/manifest.json</file>
    <file>.ddx/executions/20260429T034538-9aa875d6/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="42d2c0806ae9c513e68cd1ba40e6ee10deb676fb">
diff --git a/.ddx/executions/20260429T034538-9aa875d6/manifest.json b/.ddx/executions/20260429T034538-9aa875d6/manifest.json
new file mode 100644
index 00000000..0596d1e4
--- /dev/null
+++ b/.ddx/executions/20260429T034538-9aa875d6/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260429T034538-9aa875d6",
+  "bead_id": "ddx-87fb72c2",
+  "base_rev": "3f0173545b449059093034b28ace64b4c13a355d",
+  "created_at": "2026-04-29T03:45:39.210664917Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-87fb72c2",
+    "title": "routing-config-deprecation: hard error on default_harness; opt-in for ladders/overrides",
+    "description": "Cover D4 from ddx-fdd3ea36. Deliberate breaking change. - agent.routing.default_harness: field DELETED from config schema. Loading a config with it set produces a HARD ERROR at startup with a migration pointer. - agent.routing.profile_ladders: kept but gated behind explicit --escalate only; one-time process warning at config-load if set without --escalate ever being invoked. - agent.routing.model_overrides: same treatment as profile_ladders — explicit-opt-in via --override-model only; one-time warning otherwise. - agent.harness (top-level): kept, clarified in comments/docs as a tie-break preference, NOT a default override. Migration note in DDx CHANGELOG and docs/migrations/.",
+    "acceptance": "1. Migration test: config containing agent.routing.default_harness fails at startup with migration message; exit code != 0. 2. Migration test: config containing agent.routing.profile_ladders + model_overrides loads with one-time warning; default path does NOT consult them; --escalate / --override-model do consult them. 3. Config schema: default_harness field removed (Go struct + JSON schema + sample config). 4. CHANGELOG entry under 'Breaking changes' with migration pointer. 5. docs/migrations/ (or equivalent) gains a routing-config migration guide.",
+    "parent": "ddx-fdd3ea36",
+    "labels": [
+      "feat-006",
+      "routing",
+      "breaking-change"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T03:45:38Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "131811",
+      "execute-loop-heartbeat-at": "2026-04-29T03:45:38.662270725Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T034538-9aa875d6",
+    "prompt": ".ddx/executions/20260429T034538-9aa875d6/prompt.md",
+    "manifest": ".ddx/executions/20260429T034538-9aa875d6/manifest.json",
+    "result": ".ddx/executions/20260429T034538-9aa875d6/result.json",
+    "checks": ".ddx/executions/20260429T034538-9aa875d6/checks.json",
+    "usage": ".ddx/executions/20260429T034538-9aa875d6/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-87fb72c2-20260429T034538-9aa875d6"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T034538-9aa875d6/result.json b/.ddx/executions/20260429T034538-9aa875d6/result.json
new file mode 100644
index 00000000..35cbc3a8
--- /dev/null
+++ b/.ddx/executions/20260429T034538-9aa875d6/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-87fb72c2",
+  "attempt_id": "20260429T034538-9aa875d6",
+  "base_rev": "3f0173545b449059093034b28ace64b4c13a355d",
+  "result_rev": "fab54a93408951295081a4afc8d1dd91e8d20c85",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-995c0a57",
+  "duration_ms": 765653,
+  "tokens": 30056,
+  "cost_usd": 5.05551375,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T034538-9aa875d6",
+  "prompt_file": ".ddx/executions/20260429T034538-9aa875d6/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T034538-9aa875d6/manifest.json",
+  "result_file": ".ddx/executions/20260429T034538-9aa875d6/result.json",
+  "usage_file": ".ddx/executions/20260429T034538-9aa875d6/usage.json",
+  "started_at": "2026-04-29T03:45:39.210936291Z",
+  "finished_at": "2026-04-29T03:58:24.864150728Z"
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

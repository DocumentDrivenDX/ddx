<bead-review>
  <bead id="ddx-51ac997b" iter=1>
    <title>escalation: ladder.Next (MinPower-only output) + tests</title>
    <description>
Implement the escalation ladder as MinPower-only floors. NO vendor names. ladder.Next picks next floor on substantive failure. Skip-tier (no viable provider at floor) returns typed NoViableProviderError so the loop bumps further. Uses MinPower/MaxPower fields on ExecuteRequest that ALREADY exist in fizeau v0.9.28 (agent-79e194aa closed) — no upstream work needed for this bead. Reads previous attempt's RoutingActual.Power (lands via agent-3bb96bf5 / S10_1) to compute next floor.
    </description>
    <acceptance>
1. Ladder type + Next() method in cli/internal/agent/escalation/. 2. Unit tests cover all transitions. 3. Skip-tier returns typed NoViableProviderError. 4. Tests assert ladder uses RoutingActual.Power as the input for next-floor computation.
    </acceptance>
    <notes>
ladder.go and ladder_test.go deleted as part of ddx-abb40ce5 reach-backfill (no production callers via deadcode RTA). Wiring effort to recreate the primitive consolidated into ddx-cfedee8e.
    </notes>
    <labels>phase:2, story:10, area:routing, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260504T002204-d09d6171/manifest.json</file>
    <file>.ddx/executions/20260504T002204-d09d6171/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="11cffacc789d466a2b23ef15566aee819062028f">
diff --git a/.ddx/executions/20260504T002204-d09d6171/manifest.json b/.ddx/executions/20260504T002204-d09d6171/manifest.json
new file mode 100644
index 00000000..c64c28c6
--- /dev/null
+++ b/.ddx/executions/20260504T002204-d09d6171/manifest.json
@@ -0,0 +1,43 @@
+{
+  "attempt_id": "20260504T002204-d09d6171",
+  "bead_id": "ddx-51ac997b",
+  "base_rev": "d642be9662e3f454a224a159fac265e46cc1a624",
+  "created_at": "2026-05-04T00:22:09.523728154Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-51ac997b",
+    "title": "escalation: ladder.Next (MinPower-only output) + tests",
+    "description": "Implement the escalation ladder as MinPower-only floors. NO vendor names. ladder.Next picks next floor on substantive failure. Skip-tier (no viable provider at floor) returns typed NoViableProviderError so the loop bumps further. Uses MinPower/MaxPower fields on ExecuteRequest that ALREADY exist in fizeau v0.9.28 (agent-79e194aa closed) — no upstream work needed for this bead. Reads previous attempt's RoutingActual.Power (lands via agent-3bb96bf5 / S10_1) to compute next floor.",
+    "acceptance": "1. Ladder type + Next() method in cli/internal/agent/escalation/. 2. Unit tests cover all transitions. 3. Skip-tier returns typed NoViableProviderError. 4. Tests assert ladder uses RoutingActual.Power as the input for next-floor computation.",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "story:10",
+      "area:routing",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-04T00:22:04Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "closing_commit_sha": "97b7551257f1a9374d6face0ac081e2ec9c3ba02",
+      "events_attachment": "ddx-51ac997b/events.jsonl",
+      "execute-loop-heartbeat-at": "2026-05-04T00:22:04.687361375Z",
+      "execute-loop-no-changes-count": 1,
+      "session_id": "eb-4eee7fb9"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260504T002204-d09d6171",
+    "prompt": ".ddx/executions/20260504T002204-d09d6171/prompt.md",
+    "manifest": ".ddx/executions/20260504T002204-d09d6171/manifest.json",
+    "result": ".ddx/executions/20260504T002204-d09d6171/result.json",
+    "checks": ".ddx/executions/20260504T002204-d09d6171/checks.json",
+    "usage": ".ddx/executions/20260504T002204-d09d6171/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-51ac997b-20260504T002204-d09d6171"
+  },
+  "prompt_sha": "5c4f93dbf3f9265238aebf4c1fc6a5dffe4c08ad2854e345e2897f20a5504adb"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T002204-d09d6171/result.json b/.ddx/executions/20260504T002204-d09d6171/result.json
new file mode 100644
index 00000000..c8a44cea
--- /dev/null
+++ b/.ddx/executions/20260504T002204-d09d6171/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-51ac997b",
+  "attempt_id": "20260504T002204-d09d6171",
+  "base_rev": "d642be9662e3f454a224a159fac265e46cc1a624",
+  "result_rev": "799f7db2d453332fedc23f453bd874554d5d0595",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a02a1e1d",
+  "duration_ms": 228934,
+  "tokens": 5719,
+  "cost_usd": 1.2614992500000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260504T002204-d09d6171",
+  "prompt_file": ".ddx/executions/20260504T002204-d09d6171/prompt.md",
+  "manifest_file": ".ddx/executions/20260504T002204-d09d6171/manifest.json",
+  "result_file": ".ddx/executions/20260504T002204-d09d6171/result.json",
+  "usage_file": ".ddx/executions/20260504T002204-d09d6171/usage.json",
+  "started_at": "2026-05-04T00:22:09.524500487Z",
+  "finished_at": "2026-05-04T00:25:58.459336913Z"
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

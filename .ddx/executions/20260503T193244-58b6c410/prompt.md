<bead-review>
  <bead id="ddx-b69f04f8" iter=1>
    <title>federation: spoke lifecycle — idempotent register, jittered heartbeat, URL-change handling</title>
    <description>
Spoke side: --hub-address flag. Idempotent register on stable node_id. Jittered 30s heartbeat (stale at 2m → distinct 'offline' state on immediate fan-out failure). URL-change handling (re-register if local URL flips).
    </description>
    <acceptance>
1. --hub-address flag wired. 2. Register idempotent (duplicate node_id hard-rejected per locked decision). 3. Heartbeat jittered + 30s. 4. Stale at 2m. 5. URL-change re-registers. 6. Tests cover lifecycle.
    </acceptance>
    <labels>phase:2, story:14, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T163232-b1e455e9/manifest.json</file>
    <file>.ddx/executions/20260503T163232-b1e455e9/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2865f51fa2ea2acb2a6bcc5789b84f0e61565ae7">
diff --git a/.ddx/executions/20260503T163232-b1e455e9/manifest.json b/.ddx/executions/20260503T163232-b1e455e9/manifest.json
new file mode 100644
index 00000000..89c87225
--- /dev/null
+++ b/.ddx/executions/20260503T163232-b1e455e9/manifest.json
@@ -0,0 +1,39 @@
+{
+  "attempt_id": "20260503T163232-b1e455e9",
+  "bead_id": "ddx-b69f04f8",
+  "base_rev": "942f5e4ba24c97ec8d6acb80c7d5f750465b52b2",
+  "created_at": "2026-05-03T16:32:34.393761717Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b69f04f8",
+    "title": "federation: spoke lifecycle — idempotent register, jittered heartbeat, URL-change handling",
+    "description": "Spoke side: --hub-address flag. Idempotent register on stable node_id. Jittered 30s heartbeat (stale at 2m → distinct 'offline' state on immediate fan-out failure). URL-change handling (re-register if local URL flips).",
+    "acceptance": "1. --hub-address flag wired. 2. Register idempotent (duplicate node_id hard-rejected per locked decision). 3. Heartbeat jittered + 30s. 4. Stale at 2m. 5. URL-change re-registers. 6. Tests cover lifecycle.",
+    "parent": "ddx-a038a090",
+    "labels": [
+      "phase:2",
+      "story:14",
+      "area:server",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-03T16:32:32Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "463338",
+      "execute-loop-heartbeat-at": "2026-05-03T16:32:32.799540895Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260503T163232-b1e455e9",
+    "prompt": ".ddx/executions/20260503T163232-b1e455e9/prompt.md",
+    "manifest": ".ddx/executions/20260503T163232-b1e455e9/manifest.json",
+    "result": ".ddx/executions/20260503T163232-b1e455e9/result.json",
+    "checks": ".ddx/executions/20260503T163232-b1e455e9/checks.json",
+    "usage": ".ddx/executions/20260503T163232-b1e455e9/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b69f04f8-20260503T163232-b1e455e9"
+  },
+  "prompt_sha": "7fa97478fbc5afde0f246dfa36660911516f461382d453de5d524a1fd3546cc1"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260503T163232-b1e455e9/result.json b/.ddx/executions/20260503T163232-b1e455e9/result.json
new file mode 100644
index 00000000..666a0b06
--- /dev/null
+++ b/.ddx/executions/20260503T163232-b1e455e9/result.json
@@ -0,0 +1,19 @@
+{
+  "bead_id": "ddx-b69f04f8",
+  "attempt_id": "20260503T163232-b1e455e9",
+  "base_rev": "942f5e4ba24c97ec8d6acb80c7d5f750465b52b2",
+  "result_rev": "4c171a78efc8265889dedaa3d7923b102392ee05",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-d19ef3d3",
+  "duration_ms": 10804162,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T163232-b1e455e9",
+  "prompt_file": ".ddx/executions/20260503T163232-b1e455e9/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T163232-b1e455e9/manifest.json",
+  "result_file": ".ddx/executions/20260503T163232-b1e455e9/result.json",
+  "started_at": "2026-05-03T16:32:34.394078509Z",
+  "finished_at": "2026-05-03T19:32:38.556868663Z"
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

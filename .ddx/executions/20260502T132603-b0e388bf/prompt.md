<bead-review>
  <bead id="ddx-18b15451" iter=1>
    <title>artifacts: wire server-side search in frontend, debounce + stale-response drop</title>
    <description>
Frontend artifacts search currently runs over loaded edges only — incorrect past page 1. Wire to server-side artifacts(search:) with debounce + stale-response drop.
    </description>
    <acceptance>
1. Frontend search calls server. 2. Debounced (200ms typical). 3. Stale responses dropped on race. 4. Tests cover correctness past page 1.
    </acceptance>
    <labels>phase:2, story:7, area:web, kind:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T132304-eba8b624/manifest.json</file>
    <file>.ddx/executions/20260502T132304-eba8b624/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="dcbe7dc59c70228ac065ccf6241ccbe24695c9b6">
diff --git a/.ddx/executions/20260502T132304-eba8b624/manifest.json b/.ddx/executions/20260502T132304-eba8b624/manifest.json
new file mode 100644
index 00000000..dbdb02d0
--- /dev/null
+++ b/.ddx/executions/20260502T132304-eba8b624/manifest.json
@@ -0,0 +1,48 @@
+{
+  "attempt_id": "20260502T132304-eba8b624",
+  "bead_id": "ddx-18b15451",
+  "base_rev": "71e6f70861f03249b9a20028f417f36414309792",
+  "created_at": "2026-05-02T13:23:05.349867848Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-18b15451",
+    "title": "artifacts: wire server-side search in frontend, debounce + stale-response drop",
+    "description": "Frontend artifacts search currently runs over loaded edges only — incorrect past page 1. Wire to server-side artifacts(search:) with debounce + stale-response drop.",
+    "acceptance": "1. Frontend search calls server. 2. Debounced (200ms typical). 3. Stale responses dropped on race. 4. Tests cover correctness past page 1.",
+    "parent": "ddx-97335425",
+    "labels": [
+      "phase:2",
+      "story:7",
+      "area:web",
+      "kind:fix"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T13:23:03Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1727028",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "staging tracker: fatal: Unable to create '/home/erik/Projects/ddx/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository, or the lock file may be stale: exit status 128",
+          "created_at": "2026-05-02T13:01:01.195698559Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T13:23:03.997517135Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T132304-eba8b624",
+    "prompt": ".ddx/executions/20260502T132304-eba8b624/prompt.md",
+    "manifest": ".ddx/executions/20260502T132304-eba8b624/manifest.json",
+    "result": ".ddx/executions/20260502T132304-eba8b624/result.json",
+    "checks": ".ddx/executions/20260502T132304-eba8b624/checks.json",
+    "usage": ".ddx/executions/20260502T132304-eba8b624/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-18b15451-20260502T132304-eba8b624"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T132304-eba8b624/result.json b/.ddx/executions/20260502T132304-eba8b624/result.json
new file mode 100644
index 00000000..115178dd
--- /dev/null
+++ b/.ddx/executions/20260502T132304-eba8b624/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-18b15451",
+  "attempt_id": "20260502T132304-eba8b624",
+  "base_rev": "71e6f70861f03249b9a20028f417f36414309792",
+  "result_rev": "8d1edfa452b6ab528bd440c138b87eade6ef59ad",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-34ef16d9",
+  "duration_ms": 174836,
+  "tokens": 11048,
+  "cost_usd": 1.2124774999999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T132304-eba8b624",
+  "prompt_file": ".ddx/executions/20260502T132304-eba8b624/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T132304-eba8b624/manifest.json",
+  "result_file": ".ddx/executions/20260502T132304-eba8b624/result.json",
+  "usage_file": ".ddx/executions/20260502T132304-eba8b624/usage.json",
+  "started_at": "2026-05-02T13:23:05.350136473Z",
+  "finished_at": "2026-05-02T13:26:00.186436892Z"
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

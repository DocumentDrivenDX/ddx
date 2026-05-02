<bead-review>
  <bead id="ddx-c77a809c" iter=1>
    <title>operator-prompts: ADR — Operator-Prompt Beads as the Web Write Path</title>
    <description>
ADR locking the trust model: each prompt becomes a bead with issueType=operator-prompt, drained by the existing execute-loop. Reuses worktree isolation, land-coordinator, post-land build gate, evidence ledger. Audit = the bead itself. Synchronous in-process execution rejected. Per locked decision: full agent toolkit allowed (not narrow scope).
    </description>
    <acceptance>
1. ADR-&lt;NNN&gt; authored with explicit alternatives considered. 2. Trust model: localhost OR ts-net WhoIs (extends requireTrusted). 3. CSRF, DNS rebinding, XSS-becomes-write vectors documented. 4. ddx doc audit clean.
    </acceptance>
    <labels>phase:2, story:15, area:specs, kind:doc</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T140356-83dfa6ff/manifest.json</file>
    <file>.ddx/executions/20260502T140356-83dfa6ff/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f8f9589a2527a33fe36ea6100c76309a5cf0fefa">
diff --git a/.ddx/executions/20260502T140356-83dfa6ff/manifest.json b/.ddx/executions/20260502T140356-83dfa6ff/manifest.json
new file mode 100644
index 00000000..1c86d919
--- /dev/null
+++ b/.ddx/executions/20260502T140356-83dfa6ff/manifest.json
@@ -0,0 +1,48 @@
+{
+  "attempt_id": "20260502T140356-83dfa6ff",
+  "bead_id": "ddx-c77a809c",
+  "base_rev": "6df0b8977948b9e5efb7b045860f122349835ffb",
+  "created_at": "2026-05-02T14:03:57.587376232Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c77a809c",
+    "title": "operator-prompts: ADR — Operator-Prompt Beads as the Web Write Path",
+    "description": "ADR locking the trust model: each prompt becomes a bead with issueType=operator-prompt, drained by the existing execute-loop. Reuses worktree isolation, land-coordinator, post-land build gate, evidence ledger. Audit = the bead itself. Synchronous in-process execution rejected. Per locked decision: full agent toolkit allowed (not narrow scope).",
+    "acceptance": "1. ADR-\u003cNNN\u003e authored with explicit alternatives considered. 2. Trust model: localhost OR ts-net WhoIs (extends requireTrusted). 3. CSRF, DNS rebinding, XSS-becomes-write vectors documented. 4. ddx doc audit clean.",
+    "parent": "ddx-1d85c927",
+    "labels": [
+      "phase:2",
+      "story:15",
+      "area:specs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T14:03:56Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1727028",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "staging tracker: fatal: Unable to create '/home/erik/Projects/ddx/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository, or the lock file may be stale: exit status 128",
+          "created_at": "2026-05-02T13:01:02.20377217Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T14:03:56.239917806Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T140356-83dfa6ff",
+    "prompt": ".ddx/executions/20260502T140356-83dfa6ff/prompt.md",
+    "manifest": ".ddx/executions/20260502T140356-83dfa6ff/manifest.json",
+    "result": ".ddx/executions/20260502T140356-83dfa6ff/result.json",
+    "checks": ".ddx/executions/20260502T140356-83dfa6ff/checks.json",
+    "usage": ".ddx/executions/20260502T140356-83dfa6ff/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c77a809c-20260502T140356-83dfa6ff"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T140356-83dfa6ff/result.json b/.ddx/executions/20260502T140356-83dfa6ff/result.json
new file mode 100644
index 00000000..9c704093
--- /dev/null
+++ b/.ddx/executions/20260502T140356-83dfa6ff/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-c77a809c",
+  "attempt_id": "20260502T140356-83dfa6ff",
+  "base_rev": "6df0b8977948b9e5efb7b045860f122349835ffb",
+  "result_rev": "32ee71b6b059e6c8a7ebd6fd8c65a2293f514312",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a51db729",
+  "duration_ms": 72926,
+  "tokens": 3063,
+  "cost_usd": 0.49060499999999996,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T140356-83dfa6ff",
+  "prompt_file": ".ddx/executions/20260502T140356-83dfa6ff/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T140356-83dfa6ff/manifest.json",
+  "result_file": ".ddx/executions/20260502T140356-83dfa6ff/result.json",
+  "usage_file": ".ddx/executions/20260502T140356-83dfa6ff/usage.json",
+  "started_at": "2026-05-02T14:03:57.58767544Z",
+  "finished_at": "2026-05-02T14:05:10.513783077Z"
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

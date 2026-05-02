<bead-review>
  <bead id="ddx-cd2ecf79" iter=1>
    <title>artifact-types: FEAT-018 + FEAT-005 spec amendments</title>
    <description>
FEAT-018 (plugin API stability) registers the artifact-type discovery contract: plugins ship per-type definitions at &lt;plugin&gt;/workflows/**/artifacts/&lt;typeId&gt;/{meta.yml,template.md,prompt.md,example.md}. Plugins lacking workflows/ tree opt in via package.yaml.artifact_type_roots. FEAT-005 amendment: prefix lookup is plugin-defined; conventional types remain valid fallback.
    </description>
    <acceptance>
1. FEAT-018 documents the discovery contract + frontmatter shape. 2. FEAT-005 amended to reference plugin-declared types. 3. ddx doc audit clean. 4. New shape mandated; legacy HELIX shape supported as compat.
    </acceptance>
    <labels>phase:2, story:17, area:specs, kind:doc</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T140946-6045255a/manifest.json</file>
    <file>.ddx/executions/20260502T140946-6045255a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="53cac6fd568505d5442b276fa6d1c0b29baa0b6a">
diff --git a/.ddx/executions/20260502T140946-6045255a/manifest.json b/.ddx/executions/20260502T140946-6045255a/manifest.json
new file mode 100644
index 00000000..6b244dfd
--- /dev/null
+++ b/.ddx/executions/20260502T140946-6045255a/manifest.json
@@ -0,0 +1,48 @@
+{
+  "attempt_id": "20260502T140946-6045255a",
+  "bead_id": "ddx-cd2ecf79",
+  "base_rev": "238d3098dc5d72c481811b4e0b992fb88d383b15",
+  "created_at": "2026-05-02T14:09:47.781271633Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-cd2ecf79",
+    "title": "artifact-types: FEAT-018 + FEAT-005 spec amendments",
+    "description": "FEAT-018 (plugin API stability) registers the artifact-type discovery contract: plugins ship per-type definitions at \u003cplugin\u003e/workflows/**/artifacts/\u003ctypeId\u003e/{meta.yml,template.md,prompt.md,example.md}. Plugins lacking workflows/ tree opt in via package.yaml.artifact_type_roots. FEAT-005 amendment: prefix lookup is plugin-defined; conventional types remain valid fallback.",
+    "acceptance": "1. FEAT-018 documents the discovery contract + frontmatter shape. 2. FEAT-005 amended to reference plugin-declared types. 3. ddx doc audit clean. 4. New shape mandated; legacy HELIX shape supported as compat.",
+    "parent": "ddx-43d67aa5",
+    "labels": [
+      "phase:2",
+      "story:17",
+      "area:specs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T14:09:46Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1727028",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "staging tracker: fatal: Unable to create '/home/erik/Projects/ddx/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository, or the lock file may be stale: exit status 128",
+          "created_at": "2026-05-02T13:01:02.356940157Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T14:09:46.50944238Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T140946-6045255a",
+    "prompt": ".ddx/executions/20260502T140946-6045255a/prompt.md",
+    "manifest": ".ddx/executions/20260502T140946-6045255a/manifest.json",
+    "result": ".ddx/executions/20260502T140946-6045255a/result.json",
+    "checks": ".ddx/executions/20260502T140946-6045255a/checks.json",
+    "usage": ".ddx/executions/20260502T140946-6045255a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-cd2ecf79-20260502T140946-6045255a"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T140946-6045255a/result.json b/.ddx/executions/20260502T140946-6045255a/result.json
new file mode 100644
index 00000000..34c381d5
--- /dev/null
+++ b/.ddx/executions/20260502T140946-6045255a/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-cd2ecf79",
+  "attempt_id": "20260502T140946-6045255a",
+  "base_rev": "238d3098dc5d72c481811b4e0b992fb88d383b15",
+  "result_rev": "b07536a15f311ca3f272400dd823b5d60b5d17cb",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-ba323ebc",
+  "duration_ms": 185728,
+  "tokens": 10493,
+  "cost_usd": 1.1946825,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T140946-6045255a",
+  "prompt_file": ".ddx/executions/20260502T140946-6045255a/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T140946-6045255a/manifest.json",
+  "result_file": ".ddx/executions/20260502T140946-6045255a/result.json",
+  "usage_file": ".ddx/executions/20260502T140946-6045255a/usage.json",
+  "started_at": "2026-05-02T14:09:47.781541799Z",
+  "finished_at": "2026-05-02T14:12:53.509630215Z"
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

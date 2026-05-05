<bead-review>
  <bead id="ddx-1d52a30c" iter=1>
    <title>artifact-types: cli/internal/artifacttypes/ loader + index + mtime cache + path-escape guard</title>
    <description>
New package cli/internal/artifacttypes/. Scans &lt;plugin&gt;/workflows/**/artifacts/*/meta.yml by default; respects package.yaml.artifact_type_roots opt-in. Normalizes to {plugin, typeId, name, description, prefix, pattern, phase, templatePath, promptPath, examples[], sourceMetaPath}. mtime-based cache. Path-escape guard for plugin paths.
    </description>
    <acceptance>
1. Loader package built. 2. mtime cache invalidates correctly. 3. Path-escape guard refuses meta.yml outside the plugin tree. 4. Tests cover both new + legacy meta.yml shapes.
    </acceptance>
    <labels>phase:2, story:17, area:server, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T112843-1827214a/manifest.json</file>
    <file>.ddx/executions/20260505T112843-1827214a/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="bdee418b5ee3b18df41e83a6b0b5e39f923bb2de">
diff --git a/.ddx/executions/20260505T112843-1827214a/manifest.json b/.ddx/executions/20260505T112843-1827214a/manifest.json
new file mode 100644
index 00000000..5bc5ed5d
--- /dev/null
+++ b/.ddx/executions/20260505T112843-1827214a/manifest.json
@@ -0,0 +1,80 @@
+{
+  "attempt_id": "20260505T112843-1827214a",
+  "bead_id": "ddx-1d52a30c",
+  "base_rev": "c0e972b7e6ba2b42f7467bd9f605d766bd2317ea",
+  "created_at": "2026-05-05T11:28:45.231472478Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-1d52a30c",
+    "title": "artifact-types: cli/internal/artifacttypes/ loader + index + mtime cache + path-escape guard",
+    "description": "New package cli/internal/artifacttypes/. Scans \u003cplugin\u003e/workflows/**/artifacts/*/meta.yml by default; respects package.yaml.artifact_type_roots opt-in. Normalizes to {plugin, typeId, name, description, prefix, pattern, phase, templatePath, promptPath, examples[], sourceMetaPath}. mtime-based cache. Path-escape guard for plugin paths.",
+    "acceptance": "1. Loader package built. 2. mtime cache invalidates correctly. 3. Path-escape guard refuses meta.yml outside the plugin tree. 4. Tests cover both new + legacy meta.yml shapes.",
+    "parent": "ddx-43d67aa5",
+    "labels": [
+      "phase:2",
+      "story:17",
+      "area:server",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T11:28:43Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T05:30:32.20749592Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T052403-ae167f33\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":5331020,\"output_tokens\":50871,\"total_tokens\":5381891,\"cost_usd\":0,\"duration_ms\":386410,\"exit_code\":0}",
+          "created_at": "2026-05-05T05:30:32.449914831Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=5381891 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T05:30:39.736690182Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: unparseable\nattempt_count=1\nresult_rev=4329443d7003727718f1ad1770d9b4a0e63169de\n\nreviewer: review-error: unparseable: reviewer output: unparseable JSON verdict: no JSON object found (raw output 66 bytes; see .ddx/executions/20260505T053040-97a38850)\nharness=claude\nmodel=opus\ninput_bytes=6631\noutput_bytes=66\nelapsed_ms=48975",
+          "created_at": "2026-05-05T05:31:31.519075043Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: unparseable"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=4329443d7003727718f1ad1770d9b4a0e63169de\nbase_rev=ea26f7c593dbee0d1c1b9f0005aec6bfd2c72b71",
+          "created_at": "2026-05-05T05:31:31.746237465Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T11:28:43.26272344Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T112843-1827214a",
+    "prompt": ".ddx/executions/20260505T112843-1827214a/prompt.md",
+    "manifest": ".ddx/executions/20260505T112843-1827214a/manifest.json",
+    "result": ".ddx/executions/20260505T112843-1827214a/result.json",
+    "checks": ".ddx/executions/20260505T112843-1827214a/checks.json",
+    "usage": ".ddx/executions/20260505T112843-1827214a/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-1d52a30c-20260505T112843-1827214a"
+  },
+  "prompt_sha": "25527baa6a65abb27475dab49ec082817cce2500559cc9182814c1820a5ce250"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T112843-1827214a/result.json b/.ddx/executions/20260505T112843-1827214a/result.json
new file mode 100644
index 00000000..5ec639eb
--- /dev/null
+++ b/.ddx/executions/20260505T112843-1827214a/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-1d52a30c",
+  "attempt_id": "20260505T112843-1827214a",
+  "base_rev": "c0e972b7e6ba2b42f7467bd9f605d766bd2317ea",
+  "result_rev": "48335b62dd697307b5861808b814b8572ff8f624",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-a3dbb85b",
+  "duration_ms": 95182,
+  "tokens": 1037832,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T112843-1827214a",
+  "prompt_file": ".ddx/executions/20260505T112843-1827214a/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T112843-1827214a/manifest.json",
+  "result_file": ".ddx/executions/20260505T112843-1827214a/result.json",
+  "usage_file": ".ddx/executions/20260505T112843-1827214a/usage.json",
+  "started_at": "2026-05-05T11:28:45.231814227Z",
+  "finished_at": "2026-05-05T11:30:20.414656038Z"
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

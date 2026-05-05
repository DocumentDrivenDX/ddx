<bead-review>
  <bead id="ddx-296019fe" iter=1>
    <title>metric: integration round-trip test + mixed-unit refusal</title>
    <description>
End-to-end: define MET → exec.run → metric history populated → ddx metric show reads back. Compare/Trend refuse mixed units; History groups by unit.
    </description>
    <acceptance>
1. Integration test covers full round-trip. 2. Compare/Trend refuse mixed units with clear error. 3. History grouping correct.
    </acceptance>
    <labels>phase:2, story:13, area:tests, kind:test</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T205301-a9fc55c6/manifest.json</file>
    <file>.ddx/executions/20260505T205301-a9fc55c6/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f4390e5ccd793a1e415d1ca99473b61bba7fbefb">
<untrusted-data>
diff --git a/.ddx/executions/20260505T205301-a9fc55c6/manifest.json b/.ddx/executions/20260505T205301-a9fc55c6/manifest.json
new file mode 100644
index 00000000..48208b3f
--- /dev/null
+++ b/.ddx/executions/20260505T205301-a9fc55c6/manifest.json
@@ -0,0 +1,80 @@
+{
+  "attempt_id": "20260505T205301-a9fc55c6",
+  "bead_id": "ddx-296019fe",
+  "base_rev": "8d5786c6789b00446b10c26680343db136614d9e",
+  "created_at": "2026-05-05T20:53:04.268847312Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-296019fe",
+    "title": "metric: integration round-trip test + mixed-unit refusal",
+    "description": "End-to-end: define MET → exec.run → metric history populated → ddx metric show reads back. Compare/Trend refuse mixed units; History groups by unit.",
+    "acceptance": "1. Integration test covers full round-trip. 2. Compare/Trend refuse mixed units with clear error. 3. History grouping correct.",
+    "parent": "ddx-921616ea",
+    "labels": [
+      "phase:2",
+      "story:13",
+      "area:tests",
+      "kind:test"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T20:53:01Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3918937",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T15:39:25.31059289Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T152806-9d9de558\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":12720048,\"output_tokens\":32943,\"total_tokens\":12752991,\"cost_usd\":0,\"duration_ms\":676487,\"exit_code\":0}",
+          "created_at": "2026-05-05T15:39:25.560700701Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=12752991 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T15:39:35.065532676Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=20cc5c6b17170528f48bf9ceeaf55b783c77494c\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T11:44:39-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=6078\noutput_bytes=0\nelapsed_ms=4140",
+          "created_at": "2026-05-05T15:39:39.73860936Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=20cc5c6b17170528f48bf9ceeaf55b783c77494c\nbase_rev=b0ed49825be9b52a5d0c602bd15b43c05eda1b88",
+          "created_at": "2026-05-05T15:39:39.958306506Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T20:53:01.682630829Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T205301-a9fc55c6",
+    "prompt": ".ddx/executions/20260505T205301-a9fc55c6/prompt.md",
+    "manifest": ".ddx/executions/20260505T205301-a9fc55c6/manifest.json",
+    "result": ".ddx/executions/20260505T205301-a9fc55c6/result.json",
+    "checks": ".ddx/executions/20260505T205301-a9fc55c6/checks.json",
+    "usage": ".ddx/executions/20260505T205301-a9fc55c6/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-296019fe-20260505T205301-a9fc55c6"
+  },
+  "prompt_sha": "95283625932d01a5cdcc1fa244c6ce3b11ccd17a23b944a4312e438fbe9116ab"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T205301-a9fc55c6/result.json b/.ddx/executions/20260505T205301-a9fc55c6/result.json
new file mode 100644
index 00000000..65fc0a24
--- /dev/null
+++ b/.ddx/executions/20260505T205301-a9fc55c6/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-296019fe",
+  "attempt_id": "20260505T205301-a9fc55c6",
+  "base_rev": "8d5786c6789b00446b10c26680343db136614d9e",
+  "result_rev": "9674d3fcb8f75abac36592bbb85604e5bef4032e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-e760291e",
+  "duration_ms": 52886,
+  "tokens": 532268,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T205301-a9fc55c6",
+  "prompt_file": ".ddx/executions/20260505T205301-a9fc55c6/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T205301-a9fc55c6/manifest.json",
+  "result_file": ".ddx/executions/20260505T205301-a9fc55c6/result.json",
+  "usage_file": ".ddx/executions/20260505T205301-a9fc55c6/usage.json",
+  "started_at": "2026-05-05T20:53:04.269291687Z",
+  "finished_at": "2026-05-05T20:53:57.1555375Z"
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

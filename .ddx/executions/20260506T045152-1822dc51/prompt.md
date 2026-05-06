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
    <file>.ddx/executions/20260506T045018-89fe1ea3/manifest.json</file>
    <file>.ddx/executions/20260506T045018-89fe1ea3/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a19cae698a08a30da9ce71e8f2cc92a67e40df39">
<untrusted-data>
diff --git a/.ddx/executions/20260506T045018-89fe1ea3/manifest.json b/.ddx/executions/20260506T045018-89fe1ea3/manifest.json
new file mode 100644
index 00000000..28796a07
--- /dev/null
+++ b/.ddx/executions/20260506T045018-89fe1ea3/manifest.json
@@ -0,0 +1,160 @@
+{
+  "attempt_id": "20260506T045018-89fe1ea3",
+  "bead_id": "ddx-296019fe",
+  "base_rev": "b0fb64eb1acbd4da69e4e17c1e7762647cb0b034",
+  "created_at": "2026-05-06T04:50:20.850843127Z",
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
+      "claimed-at": "2026-05-06T04:50:18Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
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
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T20:53:57.157929872Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T205301-a9fc55c6\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":528082,\"output_tokens\":4186,\"total_tokens\":532268,\"cost_usd\":0,\"duration_ms\":52886,\"exit_code\":0}",
+          "created_at": "2026-05-05T20:53:57.389059901Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=532268 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T20:54:05.254403667Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=f4390e5ccd793a1e415d1ca99473b61bba7fbefb\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T16:59:09-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=7903\noutput_bytes=0\nelapsed_ms=4129",
+          "created_at": "2026-05-05T20:54:09.930526942Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=f4390e5ccd793a1e415d1ca99473b61bba7fbefb\nbase_rev=8d5786c6789b00446b10c26680343db136614d9e",
+          "created_at": "2026-05-05T20:54:10.135316538Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T02:35:31.955831613Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T023449-a3c451b6\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":406960,\"output_tokens\":2818,\"total_tokens\":409778,\"cost_usd\":0,\"duration_ms\":40048,\"exit_code\":0}",
+          "created_at": "2026-05-06T02:35:32.159094508Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=409778 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T02:35:37.646901508Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=670c6b83b90d59dcc0aa25fb5448d5e51a6a0c35\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T22:40:42-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=10203\noutput_bytes=0\nelapsed_ms=4115",
+          "created_at": "2026-05-06T02:35:42.275869605Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=670c6b83b90d59dcc0aa25fb5448d5e51a6a0c35\nbase_rev=53e305bb86f52a066577f86ad64682d89a64da26",
+          "created_at": "2026-05-06T02:35:42.498957852Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T04:50:18.167572268Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T045018-89fe1ea3",
+    "prompt": ".ddx/executions/20260506T045018-89fe1ea3/prompt.md",
+    "manifest": ".ddx/executions/20260506T045018-89fe1ea3/manifest.json",
+    "result": ".ddx/executions/20260506T045018-89fe1ea3/result.json",
+    "checks": ".ddx/executions/20260506T045018-89fe1ea3/checks.json",
+    "usage": ".ddx/executions/20260506T045018-89fe1ea3/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-296019fe-20260506T045018-89fe1ea3"
+  },
+  "prompt_sha": "37e0efb40853627b1c6fb51d309357b44cc55f88b2aac7cdbb2b188696484f21"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T045018-89fe1ea3/result.json b/.ddx/executions/20260506T045018-89fe1ea3/result.json
new file mode 100644
index 00000000..5b1e6168
--- /dev/null
+++ b/.ddx/executions/20260506T045018-89fe1ea3/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-296019fe",
+  "attempt_id": "20260506T045018-89fe1ea3",
+  "base_rev": "b0fb64eb1acbd4da69e4e17c1e7762647cb0b034",
+  "result_rev": "7c161c0dd97d9d9dff70655ea0230a9798bc058b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-49c4794c",
+  "duration_ms": 81275,
+  "tokens": 774838,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T045018-89fe1ea3",
+  "prompt_file": ".ddx/executions/20260506T045018-89fe1ea3/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T045018-89fe1ea3/manifest.json",
+  "result_file": ".ddx/executions/20260506T045018-89fe1ea3/result.json",
+  "usage_file": ".ddx/executions/20260506T045018-89fe1ea3/usage.json",
+  "started_at": "2026-05-06T04:50:20.851272627Z",
+  "finished_at": "2026-05-06T04:51:42.126419106Z"
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

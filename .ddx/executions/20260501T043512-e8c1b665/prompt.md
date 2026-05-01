<bead-review>
  <bead id="ddx-bd37afc7" iter=1>
    <title>Remove live harness execution tests from DDx</title>
    <description>
DDx should not execute real third-party harness binaries now that harness ownership lives in Fizeau. Remove or quarantine live harness execution tests such as opencode/claude/codex echo tests from DDx packages, and replace any needed DDx coverage with Fizeau service stubs, fake executors, or contract-level assertions. The normal DDx suite should not start opencode, claude, codex, gemini, pi, or other real harness processes.
    </description>
    <acceptance>
1. DDx go test ./... does not invoke real harness binaries (opencode, claude, codex, gemini, pi) as part of the default or short test suites.\n2. Existing DDx behavior coverage is preserved with stubs/fakes at the DDx-Fizeau boundary where needed.\n3. Any remaining live harness smoke tests are moved to Fizeau or gated behind an explicit opt-in env var/build tag with a clear comment.\n4. go test ./internal/agent ./cmd passes without requiring installed harness CLIs.\n5. A grep/static check documents the remaining allowed harness binary invocations in tests.
    </acceptance>
    <notes>
REVIEW:BLOCK

Diff contains only an execution result.json artifact; no code or test changes are present to evaluate against any acceptance criterion.
    </notes>
    <labels>area:agent, area:test, kind:cleanup</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260501T042518-6985c3b2/manifest.json</file>
    <file>.ddx/executions/20260501T042518-6985c3b2/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="9b6086e273907706d6a5259eb261d2488410d606">
diff --git a/.ddx/executions/20260501T042518-6985c3b2/manifest.json b/.ddx/executions/20260501T042518-6985c3b2/manifest.json
new file mode 100644
index 00000000..12a6ef15
--- /dev/null
+++ b/.ddx/executions/20260501T042518-6985c3b2/manifest.json
@@ -0,0 +1,86 @@
+{
+  "attempt_id": "20260501T042518-6985c3b2",
+  "bead_id": "ddx-bd37afc7",
+  "base_rev": "7f103d50c2428ac295c9d5560da97a9d9450a982",
+  "created_at": "2026-05-01T04:25:19.931495902Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-bd37afc7",
+    "title": "Remove live harness execution tests from DDx",
+    "description": "DDx should not execute real third-party harness binaries now that harness ownership lives in Fizeau. Remove or quarantine live harness execution tests such as opencode/claude/codex echo tests from DDx packages, and replace any needed DDx coverage with Fizeau service stubs, fake executors, or contract-level assertions. The normal DDx suite should not start opencode, claude, codex, gemini, pi, or other real harness processes.",
+    "acceptance": "1. DDx go test ./... does not invoke real harness binaries (opencode, claude, codex, gemini, pi) as part of the default or short test suites.\\n2. Existing DDx behavior coverage is preserved with stubs/fakes at the DDx-Fizeau boundary where needed.\\n3. Any remaining live harness smoke tests are moved to Fizeau or gated behind an explicit opt-in env var/build tag with a clear comment.\\n4. go test ./internal/agent ./cmd passes without requiring installed harness CLIs.\\n5. A grep/static check documents the remaining allowed harness binary invocations in tests.",
+    "labels": [
+      "area:agent",
+      "area:test",
+      "kind:cleanup"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-01T04:25:18Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3396374",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[],\"requested_harness\":\"claude\"}",
+          "created_at": "2026-05-01T04:09:10.825148673Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260501T040146-d9dcddba\",\"harness\":\"claude\",\"input_tokens\":31,\"output_tokens\":10973,\"total_tokens\":11004,\"cost_usd\":1.3745802999999999,\"duration_ms\":443676,\"exit_code\":0}",
+          "created_at": "2026-05-01T04:09:10.931197117Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=11004 cost_usd=1.3746"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-01T04:09:15.514003516Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "Diff contains only an execution result.json artifact; no code or test changes are present to evaluate against any acceptance criterion.\nharness=claude\nmodel=opus\ninput_bytes=4145\noutput_bytes=500\nelapsed_ms=7272",
+          "created_at": "2026-05-01T04:09:22.974967968Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "BLOCK"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-05-01T04:09:23.087227615Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: BLOCK"
+        },
+        {
+          "actor": "ddx",
+          "body": "post-merge review: BLOCK (flagged for human)\nDiff contains only an execution result.json artifact; no code or test changes are present to evaluate against any acceptance criterion.\nresult_rev=dfbaf1f3c0ff5aac557b014e7b6c65f4331944d2\nbase_rev=c015d68433d939b3e4379a464c91cc2ea014e648",
+          "created_at": "2026-05-01T04:09:23.188063272Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_block"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-01T04:25:18.533005176Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260501T042518-6985c3b2",
+    "prompt": ".ddx/executions/20260501T042518-6985c3b2/prompt.md",
+    "manifest": ".ddx/executions/20260501T042518-6985c3b2/manifest.json",
+    "result": ".ddx/executions/20260501T042518-6985c3b2/result.json",
+    "checks": ".ddx/executions/20260501T042518-6985c3b2/checks.json",
+    "usage": ".ddx/executions/20260501T042518-6985c3b2/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-bd37afc7-20260501T042518-6985c3b2"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260501T042518-6985c3b2/result.json b/.ddx/executions/20260501T042518-6985c3b2/result.json
new file mode 100644
index 00000000..1db33531
--- /dev/null
+++ b/.ddx/executions/20260501T042518-6985c3b2/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-bd37afc7",
+  "attempt_id": "20260501T042518-6985c3b2",
+  "base_rev": "7f103d50c2428ac295c9d5560da97a9d9450a982",
+  "result_rev": "e705cc419d73e3a83b419e158165208dafa97f4b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-3e19c75b",
+  "duration_ms": 588158,
+  "tokens": 10090,
+  "cost_usd": 1.27620975,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260501T042518-6985c3b2",
+  "prompt_file": ".ddx/executions/20260501T042518-6985c3b2/prompt.md",
+  "manifest_file": ".ddx/executions/20260501T042518-6985c3b2/manifest.json",
+  "result_file": ".ddx/executions/20260501T042518-6985c3b2/result.json",
+  "usage_file": ".ddx/executions/20260501T042518-6985c3b2/usage.json",
+  "started_at": "2026-05-01T04:25:19.931929484Z",
+  "finished_at": "2026-05-01T04:35:08.090534404Z"
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

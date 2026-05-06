<bead-review>
  <bead id="ddx-9c81b20f" iter=1>
    <title>cli: wire ddx try quality hooks into runtime</title>
    <description>
PROBLEM
  ddx try constructs ExecuteBeadLoopRuntime with PreClaimHook only, so even after quality hooks exist in the agent package, direct ddx try invocations will not run lint or triage.

ROOT CAUSE
  cli/cmd/try.go:320-329 constructs ExecuteBeadLoopRuntime for ddx try and currently sets Once, Log, EventSink, WorkerID, ProjectRoot, SessionID, PreClaimHook, and NoReview. There are no PreDispatchLintHook or PostAttemptTriageHook assignments at this wire site.

PROPOSED FIX
  - In cli/cmd/try.go, build and assign PreDispatchLintHook and PostAttemptTriageHook when constructing ExecuteBeadLoopRuntime.
  - Reuse the agent-package hook constructors from the lint and triage child beads and pass the resolved config/project root/runner plumbing they require.
  - Default behavior remains warn-only/fail-open as implemented by the loop and hook layers.
  - Add --no-lint and --no-triage flags only if needed by the implementation; otherwise keep defaults with no extra flags.
  - Ensure operator output remains clear when lint blocks under opt-in block mode.

NON-SCOPE
  - Implementing hook contracts or hook bodies; depends on child beads.
  - ddx work integration.
  - BLOCK threshold policy tuning.
  - GraphQL exposure of OutcomeReason.

PARENT
  ddx-e1a576a7.

DEPS
  - ddx-d1dae2dd: pre-dispatch lint loop policy must exist first.
  - ddx-68706ef9: runner-backed lint hook must exist first.
  - ddx-4375292b: runner-backed post-attempt triage hook must exist first.
  - ddx-e1a576a7: parent decomposition edge requested by the parent attempt instructions.
    </description>
    <acceptance>
1. cli/cmd/try.go wires PreDispatchLintHook into ExecuteBeadLoopRuntime construction for ddx try.
2. cli/cmd/try.go wires PostAttemptTriageHook into ExecuteBeadLoopRuntime construction for ddx try.
3. TestTry_HooksWired proves the command runtime receives both hooks when ddx try runs with default options.
4. WIRED-IN: git grep PreDispatchLintHook cli/cmd/ shows the try.go wire site.
5. WIRED-IN: git grep PostAttemptTriageHook cli/cmd/ shows the try.go wire site.
6. cd cli &amp;&amp; go test ./cmd/... ./internal/agent/... green.
7. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, area:cli, kind:feature, reliability, bead-quality, adr:023</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T023613-5f698e51/manifest.json</file>
    <file>.ddx/executions/20260506T023613-5f698e51/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="38249f6a4e99725124bf4ed20f40c16d3ba30367">
<untrusted-data>
diff --git a/.ddx/executions/20260506T023613-5f698e51/manifest.json b/.ddx/executions/20260506T023613-5f698e51/manifest.json
new file mode 100644
index 00000000..bc53e1ca
--- /dev/null
+++ b/.ddx/executions/20260506T023613-5f698e51/manifest.json
@@ -0,0 +1,123 @@
+{
+  "attempt_id": "20260506T023613-5f698e51",
+  "bead_id": "ddx-9c81b20f",
+  "base_rev": "4daf0ed87d37cec24b3a252e684cc54884f000b8",
+  "created_at": "2026-05-06T02:36:16.199486028Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9c81b20f",
+    "title": "cli: wire ddx try quality hooks into runtime",
+    "description": "PROBLEM\n  ddx try constructs ExecuteBeadLoopRuntime with PreClaimHook only, so even after quality hooks exist in the agent package, direct ddx try invocations will not run lint or triage.\n\nROOT CAUSE\n  cli/cmd/try.go:320-329 constructs ExecuteBeadLoopRuntime for ddx try and currently sets Once, Log, EventSink, WorkerID, ProjectRoot, SessionID, PreClaimHook, and NoReview. There are no PreDispatchLintHook or PostAttemptTriageHook assignments at this wire site.\n\nPROPOSED FIX\n  - In cli/cmd/try.go, build and assign PreDispatchLintHook and PostAttemptTriageHook when constructing ExecuteBeadLoopRuntime.\n  - Reuse the agent-package hook constructors from the lint and triage child beads and pass the resolved config/project root/runner plumbing they require.\n  - Default behavior remains warn-only/fail-open as implemented by the loop and hook layers.\n  - Add --no-lint and --no-triage flags only if needed by the implementation; otherwise keep defaults with no extra flags.\n  - Ensure operator output remains clear when lint blocks under opt-in block mode.\n\nNON-SCOPE\n  - Implementing hook contracts or hook bodies; depends on child beads.\n  - ddx work integration.\n  - BLOCK threshold policy tuning.\n  - GraphQL exposure of OutcomeReason.\n\nPARENT\n  ddx-e1a576a7.\n\nDEPS\n  - ddx-d1dae2dd: pre-dispatch lint loop policy must exist first.\n  - ddx-68706ef9: runner-backed lint hook must exist first.\n  - ddx-4375292b: runner-backed post-attempt triage hook must exist first.\n  - ddx-e1a576a7: parent decomposition edge requested by the parent attempt instructions.",
+    "acceptance": "1. cli/cmd/try.go wires PreDispatchLintHook into ExecuteBeadLoopRuntime construction for ddx try.\n2. cli/cmd/try.go wires PostAttemptTriageHook into ExecuteBeadLoopRuntime construction for ddx try.\n3. TestTry_HooksWired proves the command runtime receives both hooks when ddx try runs with default options.\n4. WIRED-IN: git grep PreDispatchLintHook cli/cmd/ shows the try.go wire site.\n5. WIRED-IN: git grep PostAttemptTriageHook cli/cmd/ shows the try.go wire site.\n6. cd cli \u0026\u0026 go test ./cmd/... ./internal/agent/... green.\n7. lefthook run pre-commit passes.",
+    "parent": "ddx-e1a576a7",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "area:cli",
+      "kind:feature",
+      "reliability",
+      "bead-quality",
+      "adr:023"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T02:36:13Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T23:42:37.188515696Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T234013-2c206bcf\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":884774,\"output_tokens\":6011,\"total_tokens\":890785,\"cost_usd\":0,\"duration_ms\":140709,\"exit_code\":0}",
+          "created_at": "2026-05-05T23:42:37.401420356Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=890785 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T23:42:49.246033426Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=af16757c77cf1a9c02d8f8487a795817395681f3\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T19:47:53-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=9387\noutput_bytes=0\nelapsed_ms=4123",
+          "created_at": "2026-05-05T23:42:53.880030083Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=af16757c77cf1a9c02d8f8487a795817395681f3\nbase_rev=38f3c3837c4678b7ea76f122711dd78b87e7e518",
+          "created_at": "2026-05-05T23:42:54.075263772Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T00:53:22.219436752Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T005206-bb877a90\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":494820,\"output_tokens\":3955,\"total_tokens\":498775,\"cost_usd\":0,\"duration_ms\":73909,\"exit_code\":0}",
+          "created_at": "2026-05-06T00:53:22.451149261Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=498775 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T00:53:31.088627936Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=e213cdf56d28f4457f5aeb2e366de8998f044382\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T20:58:35-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=11714\noutput_bytes=0\nelapsed_ms=4080",
+          "created_at": "2026-05-06T00:53:35.655545808Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=e213cdf56d28f4457f5aeb2e366de8998f044382\nbase_rev=6904320a1869a05af69612105d4e05af1b184d49",
+          "created_at": "2026-05-06T00:53:35.867779337Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T02:36:13.54420074Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T023613-5f698e51",
+    "prompt": ".ddx/executions/20260506T023613-5f698e51/prompt.md",
+    "manifest": ".ddx/executions/20260506T023613-5f698e51/manifest.json",
+    "result": ".ddx/executions/20260506T023613-5f698e51/result.json",
+    "checks": ".ddx/executions/20260506T023613-5f698e51/checks.json",
+    "usage": ".ddx/executions/20260506T023613-5f698e51/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9c81b20f-20260506T023613-5f698e51"
+  },
+  "prompt_sha": "8eb3ecb71a311e90f79f8d5d1e4b03f2cee13e25cafe84983e66870c63ca149f"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T023613-5f698e51/result.json b/.ddx/executions/20260506T023613-5f698e51/result.json
new file mode 100644
index 00000000..f0deca99
--- /dev/null
+++ b/.ddx/executions/20260506T023613-5f698e51/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-9c81b20f",
+  "attempt_id": "20260506T023613-5f698e51",
+  "base_rev": "4daf0ed87d37cec24b3a252e684cc54884f000b8",
+  "result_rev": "09c7d224f29bf8735830bebe11077e1f8e47502b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-2e9e507b",
+  "duration_ms": 94053,
+  "tokens": 711740,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T023613-5f698e51",
+  "prompt_file": ".ddx/executions/20260506T023613-5f698e51/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T023613-5f698e51/manifest.json",
+  "result_file": ".ddx/executions/20260506T023613-5f698e51/result.json",
+  "usage_file": ".ddx/executions/20260506T023613-5f698e51/usage.json",
+  "started_at": "2026-05-06T02:36:16.201249401Z",
+  "finished_at": "2026-05-06T02:37:50.254493026Z"
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

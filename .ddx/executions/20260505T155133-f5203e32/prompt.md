<bead-review>
  <bead id="ddx-68706ef9" iter=1>
    <title>agent: implement runner-backed bead lifecycle lint hook</title>
    <description>
PROBLEM
  The pre-dispatch lint hook needs a production implementation that invokes the bead-lifecycle skill through the internal runner library, not by spawning ddx agent run as a subprocess.

ROOT CAUSE
  cli/internal/agent/dispatch.go:35-63 centralizes agent invocation in dispatchViaResolvedConfig. cli/internal/agent/execute_bead.go:168 defines the existing AgentRunner injection seam as Run(opts RunArgs) (*Result, error). cli/internal/agent/execute_bead.go:1110-1112 routes execute-bead calls through dispatchViaResolvedConfig. cli/internal/agent/quality_hooks.go defines LintResult, but no cli/internal/agent/lint_hook.go exists to compose a bead-lifecycle lint-mode prompt and parse its structured output.

PROPOSED FIX
  - Add cli/internal/agent/lint_hook.go with a small constructor, e.g. NewPreDispatchLintHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner) func(ctx context.Context, beadID string) (LintResult, error). If a narrower signature fits existing package boundaries better, keep the same explicit inputs: projectRoot, bead lookup, sealed resolved config, optional service, optional AgentRunner.
  - The hook reads the bead by ID and builds a prompt whose first line is MODE: lint, names the bead-lifecycle skill, includes the bead title/type/labels/parent/deps/description/acceptance/custom fields, and instructs JSON-only output matching LintResult.
  - Invoke dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, AgentRunRuntime{Prompt: prompt, WorkDir: projectRoot, PromptSource: "bead-lifecycle-lint"}) or an equivalent package-local wrapper that still uses AgentRunner/ResolvedConfig and not os/exec.
  - Parse Result.Output or Result.CondensedOutput as JSON into LintResult. Return typed/sentinel errors for missing skill/harness, bad JSON, timeout/context cancellation, and empty model output so ddx-d1dae2dd can append warnings and fail open.
  - Keep hook code independent of cli/cmd; CLI wiring belongs in a separate bead so ddx work can reuse this package-level hook later.

NON-SCOPE
  - Execute-loop warn/block policy; covered by ddx-d1dae2dd.
  - Config schema for block threshold beyond whatever ddx-d1dae2dd needs.
  - Triage hook implementation; covered by ddx-4375292b.
  - CLI flags for disabling lint.

PARENT
  ddx-e1a576a7.

DEPS
  No deps. ddx-c8dc1146 is closed and the LintResult/runtime hook contract exists.
    </description>
    <acceptance>
1. cli/internal/agent/lint_hook.go implements NewPreDispatchLintHook or an equivalently named constructor returning func(ctx context.Context, beadID string) (LintResult, error), with explicit injected AgentRunner/service/config inputs and no dependency on cli/cmd.
2. TestLintHook_UsesRunnerLibrary asserts the hook reaches an injected AgentRunner.Run path with AgentRunRuntime/RunArgs-derived prompt content, and a static grep/AST assertion proves lint_hook.go does not import os/exec or shell out to ddx agent run.
3. TestLintHook_PromptIncludesStandaloneBead verifies the prompt starts with MODE: lint, names bead-lifecycle, includes title/type/labels/parent/deps/description/acceptance, and requests JSON fields score, rationale, suggested_fixes, waivers_applied.
4. TestLintHook_BadJSON_ReturnsInfrastructureError proves malformed runner output is surfaced as a typed/sentinel hook infrastructure error for fail-open handling.
5. TestLintHook_Timeout_ReturnsInfrastructureError proves context timeout/cancellation is surfaced as a typed/sentinel infrastructure error.
6. TestLintHook_EmptyOutput_ReturnsInfrastructureError proves empty Result.Output/CondensedOutput does not become a zero-score valid lint result.
7. cd cli &amp;&amp; go test ./internal/agent/... green.
8. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, kind:feature, reliability, bead-quality, adr:023</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T154945-708201c4/manifest.json</file>
    <file>.ddx/executions/20260505T154945-708201c4/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="91e7eb12d61ced7295c6c0780b18d2934866c927">
diff --git a/.ddx/executions/20260505T154945-708201c4/manifest.json b/.ddx/executions/20260505T154945-708201c4/manifest.json
new file mode 100644
index 00000000..dda45d05
--- /dev/null
+++ b/.ddx/executions/20260505T154945-708201c4/manifest.json
@@ -0,0 +1,181 @@
+{
+  "attempt_id": "20260505T154945-708201c4",
+  "bead_id": "ddx-68706ef9",
+  "base_rev": "d90ea96fd28f79833ef2dc72c8690baf14876cfa",
+  "created_at": "2026-05-05T15:49:48.01235499Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-68706ef9",
+    "title": "agent: implement runner-backed bead lifecycle lint hook",
+    "description": "PROBLEM\n  The pre-dispatch lint hook needs a production implementation that invokes the bead-lifecycle skill through the internal runner library, not by spawning ddx agent run as a subprocess.\n\nROOT CAUSE\n  cli/internal/agent/dispatch.go:35-63 centralizes agent invocation in dispatchViaResolvedConfig. cli/internal/agent/execute_bead.go:168 defines the existing AgentRunner injection seam as Run(opts RunArgs) (*Result, error). cli/internal/agent/execute_bead.go:1110-1112 routes execute-bead calls through dispatchViaResolvedConfig. cli/internal/agent/quality_hooks.go defines LintResult, but no cli/internal/agent/lint_hook.go exists to compose a bead-lifecycle lint-mode prompt and parse its structured output.\n\nPROPOSED FIX\n  - Add cli/internal/agent/lint_hook.go with a small constructor, e.g. NewPreDispatchLintHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner) func(ctx context.Context, beadID string) (LintResult, error). If a narrower signature fits existing package boundaries better, keep the same explicit inputs: projectRoot, bead lookup, sealed resolved config, optional service, optional AgentRunner.\n  - The hook reads the bead by ID and builds a prompt whose first line is MODE: lint, names the bead-lifecycle skill, includes the bead title/type/labels/parent/deps/description/acceptance/custom fields, and instructs JSON-only output matching LintResult.\n  - Invoke dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, AgentRunRuntime{Prompt: prompt, WorkDir: projectRoot, PromptSource: \"bead-lifecycle-lint\"}) or an equivalent package-local wrapper that still uses AgentRunner/ResolvedConfig and not os/exec.\n  - Parse Result.Output or Result.CondensedOutput as JSON into LintResult. Return typed/sentinel errors for missing skill/harness, bad JSON, timeout/context cancellation, and empty model output so ddx-d1dae2dd can append warnings and fail open.\n  - Keep hook code independent of cli/cmd; CLI wiring belongs in a separate bead so ddx work can reuse this package-level hook later.\n\nNON-SCOPE\n  - Execute-loop warn/block policy; covered by ddx-d1dae2dd.\n  - Config schema for block threshold beyond whatever ddx-d1dae2dd needs.\n  - Triage hook implementation; covered by ddx-4375292b.\n  - CLI flags for disabling lint.\n\nPARENT\n  ddx-e1a576a7.\n\nDEPS\n  No deps. ddx-c8dc1146 is closed and the LintResult/runtime hook contract exists.",
+    "acceptance": "1. cli/internal/agent/lint_hook.go implements NewPreDispatchLintHook or an equivalently named constructor returning func(ctx context.Context, beadID string) (LintResult, error), with explicit injected AgentRunner/service/config inputs and no dependency on cli/cmd.\n2. TestLintHook_UsesRunnerLibrary asserts the hook reaches an injected AgentRunner.Run path with AgentRunRuntime/RunArgs-derived prompt content, and a static grep/AST assertion proves lint_hook.go does not import os/exec or shell out to ddx agent run.\n3. TestLintHook_PromptIncludesStandaloneBead verifies the prompt starts with MODE: lint, names bead-lifecycle, includes title/type/labels/parent/deps/description/acceptance, and requests JSON fields score, rationale, suggested_fixes, waivers_applied.\n4. TestLintHook_BadJSON_ReturnsInfrastructureError proves malformed runner output is surfaced as a typed/sentinel hook infrastructure error for fail-open handling.\n5. TestLintHook_Timeout_ReturnsInfrastructureError proves context timeout/cancellation is surfaced as a typed/sentinel infrastructure error.\n6. TestLintHook_EmptyOutput_ReturnsInfrastructureError proves empty Result.Output/CondensedOutput does not become a zero-score valid lint result.\n7. cd cli \u0026\u0026 go test ./internal/agent/... green.\n8. lefthook run pre-commit passes.",
+    "parent": "ddx-e1a576a7",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "kind:feature",
+      "reliability",
+      "bead-quality",
+      "adr:023"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T15:49:45Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2464742",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T21:47:59.139652811Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=3e3884202bc2c12628854f5922de2f6ede7c9887\nbase_rev=3e3884202bc2c12628854f5922de2f6ede7c9887\nretry_after=2026-05-05T03:47:59Z",
+          "created_at": "2026-05-04T21:48:00.001591312Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T03:58:01.493092651Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T034823-39dce609\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":9260538,\"output_tokens\":31479,\"total_tokens\":9292017,\"cost_usd\":0,\"duration_ms\":576037,\"exit_code\":0}",
+          "created_at": "2026-05-05T03:58:01.732691553Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=9292017 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T03:58:07.565467619Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=be428fcdfae7099e69b6ed8ae9a76541b80f5966\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T00:03:12-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=14060\noutput_bytes=0\nelapsed_ms=4150",
+          "created_at": "2026-05-05T03:58:12.297234554Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=be428fcdfae7099e69b6ed8ae9a76541b80f5966\nbase_rev=4273cf4b1829544afff1230a6900f07d4a4850b5",
+          "created_at": "2026-05-05T03:58:12.519759478Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T10:22:04.039198992Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T101936-c2e8dad5\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":1379384,\"output_tokens\":7273,\"total_tokens\":1386657,\"cost_usd\":0,\"duration_ms\":145889,\"exit_code\":0}",
+          "created_at": "2026-05-05T10:22:04.267717049Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=1386657 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T10:22:09.833912423Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=f1bd9eca7b45cec622e54ddc5cbd805c306c6694\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T06:27:14-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=15828\noutput_bytes=0\nelapsed_ms=4173",
+          "created_at": "2026-05-05T10:22:14.491021025Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=f1bd9eca7b45cec622e54ddc5cbd805c306c6694\nbase_rev=a6e4b2da3a2d72ffbad1e1f9ef7da929f858e387",
+          "created_at": "2026-05-05T10:22:14.701197306Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T12:39:26.806427586Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T123609-291dbd4c\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":1389143,\"output_tokens\":6368,\"total_tokens\":1395511,\"cost_usd\":0,\"duration_ms\":194577,\"exit_code\":0}",
+          "created_at": "2026-05-05T12:39:27.055936981Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=1395511 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T12:39:34.989451279Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=c621affb2dcbd6b81bd9599cf72b0f2bc2e33302\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T08:44:39-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=18132\noutput_bytes=0\nelapsed_ms=4152",
+          "created_at": "2026-05-05T12:39:39.693198367Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=c621affb2dcbd6b81bd9599cf72b0f2bc2e33302\nbase_rev=ccc2426b66041d9aaed52112a1c3bec9242ce69e",
+          "created_at": "2026-05-05T12:39:39.911258786Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T15:49:45.021870652Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T03:47:59Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T154945-708201c4",
+    "prompt": ".ddx/executions/20260505T154945-708201c4/prompt.md",
+    "manifest": ".ddx/executions/20260505T154945-708201c4/manifest.json",
+    "result": ".ddx/executions/20260505T154945-708201c4/result.json",
+    "checks": ".ddx/executions/20260505T154945-708201c4/checks.json",
+    "usage": ".ddx/executions/20260505T154945-708201c4/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-68706ef9-20260505T154945-708201c4"
+  },
+  "prompt_sha": "3eef12ab688f5559f675d99ae1cd18b9ecf01d600cb68a50d7a28ea127e1bd1a"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T154945-708201c4/result.json b/.ddx/executions/20260505T154945-708201c4/result.json
new file mode 100644
index 00000000..689c129d
--- /dev/null
+++ b/.ddx/executions/20260505T154945-708201c4/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-68706ef9",
+  "attempt_id": "20260505T154945-708201c4",
+  "base_rev": "d90ea96fd28f79833ef2dc72c8690baf14876cfa",
+  "result_rev": "9fb42d2a17ca73643f9a347d9a01750067d4004e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-80c7c9f3",
+  "duration_ms": 97800,
+  "tokens": 867854,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T154945-708201c4",
+  "prompt_file": ".ddx/executions/20260505T154945-708201c4/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T154945-708201c4/manifest.json",
+  "result_file": ".ddx/executions/20260505T154945-708201c4/result.json",
+  "usage_file": ".ddx/executions/20260505T154945-708201c4/usage.json",
+  "started_at": "2026-05-05T15:49:48.012828281Z",
+  "finished_at": "2026-05-05T15:51:25.813733549Z"
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

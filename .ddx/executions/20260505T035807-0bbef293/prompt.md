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
    <file>.ddx/executions/20260505T034823-39dce609/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T034823-39dce609/manifest.json</file>
    <file>.ddx/executions/20260505T034823-39dce609/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="be428fcdfae7099e69b6ed8ae9a76541b80f5966">
diff --git a/.ddx/executions/20260505T034823-39dce609/checks/production-reachability.json b/.ddx/executions/20260505T034823-39dce609/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T034823-39dce609/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T034823-39dce609/manifest.json b/.ddx/executions/20260505T034823-39dce609/manifest.json
new file mode 100644
index 00000000..6b0fd756
--- /dev/null
+++ b/.ddx/executions/20260505T034823-39dce609/manifest.json
@@ -0,0 +1,61 @@
+{
+  "attempt_id": "20260505T034823-39dce609",
+  "bead_id": "ddx-68706ef9",
+  "base_rev": "4273cf4b1829544afff1230a6900f07d4a4850b5",
+  "created_at": "2026-05-05T03:48:25.452074903Z",
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
+      "claimed-at": "2026-05-05T03:48:23Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
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
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T03:48:23.352057843Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T03:47:59Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T034823-39dce609",
+    "prompt": ".ddx/executions/20260505T034823-39dce609/prompt.md",
+    "manifest": ".ddx/executions/20260505T034823-39dce609/manifest.json",
+    "result": ".ddx/executions/20260505T034823-39dce609/result.json",
+    "checks": ".ddx/executions/20260505T034823-39dce609/checks.json",
+    "usage": ".ddx/executions/20260505T034823-39dce609/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-68706ef9-20260505T034823-39dce609"
+  },
+  "prompt_sha": "090ebf827d29b1f8f1da84d4d32d9a37a93cbf62b0a8a81d27a0a8f78ae91e33"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T034823-39dce609/result.json b/.ddx/executions/20260505T034823-39dce609/result.json
new file mode 100644
index 00000000..e41227fd
--- /dev/null
+++ b/.ddx/executions/20260505T034823-39dce609/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-68706ef9",
+  "attempt_id": "20260505T034823-39dce609",
+  "base_rev": "4273cf4b1829544afff1230a6900f07d4a4850b5",
+  "result_rev": "2f48b7bb8e375c707b2a7720ec7afcacc4c8a0c0",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-fd399a30",
+  "duration_ms": 576037,
+  "tokens": 9292017,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T034823-39dce609",
+  "prompt_file": ".ddx/executions/20260505T034823-39dce609/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T034823-39dce609/manifest.json",
+  "result_file": ".ddx/executions/20260505T034823-39dce609/result.json",
+  "usage_file": ".ddx/executions/20260505T034823-39dce609/usage.json",
+  "started_at": "2026-05-05T03:48:25.452391361Z",
+  "finished_at": "2026-05-05T03:58:01.490371155Z"
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

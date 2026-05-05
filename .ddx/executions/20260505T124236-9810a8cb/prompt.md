<bead-review>
  <bead id="ddx-4375292b" iter=1>
    <title>agent: invoke runner-backed post-attempt triage hook</title>
    <description>
PROBLEM
  ddx try needs post-attempt triage after the loop has adjudicated an attempt outcome and before cooldown decisions, so transient failures can be classified without mutating the underlying status. Today the loop records status/detail/rationale but has no triage hook invocation path that can populate OutcomeReason.

ROOT CAUSE
  cli/internal/agent/execute_bead_loop.go:940-1090 contains the outcome adjudication and cooldown branch chain. cli/internal/agent/execute_bead_loop.go:976 and 1084 call shouldSuppressNoProgress before applying no-progress cooldown. cli/internal/agent/execute_bead_loop.go:1099 appends the report to the loop result after cooldown handling. cli/internal/agent/execute_bead_loop.go:30 now exposes ExecuteBeadLoopRuntime.PostAttemptTriageHook and cli/internal/agent/quality_hooks.go defines TriageResult, but no cli/internal/agent/triage_hook.go exists and the loop never calls the hook.

PROPOSED FIX
  - Add cli/internal/agent/triage_hook.go with a constructor, e.g. NewPostAttemptTriageHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner, sessionLogReader SessionLogExcerptReader) func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error). If package boundaries require a different name, preserve the explicit injected inputs.
  - The hook builds a prompt whose first line is MODE: triage, names bead-lifecycle, includes the bead body, ExecuteBeadReport as JSON, the most relevant recent bead events, and a bounded session-log excerpt when report.SessionID is non-empty and a log can be found under the project agent log dir.
  - Invoke dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, AgentRunRuntime{Prompt: prompt, WorkDir: projectRoot, PromptSource: "bead-lifecycle-triage"}) or an equivalent package-local wrapper that still uses AgentRunner/ResolvedConfig and not os/exec.
  - In execute_bead_loop.go, call runtime.PostAttemptTriageHook after report.Status/Detail/Rationale/BaseRev/ResultRev are final for the attempt and before any shouldSuppressNoProgress or SetExecutionCooldown branch. A valid classification sets report.OutcomeReason = TriageResult.Classification and records triage details in the execute-bead event body or a dedicated bead-quality.triage event.
  - On hook failure, fail open: leave OutcomeReason empty and keep legacy cooldown behavior.

NON-SCOPE
  - Defining OutcomeReason and TriageResult; closed in ddx-c8dc1146.
  - CLI wiring for ddx try; covered by sibling/parent wiring work.
  - GraphQL exposure of OutcomeReason.
  - Automatically creating SuggestedFollowupBeads; record/surface only unless a later bead opts in.

PARENT
  ddx-e1a576a7.

DEPS
  No deps. ddx-c8dc1146 is closed and the OutcomeReason/TriageResult contract exists.
    </description>
    <acceptance>
1. cli/internal/agent/triage_hook.go implements NewPostAttemptTriageHook or an equivalently named constructor returning func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error), with explicit injected AgentRunner/service/config inputs and no dependency on cli/cmd.
2. TestTriageHook_UsesRunnerLibrary asserts the hook reaches an injected AgentRunner.Run path and a static grep/AST assertion proves triage_hook.go does not import os/exec or shell out to ddx agent run.
3. TestTriageHook_PromptIncludesOutcomeAndLogExcerpt verifies the prompt starts with MODE: triage, names bead-lifecycle, includes bead content, ExecuteBeadReport JSON, and a bounded session-log excerpt when available.
4. TestLoop_TriageHook_FiresPostOutcome proves PostAttemptTriageHook runs after report status/detail are populated and before no-progress cooldown is applied.
5. TestTriageHook_RecordsButDoesNotMutateOutcome proves valid triage classification populates OutcomeReason and/or a triage event without changing ExecuteBeadReport.Status.
6. TestTriageHook_HookError_FallsThroughToLegacyCooldown proves hook error leaves OutcomeReason empty and preserves existing cooldown behavior.
7. The triage prompt requests structured JSON fields classification, recommended_action, rationale, suggested_amendments, and suggested_followup_beads.
8. WIRED-IN: git grep PostAttemptTriageHook cli/internal/agent shows the execute-loop invocation site, not just the runtime field declaration.
9. cd cli &amp;&amp; go test ./internal/agent/... green.
10. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, kind:feature, reliability, bead-quality, adr:023</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T123940-47727d8d/manifest.json</file>
    <file>.ddx/executions/20260505T123940-47727d8d/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="54680047fa9e15daf4616a28ceef0d73f635a1b7">
diff --git a/.ddx/executions/20260505T123940-47727d8d/manifest.json b/.ddx/executions/20260505T123940-47727d8d/manifest.json
new file mode 100644
index 00000000..b80b11ca
--- /dev/null
+++ b/.ddx/executions/20260505T123940-47727d8d/manifest.json
@@ -0,0 +1,141 @@
+{
+  "attempt_id": "20260505T123940-47727d8d",
+  "bead_id": "ddx-4375292b",
+  "base_rev": "31c92199664a52c631b1efe2128485e5d46f03a2",
+  "created_at": "2026-05-05T12:39:43.601930449Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-4375292b",
+    "title": "agent: invoke runner-backed post-attempt triage hook",
+    "description": "PROBLEM\n  ddx try needs post-attempt triage after the loop has adjudicated an attempt outcome and before cooldown decisions, so transient failures can be classified without mutating the underlying status. Today the loop records status/detail/rationale but has no triage hook invocation path that can populate OutcomeReason.\n\nROOT CAUSE\n  cli/internal/agent/execute_bead_loop.go:940-1090 contains the outcome adjudication and cooldown branch chain. cli/internal/agent/execute_bead_loop.go:976 and 1084 call shouldSuppressNoProgress before applying no-progress cooldown. cli/internal/agent/execute_bead_loop.go:1099 appends the report to the loop result after cooldown handling. cli/internal/agent/execute_bead_loop.go:30 now exposes ExecuteBeadLoopRuntime.PostAttemptTriageHook and cli/internal/agent/quality_hooks.go defines TriageResult, but no cli/internal/agent/triage_hook.go exists and the loop never calls the hook.\n\nPROPOSED FIX\n  - Add cli/internal/agent/triage_hook.go with a constructor, e.g. NewPostAttemptTriageHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner, sessionLogReader SessionLogExcerptReader) func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error). If package boundaries require a different name, preserve the explicit injected inputs.\n  - The hook builds a prompt whose first line is MODE: triage, names bead-lifecycle, includes the bead body, ExecuteBeadReport as JSON, the most relevant recent bead events, and a bounded session-log excerpt when report.SessionID is non-empty and a log can be found under the project agent log dir.\n  - Invoke dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, AgentRunRuntime{Prompt: prompt, WorkDir: projectRoot, PromptSource: \"bead-lifecycle-triage\"}) or an equivalent package-local wrapper that still uses AgentRunner/ResolvedConfig and not os/exec.\n  - In execute_bead_loop.go, call runtime.PostAttemptTriageHook after report.Status/Detail/Rationale/BaseRev/ResultRev are final for the attempt and before any shouldSuppressNoProgress or SetExecutionCooldown branch. A valid classification sets report.OutcomeReason = TriageResult.Classification and records triage details in the execute-bead event body or a dedicated bead-quality.triage event.\n  - On hook failure, fail open: leave OutcomeReason empty and keep legacy cooldown behavior.\n\nNON-SCOPE\n  - Defining OutcomeReason and TriageResult; closed in ddx-c8dc1146.\n  - CLI wiring for ddx try; covered by sibling/parent wiring work.\n  - GraphQL exposure of OutcomeReason.\n  - Automatically creating SuggestedFollowupBeads; record/surface only unless a later bead opts in.\n\nPARENT\n  ddx-e1a576a7.\n\nDEPS\n  No deps. ddx-c8dc1146 is closed and the OutcomeReason/TriageResult contract exists.",
+    "acceptance": "1. cli/internal/agent/triage_hook.go implements NewPostAttemptTriageHook or an equivalently named constructor returning func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error), with explicit injected AgentRunner/service/config inputs and no dependency on cli/cmd.\n2. TestTriageHook_UsesRunnerLibrary asserts the hook reaches an injected AgentRunner.Run path and a static grep/AST assertion proves triage_hook.go does not import os/exec or shell out to ddx agent run.\n3. TestTriageHook_PromptIncludesOutcomeAndLogExcerpt verifies the prompt starts with MODE: triage, names bead-lifecycle, includes bead content, ExecuteBeadReport JSON, and a bounded session-log excerpt when available.\n4. TestLoop_TriageHook_FiresPostOutcome proves PostAttemptTriageHook runs after report status/detail are populated and before no-progress cooldown is applied.\n5. TestTriageHook_RecordsButDoesNotMutateOutcome proves valid triage classification populates OutcomeReason and/or a triage event without changing ExecuteBeadReport.Status.\n6. TestTriageHook_HookError_FallsThroughToLegacyCooldown proves hook error leaves OutcomeReason empty and preserves existing cooldown behavior.\n7. The triage prompt requests structured JSON fields classification, recommended_action, rationale, suggested_amendments, and suggested_followup_beads.\n8. WIRED-IN: git grep PostAttemptTriageHook cli/internal/agent shows the execute-loop invocation site, not just the runtime field declaration.\n9. cd cli \u0026\u0026 go test ./internal/agent/... green.\n10. lefthook run pre-commit passes.",
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
+      "claimed-at": "2026-05-05T12:39:40Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2464742",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T21:48:52.452461738Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=f418810f4e2699af6f6237809e10d19e5e2fc32d\nbase_rev=f418810f4e2699af6f6237809e10d19e5e2fc32d\nretry_after=2026-05-05T03:48:52Z",
+          "created_at": "2026-05-04T21:48:53.219083364Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T04:07:01.591212818Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T035813-0d58a054\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":8817168,\"output_tokens\":48112,\"total_tokens\":8865280,\"cost_usd\":0,\"duration_ms\":525936,\"exit_code\":0}",
+          "created_at": "2026-05-05T04:07:01.839000541Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=8865280 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T04:07:09.347828779Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=2877e4621043d8c9959feea95083152dd55a0c33\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T00:12:13-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=15364\noutput_bytes=0\nelapsed_ms=4154",
+          "created_at": "2026-05-05T04:07:14.044432611Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=2877e4621043d8c9959feea95083152dd55a0c33\nbase_rev=b5110836759a30e62c8fcc4fa218f186a022c624",
+          "created_at": "2026-05-05T04:07:14.276467981Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T10:24:18.01393878Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T102215-1300147f\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":1233146,\"output_tokens\":7388,\"total_tokens\":1240534,\"cost_usd\":0,\"duration_ms\":120638,\"exit_code\":0}",
+          "created_at": "2026-05-05T10:24:18.232617139Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=1240534 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T10:24:23.517573226Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=48651a0d6c838ea221c6a5be3494216d9e004de2\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T06:29:28-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=17131\noutput_bytes=0\nelapsed_ms=4125",
+          "created_at": "2026-05-05T10:24:28.193281489Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=48651a0d6c838ea221c6a5be3494216d9e004de2\nbase_rev=fa0fae43525008a4d223035c81b553ee564f689a",
+          "created_at": "2026-05-05T10:24:28.404500021Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T12:39:40.753446276Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T03:48:52Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T123940-47727d8d",
+    "prompt": ".ddx/executions/20260505T123940-47727d8d/prompt.md",
+    "manifest": ".ddx/executions/20260505T123940-47727d8d/manifest.json",
+    "result": ".ddx/executions/20260505T123940-47727d8d/result.json",
+    "checks": ".ddx/executions/20260505T123940-47727d8d/checks.json",
+    "usage": ".ddx/executions/20260505T123940-47727d8d/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-4375292b-20260505T123940-47727d8d"
+  },
+  "prompt_sha": "58b57de2055573c40c970e4c1981aa3386ebd9e649e53f7305001e3698ebcaa5"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T123940-47727d8d/result.json b/.ddx/executions/20260505T123940-47727d8d/result.json
new file mode 100644
index 00000000..a2f1e28c
--- /dev/null
+++ b/.ddx/executions/20260505T123940-47727d8d/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-4375292b",
+  "attempt_id": "20260505T123940-47727d8d",
+  "base_rev": "31c92199664a52c631b1efe2128485e5d46f03a2",
+  "result_rev": "85878744f0d2f10e05912aba78f7561510fd27af",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-3d213492",
+  "duration_ms": 164784,
+  "tokens": 988600,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T123940-47727d8d",
+  "prompt_file": ".ddx/executions/20260505T123940-47727d8d/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T123940-47727d8d/manifest.json",
+  "result_file": ".ddx/executions/20260505T123940-47727d8d/result.json",
+  "usage_file": ".ddx/executions/20260505T123940-47727d8d/usage.json",
+  "started_at": "2026-05-05T12:39:43.602406865Z",
+  "finished_at": "2026-05-05T12:42:28.387213495Z"
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

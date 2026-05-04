<bead-review>
  <bead id="ddx-c8dc1146" iter=1>
    <title>agent: add quality hook contracts and OutcomeReason carrier</title>
    <description>
PROBLEM
  ddx try needs lint and triage extension points plus an outcome classification carrier before hook implementations can be wired safely. Today ExecuteBeadLoopRuntime exposes PreClaimHook and RoutePreflight only, and ExecuteBeadReport carries Disrupted/DisruptionReason but no sibling OutcomeReason for post-attempt triage.

ROOT CAUSE
  cli/internal/agent/execute_bead_loop.go:24-38 defines ExecuteBeadLoopRuntime with PreClaimHook and RoutePreflight closures but no PreDispatchLintHook or PostAttemptTriageHook slot. cli/internal/agent/execute_bead_loop.go:198-266 defines ExecuteBeadReport with Disrupted and DisruptionReason but no OutcomeReason field. cli/internal/agent/execute_bead_loop.go:1522-1532 suppresses no-progress cooldowns only through Disrupted/BaseRev/ResultRev checks.

PROPOSED FIX
  - Add LintResult, TriageResult, and FollowupBead structs in cli/internal/agent/quality_hooks.go or alongside ExecuteBeadLoopRuntime.
  - Add ExecuteBeadLoopRuntime.PreDispatchLintHook func(ctx context.Context, beadID string) (LintResult, error).
  - Add ExecuteBeadLoopRuntime.PostAttemptTriageHook func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error).
  - Add ExecuteBeadReport.OutcomeReason string beside Disrupted/DisruptionReason, preserving the existing Disrupted bool and JSON shape.
  - Include outcome_reason in executeBeadLoopEvent body serialization when populated.
  - Extend shouldSuppressNoProgress so OutcomeReason values transport, quota, routing, timeout, and merge_conflict also bypass no-progress cooldown; keep the existing Disrupted short-circuit unchanged.

NON-SCOPE
  - Invoking either hook in the loop; covered by sibling child beads.
  - Runner-backed bead-lifecycle skill calls; covered by lint/triage implementation child beads.
  - cli/cmd/try.go wiring; covered by the CLI wiring child bead.
  - GraphQL exposure of OutcomeReason.

PARENT
  ddx-e1a576a7.

DEPS
  ddx-e1a576a7 per decomposition edge requested by the parent attempt instructions.
    </description>
    <acceptance>
1. cli/internal/agent/execute_bead_loop.go or cli/internal/agent/quality_hooks.go defines LintResult, TriageResult, and FollowupBead with the fields documented in parent ddx-e1a576a7.
2. ExecuteBeadLoopRuntime exposes PreDispatchLintHook and PostAttemptTriageHook with signatures func(ctx context.Context, beadID string) (LintResult, error) and func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error).
3. ExecuteBeadReport includes OutcomeReason string with json tag outcome_reason,omitempty beside the existing Disrupted and DisruptionReason fields; Disrupted remains unchanged.
4. TestReport_OutcomeReason_Persists_BesideDisrupted verifies executeBeadLoopEvent records outcome_reason while preserving disrupted/disruption_reason behavior.
5. TestSuppressNoProgress_HonorsTransientReasons verifies OutcomeReason values transport, quota, routing, timeout, and merge_conflict bypass no-progress cooldown while non-transient reasons retain existing BaseRev/ResultRev behavior.
6. cd cli &amp;&amp; go test ./internal/agent/... green.
7. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, kind:feature, reliability, bead-quality, adr:023</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260504T163018-c3f70b35/checks/production-reachability.json</file>
    <file>.ddx/executions/20260504T163018-c3f70b35/manifest.json</file>
    <file>.ddx/executions/20260504T163018-c3f70b35/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="cf6a7d448857ea72eb74aebee83243124fef3912">
diff --git a/.ddx/executions/20260504T163018-c3f70b35/checks/production-reachability.json b/.ddx/executions/20260504T163018-c3f70b35/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260504T163018-c3f70b35/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T163018-c3f70b35/manifest.json b/.ddx/executions/20260504T163018-c3f70b35/manifest.json
new file mode 100644
index 00000000..97e8ffc6
--- /dev/null
+++ b/.ddx/executions/20260504T163018-c3f70b35/manifest.json
@@ -0,0 +1,67 @@
+{
+  "attempt_id": "20260504T163018-c3f70b35",
+  "bead_id": "ddx-c8dc1146",
+  "base_rev": "f22b59cb065f5acc599e997a8d50c522b3aad8a9",
+  "created_at": "2026-05-04T16:33:21.832419164Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-c8dc1146",
+    "title": "agent: add quality hook contracts and OutcomeReason carrier",
+    "description": "PROBLEM\n  ddx try needs lint and triage extension points plus an outcome classification carrier before hook implementations can be wired safely. Today ExecuteBeadLoopRuntime exposes PreClaimHook and RoutePreflight only, and ExecuteBeadReport carries Disrupted/DisruptionReason but no sibling OutcomeReason for post-attempt triage.\n\nROOT CAUSE\n  cli/internal/agent/execute_bead_loop.go:24-38 defines ExecuteBeadLoopRuntime with PreClaimHook and RoutePreflight closures but no PreDispatchLintHook or PostAttemptTriageHook slot. cli/internal/agent/execute_bead_loop.go:198-266 defines ExecuteBeadReport with Disrupted and DisruptionReason but no OutcomeReason field. cli/internal/agent/execute_bead_loop.go:1522-1532 suppresses no-progress cooldowns only through Disrupted/BaseRev/ResultRev checks.\n\nPROPOSED FIX\n  - Add LintResult, TriageResult, and FollowupBead structs in cli/internal/agent/quality_hooks.go or alongside ExecuteBeadLoopRuntime.\n  - Add ExecuteBeadLoopRuntime.PreDispatchLintHook func(ctx context.Context, beadID string) (LintResult, error).\n  - Add ExecuteBeadLoopRuntime.PostAttemptTriageHook func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error).\n  - Add ExecuteBeadReport.OutcomeReason string beside Disrupted/DisruptionReason, preserving the existing Disrupted bool and JSON shape.\n  - Include outcome_reason in executeBeadLoopEvent body serialization when populated.\n  - Extend shouldSuppressNoProgress so OutcomeReason values transport, quota, routing, timeout, and merge_conflict also bypass no-progress cooldown; keep the existing Disrupted short-circuit unchanged.\n\nNON-SCOPE\n  - Invoking either hook in the loop; covered by sibling child beads.\n  - Runner-backed bead-lifecycle skill calls; covered by lint/triage implementation child beads.\n  - cli/cmd/try.go wiring; covered by the CLI wiring child bead.\n  - GraphQL exposure of OutcomeReason.\n\nPARENT\n  ddx-e1a576a7.\n\nDEPS\n  ddx-e1a576a7 per decomposition edge requested by the parent attempt instructions.",
+    "acceptance": "1. cli/internal/agent/execute_bead_loop.go or cli/internal/agent/quality_hooks.go defines LintResult, TriageResult, and FollowupBead with the fields documented in parent ddx-e1a576a7.\n2. ExecuteBeadLoopRuntime exposes PreDispatchLintHook and PostAttemptTriageHook with signatures func(ctx context.Context, beadID string) (LintResult, error) and func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error).\n3. ExecuteBeadReport includes OutcomeReason string with json tag outcome_reason,omitempty beside the existing Disrupted and DisruptionReason fields; Disrupted remains unchanged.\n4. TestReport_OutcomeReason_Persists_BesideDisrupted verifies executeBeadLoopEvent records outcome_reason while preserving disrupted/disruption_reason behavior.\n5. TestSuppressNoProgress_HonorsTransientReasons verifies OutcomeReason values transport, quota, routing, timeout, and merge_conflict bypass no-progress cooldown while non-transient reasons retain existing BaseRev/ResultRev behavior.\n6. cd cli \u0026\u0026 go test ./internal/agent/... green.\n7. lefthook run pre-commit passes.",
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
+      "claimed-at": "2026-05-04T16:30:10Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3615727",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"fallback_chain\":[],\"requested_harness\":\"codex\"}",
+          "created_at": "2026-05-04T16:27:22.477313557Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260504T155405-2105ce6f\",\"harness\":\"codex\",\"input_tokens\":2569032,\"output_tokens\":9096,\"total_tokens\":2578128,\"cost_usd\":0,\"duration_ms\":1952407,\"exit_code\":0}",
+          "created_at": "2026-05-04T16:27:27.121313734Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2578128"
+        },
+        {
+          "actor": "erik",
+          "body": "fast-forwarding refs/heads/main to merge commit 481537fae82f14468023b08a180b628c63c365f9: git update-ref refs/heads/main: fatal: update_ref failed for ref 'refs/heads/main': cannot lock ref 'refs/heads/main': is at f5ebaa533ebec91e7523c4bdaa9a4d87d2021bf9 but expected 28d8351d24f8a396bbd28242016a231161c28ec8: exit status 128",
+          "created_at": "2026-05-04T16:28:05.409525409Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-04T16:30:10.662523828Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260504T163018-c3f70b35",
+    "prompt": ".ddx/executions/20260504T163018-c3f70b35/prompt.md",
+    "manifest": ".ddx/executions/20260504T163018-c3f70b35/manifest.json",
+    "result": ".ddx/executions/20260504T163018-c3f70b35/result.json",
+    "checks": ".ddx/executions/20260504T163018-c3f70b35/checks.json",
+    "usage": ".ddx/executions/20260504T163018-c3f70b35/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-c8dc1146-20260504T163018-c3f70b35"
+  },
+  "prompt_sha": "e9bcce5b246f3eef525241b76da3a13360f46355bcf4af89006cb4f659a99fa5"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T163018-c3f70b35/result.json b/.ddx/executions/20260504T163018-c3f70b35/result.json
new file mode 100644
index 00000000..c582c4ea
--- /dev/null
+++ b/.ddx/executions/20260504T163018-c3f70b35/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-c8dc1146",
+  "attempt_id": "20260504T163018-c3f70b35",
+  "base_rev": "f22b59cb065f5acc599e997a8d50c522b3aad8a9",
+  "result_rev": "4f63a237572f67b073959a5fe28e665a9f5ce669",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-3a5fda87",
+  "duration_ms": 6019046,
+  "tokens": 8807697,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260504T163018-c3f70b35",
+  "prompt_file": ".ddx/executions/20260504T163018-c3f70b35/prompt.md",
+  "manifest_file": ".ddx/executions/20260504T163018-c3f70b35/manifest.json",
+  "result_file": ".ddx/executions/20260504T163018-c3f70b35/result.json",
+  "usage_file": ".ddx/executions/20260504T163018-c3f70b35/usage.json",
+  "started_at": "2026-05-04T16:33:22.81726271Z",
+  "finished_at": "2026-05-04T18:13:41.863985185Z"
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

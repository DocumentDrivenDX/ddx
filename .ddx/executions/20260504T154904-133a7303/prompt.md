<bead-review>
  <bead id="ddx-e1a576a7" iter=1>
    <title>ddx try: PreDispatchLintHook + PostAttemptTriageHook (warn-only) + OutcomeReason field beside Disrupted</title>
    <description>
PROBLEM
Per epic ddx-a9d130d0 + ADR-023 + sub-skill bead-lifecycle/: ddx try needs to invoke the sub-skill at two points (pre-dispatch lint, post-attempt triage) and act on the structured output. Today ddx try has hook precedents (PreClaimHook, RoutePreflight) but no LLM-invoking hook + no outcome classification carrier.

ROOT CAUSE
- cli/internal/agent/execute_bead_loop.go:24-38 defines ExecuteBeadLoopRuntime with PreClaimHook + RoutePreflight closures; no slot for lint or triage hooks.
- cli/internal/agent/execute_bead_loop.go:657 RoutePreflight runs between pick and claim — exactly where lint should fire.
- cli/internal/agent/execute_bead.go:45 ExecuteBeadResult already has reason + failure_mode; ExecuteBeadReport (loop event payload) does NOT — only status/detail/rationale at execute_bead_loop.go:1317.
- shouldSuppressNoProgress at execute_bead_loop.go:1522 already respects Disrupted bool. Codex amendment: KEEP Disrupted; ADD OutcomeReason beside it (do not replace).
- ddx try at cli/cmd/try.go:327 wires PreClaimHook today; sibling fields will go through the same Runtime construction.

PROPOSED FIX
1. ExecuteBeadLoopRuntime additions (cli/internal/agent/execute_bead_loop.go ~line 30):
   PreDispatchLintHook func(ctx context.Context, beadID string) (LintResult, error)
   PostAttemptTriageHook func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error)
   LintResult struct { Score int; Rationale string; SuggestedFixes []string; WaiversApplied []string }
   TriageResult struct { Classification string; RecommendedAction string; Rationale string; SuggestedAmendments string; SuggestedFollowupBeads []FollowupBead }

2. PreDispatchLintHook fires AFTER nextCandidate, BEFORE Claim (sibling to RoutePreflight at execute_bead_loop.go:662).
   - Hook implementation in cli/internal/agent/lint_hook.go: composes runner library to invoke bead-lifecycle skill in lint mode (per FEAT-001/FEAT-010 layer-1 invocation; NOT subprocess via ddx agent run per codex).
   - WARN-ONLY behavior (default): record lint result as bead event (kind=bead-quality.lint); proceed with claim regardless of score.
   - BLOCK mode (opt-in via config flag in .ddx/config.yaml): refuse claim when score &lt; threshold; emit operator-readable explanation + cite bead-lifecycle SKILL.md guidance for fixes.
   - FAIL-OPEN per RELIABILITY-PRINCIPLES P1: infrastructure failure (skill missing, harness unavailable, bad JSON, timeout) → emit warning event + proceed. Only valid low-score response blocks (and only in BLOCK mode).

3. PostAttemptTriageHook fires AFTER outcome adjudication, BEFORE cooldown decision (read execute_bead_loop.go ~line 880-1100 for the outcome class chain).
   - Hook implementation in cli/internal/agent/triage_hook.go: composes runner library to invoke bead-lifecycle skill in triage mode with (bead + outcome event + session log excerpt).
   - Triage classification populates new ExecuteBeadReport.OutcomeReason field (string).
   - shouldSuppressNoProgress at execute_bead_loop.go:1522 extends to also short-circuit when OutcomeReason ∈ {transport, quota, routing, timeout, merge_conflict} (transient classes per RELIABILITY-PRINCIPLES P6). Disrupted check stays in place beside it.
   - FAIL-OPEN: triage hook failure → no classification → fall through to existing cooldown logic (today's behavior, no regression).

4. cli/cmd/try.go integration: wire PreDispatchLintHook + PostAttemptTriageHook into ExecuteBeadLoopRuntime construction at try.go:327. Operator can disable via --no-lint / --no-triage flags (defer if not needed; defaults are warn-only so explicit disable is rare).

5. Hook invocation cost: lint runs once per dispatch attempt (cheap sonnet call); triage runs once per non-clean outcome. Both invocations cite the bead-lifecycle skill by name in the prompt; harness auto-discovers from filesystem (no special skill-loading code needed).

NON-SCOPE
- ddx work integration (this bead is ddx try only; ddx work integration is a separate follow-up if desired)
- BLOCK mode threshold tuning (operator chooses post-baseline; no code change)
- ddx work plan quality column (deferred per epic non-scope)
- GraphQL state.OutcomeReason exposure (defer; today's GraphQL maps status only at state_graphql.go:493; if exposure needed, separate bead per codex)
- --force --reason CLI flag (add only if BLOCK mode rollout reveals need; defer)
    </description>
    <acceptance>
1. ExecuteBeadLoopRuntime (cli/internal/agent/execute_bead_loop.go ~line 30) gains PreDispatchLintHook + PostAttemptTriageHook closure fields with the documented signatures. LintResult + TriageResult + FollowupBead types defined in same file or sibling cli/internal/agent/quality_hooks.go.

2. PreDispatchLintHook fires at the documented insertion point (after nextCandidate, before Claim, sibling to RoutePreflight at execute_bead_loop.go:662). Verified by integration test TestLoop_LintHook_FiresPreClaim.

3. PostAttemptTriageHook fires after outcome adjudication, before cooldown. Verified by TestLoop_TriageHook_FiresPostOutcome.

4. cli/internal/agent/lint_hook.go + triage_hook.go implement the hooks via runner library (NOT subprocess to ddx agent run). Verified by TestLintHook_UsesRunnerLibrary asserting no os/exec call.

5. WARN-ONLY default: tests TestLintHook_LowScore_WarnDoesNotBlock + TestTriageHook_RecordsButDoesNotMutateOutcome.

6. BLOCK mode opt-in: config flag bead-quality.lint.block_threshold_score. Test TestLintHook_BlockMode_RefusesDispatchOnLowScore.

7. FAIL-OPEN: tests TestLintHook_SkillMissing_ProceedsWithWarning + TestLintHook_BadJSON_ProceedsWithWarning + TestLintHook_Timeout_ProceedsWithWarning + TestTriageHook_HookError_FallsThroughToLegacyCooldown.

8. ExecuteBeadReport gains OutcomeReason string field (cli/internal/agent/execute_bead_loop.go ~line 1317 event serialization). Disrupted bool unchanged. Test TestReport_OutcomeReason_Persists_BesideDisrupted.

9. shouldSuppressNoProgress (execute_bead_loop.go:1522) extends to short-circuit on OutcomeReason ∈ {transport, quota, routing, timeout, merge_conflict}. Test TestSuppressNoProgress_HonorsTransientReasons.

10. cli/cmd/try.go wires both hooks into Runtime construction at try.go:327. Test TestTry_HooksWired.

11. WIRED-IN: 'git grep PreDispatchLintHook cli/cmd/' shows the wire site; 'git grep PostAttemptTriageHook cli/cmd/' shows same.

12. cd cli &amp;&amp; go test ./internal/agent/... ./cmd/... green; lefthook run pre-commit passes.

13. Manual verification: 'ddx try &lt;retrofit-bead-id&gt;' (after epic child #2 ships sub-skill) emits bead-quality.lint event in the bead's event stream; 'ddx try &lt;bead-with-bad-AC&gt;' under BLOCK mode refuses dispatch with operator-readable message.

14. Conventional commit ending [&lt;this-bead-id&gt;]. Stage cli/internal/agent/execute_bead_loop.go + cli/internal/agent/lint_hook.go + cli/internal/agent/triage_hook.go + cli/internal/agent/quality_hooks.go + cli/cmd/try.go + cli/internal/agent/*_test.go + cli/cmd/try_test.go only.
    </acceptance>
    <notes>
Decomposed on 2026-05-04 because ddx-e1a576a7 exceeded the execute-bead Step 0 size threshold (14 ACs spanning runtime contracts, hook implementation, loop policy, config behavior, CLI wiring, and tests). Child beads:
- ddx-c8dc1146: shared quality hook contracts and ExecuteBeadReport.OutcomeReason carrier.
- ddx-d1dae2dd: execute-loop pre-dispatch lint invocation, warn/block/fail-open policy, and bead-quality.lint event behavior.
- ddx-68706ef9: runner-backed bead-lifecycle lint hook implementation.
- ddx-4375292b: runner-backed post-attempt triage hook, loop invocation, and OutcomeReason population.
- ddx-9c81b20f: cli/cmd/try.go runtime wiring for both hooks.
    </notes>
    <labels>phase:2, area:agent, area:cli, kind:feature, reliability, bead-quality, adr:023</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260504T153241-f5ad53de/manifest.json</file>
    <file>.ddx/executions/20260504T153241-f5ad53de/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="2ca54415fb0cd2968d326adf3e68411c79f699e5">
diff --git a/.ddx/executions/20260504T153241-f5ad53de/manifest.json b/.ddx/executions/20260504T153241-f5ad53de/manifest.json
new file mode 100644
index 00000000..dd86a472
--- /dev/null
+++ b/.ddx/executions/20260504T153241-f5ad53de/manifest.json
@@ -0,0 +1,42 @@
+{
+  "attempt_id": "20260504T153241-f5ad53de",
+  "bead_id": "ddx-e1a576a7",
+  "base_rev": "0a5a66c869ff03bf8fb05b1a8b8892bdbbac0734",
+  "created_at": "2026-05-04T15:33:35.364369414Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-e1a576a7",
+    "title": "ddx try: PreDispatchLintHook + PostAttemptTriageHook (warn-only) + OutcomeReason field beside Disrupted",
+    "description": "PROBLEM\nPer epic ddx-a9d130d0 + ADR-023 + sub-skill bead-lifecycle/: ddx try needs to invoke the sub-skill at two points (pre-dispatch lint, post-attempt triage) and act on the structured output. Today ddx try has hook precedents (PreClaimHook, RoutePreflight) but no LLM-invoking hook + no outcome classification carrier.\n\nROOT CAUSE\n- cli/internal/agent/execute_bead_loop.go:24-38 defines ExecuteBeadLoopRuntime with PreClaimHook + RoutePreflight closures; no slot for lint or triage hooks.\n- cli/internal/agent/execute_bead_loop.go:657 RoutePreflight runs between pick and claim — exactly where lint should fire.\n- cli/internal/agent/execute_bead.go:45 ExecuteBeadResult already has reason + failure_mode; ExecuteBeadReport (loop event payload) does NOT — only status/detail/rationale at execute_bead_loop.go:1317.\n- shouldSuppressNoProgress at execute_bead_loop.go:1522 already respects Disrupted bool. Codex amendment: KEEP Disrupted; ADD OutcomeReason beside it (do not replace).\n- ddx try at cli/cmd/try.go:327 wires PreClaimHook today; sibling fields will go through the same Runtime construction.\n\nPROPOSED FIX\n1. ExecuteBeadLoopRuntime additions (cli/internal/agent/execute_bead_loop.go ~line 30):\n   PreDispatchLintHook func(ctx context.Context, beadID string) (LintResult, error)\n   PostAttemptTriageHook func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error)\n   LintResult struct { Score int; Rationale string; SuggestedFixes []string; WaiversApplied []string }\n   TriageResult struct { Classification string; RecommendedAction string; Rationale string; SuggestedAmendments string; SuggestedFollowupBeads []FollowupBead }\n\n2. PreDispatchLintHook fires AFTER nextCandidate, BEFORE Claim (sibling to RoutePreflight at execute_bead_loop.go:662).\n   - Hook implementation in cli/internal/agent/lint_hook.go: composes runner library to invoke bead-lifecycle skill in lint mode (per FEAT-001/FEAT-010 layer-1 invocation; NOT subprocess via ddx agent run per codex).\n   - WARN-ONLY behavior (default): record lint result as bead event (kind=bead-quality.lint); proceed with claim regardless of score.\n   - BLOCK mode (opt-in via config flag in .ddx/config.yaml): refuse claim when score \u003c threshold; emit operator-readable explanation + cite bead-lifecycle SKILL.md guidance for fixes.\n   - FAIL-OPEN per RELIABILITY-PRINCIPLES P1: infrastructure failure (skill missing, harness unavailable, bad JSON, timeout) → emit warning event + proceed. Only valid low-score response blocks (and only in BLOCK mode).\n\n3. PostAttemptTriageHook fires AFTER outcome adjudication, BEFORE cooldown decision (read execute_bead_loop.go ~line 880-1100 for the outcome class chain).\n   - Hook implementation in cli/internal/agent/triage_hook.go: composes runner library to invoke bead-lifecycle skill in triage mode with (bead + outcome event + session log excerpt).\n   - Triage classification populates new ExecuteBeadReport.OutcomeReason field (string).\n   - shouldSuppressNoProgress at execute_bead_loop.go:1522 extends to also short-circuit when OutcomeReason ∈ {transport, quota, routing, timeout, merge_conflict} (transient classes per RELIABILITY-PRINCIPLES P6). Disrupted check stays in place beside it.\n   - FAIL-OPEN: triage hook failure → no classification → fall through to existing cooldown logic (today's behavior, no regression).\n\n4. cli/cmd/try.go integration: wire PreDispatchLintHook + PostAttemptTriageHook into ExecuteBeadLoopRuntime construction at try.go:327. Operator can disable via --no-lint / --no-triage flags (defer if not needed; defaults are warn-only so explicit disable is rare).\n\n5. Hook invocation cost: lint runs once per dispatch attempt (cheap sonnet call); triage runs once per non-clean outcome. Both invocations cite the bead-lifecycle skill by name in the prompt; harness auto-discovers from filesystem (no special skill-loading code needed).\n\nNON-SCOPE\n- ddx work integration (this bead is ddx try only; ddx work integration is a separate follow-up if desired)\n- BLOCK mode threshold tuning (operator chooses post-baseline; no code change)\n- ddx work plan quality column (deferred per epic non-scope)\n- GraphQL state.OutcomeReason exposure (defer; today's GraphQL maps status only at state_graphql.go:493; if exposure needed, separate bead per codex)\n- --force --reason CLI flag (add only if BLOCK mode rollout reveals need; defer)",
+    "acceptance": "1. ExecuteBeadLoopRuntime (cli/internal/agent/execute_bead_loop.go ~line 30) gains PreDispatchLintHook + PostAttemptTriageHook closure fields with the documented signatures. LintResult + TriageResult + FollowupBead types defined in same file or sibling cli/internal/agent/quality_hooks.go.\n\n2. PreDispatchLintHook fires at the documented insertion point (after nextCandidate, before Claim, sibling to RoutePreflight at execute_bead_loop.go:662). Verified by integration test TestLoop_LintHook_FiresPreClaim.\n\n3. PostAttemptTriageHook fires after outcome adjudication, before cooldown. Verified by TestLoop_TriageHook_FiresPostOutcome.\n\n4. cli/internal/agent/lint_hook.go + triage_hook.go implement the hooks via runner library (NOT subprocess to ddx agent run). Verified by TestLintHook_UsesRunnerLibrary asserting no os/exec call.\n\n5. WARN-ONLY default: tests TestLintHook_LowScore_WarnDoesNotBlock + TestTriageHook_RecordsButDoesNotMutateOutcome.\n\n6. BLOCK mode opt-in: config flag bead-quality.lint.block_threshold_score. Test TestLintHook_BlockMode_RefusesDispatchOnLowScore.\n\n7. FAIL-OPEN: tests TestLintHook_SkillMissing_ProceedsWithWarning + TestLintHook_BadJSON_ProceedsWithWarning + TestLintHook_Timeout_ProceedsWithWarning + TestTriageHook_HookError_FallsThroughToLegacyCooldown.\n\n8. ExecuteBeadReport gains OutcomeReason string field (cli/internal/agent/execute_bead_loop.go ~line 1317 event serialization). Disrupted bool unchanged. Test TestReport_OutcomeReason_Persists_BesideDisrupted.\n\n9. shouldSuppressNoProgress (execute_bead_loop.go:1522) extends to short-circuit on OutcomeReason ∈ {transport, quota, routing, timeout, merge_conflict}. Test TestSuppressNoProgress_HonorsTransientReasons.\n\n10. cli/cmd/try.go wires both hooks into Runtime construction at try.go:327. Test TestTry_HooksWired.\n\n11. WIRED-IN: 'git grep PreDispatchLintHook cli/cmd/' shows the wire site; 'git grep PostAttemptTriageHook cli/cmd/' shows same.\n\n12. cd cli \u0026\u0026 go test ./internal/agent/... ./cmd/... green; lefthook run pre-commit passes.\n\n13. Manual verification: 'ddx try \u003cretrofit-bead-id\u003e' (after epic child #2 ships sub-skill) emits bead-quality.lint event in the bead's event stream; 'ddx try \u003cbead-with-bad-AC\u003e' under BLOCK mode refuses dispatch with operator-readable message.\n\n14. Conventional commit ending [\u003cthis-bead-id\u003e]. Stage cli/internal/agent/execute_bead_loop.go + cli/internal/agent/lint_hook.go + cli/internal/agent/triage_hook.go + cli/internal/agent/quality_hooks.go + cli/cmd/try.go + cli/internal/agent/*_test.go + cli/cmd/try_test.go only.",
+    "parent": "ddx-a9d130d0",
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
+      "claimed-at": "2026-05-04T15:32:39Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3565930",
+      "execute-loop-heartbeat-at": "2026-05-04T15:32:39.379244703Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260504T153241-f5ad53de",
+    "prompt": ".ddx/executions/20260504T153241-f5ad53de/prompt.md",
+    "manifest": ".ddx/executions/20260504T153241-f5ad53de/manifest.json",
+    "result": ".ddx/executions/20260504T153241-f5ad53de/result.json",
+    "checks": ".ddx/executions/20260504T153241-f5ad53de/checks.json",
+    "usage": ".ddx/executions/20260504T153241-f5ad53de/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-e1a576a7-20260504T153241-f5ad53de"
+  },
+  "prompt_sha": "69372b82351859df5ef15e85609ed1f65a63911614ae6bf0034f749de2d24b91"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T153241-f5ad53de/result.json b/.ddx/executions/20260504T153241-f5ad53de/result.json
new file mode 100644
index 00000000..de10261e
--- /dev/null
+++ b/.ddx/executions/20260504T153241-f5ad53de/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-e1a576a7",
+  "attempt_id": "20260504T153241-f5ad53de",
+  "base_rev": "0a5a66c869ff03bf8fb05b1a8b8892bdbbac0734",
+  "result_rev": "6b74a1e21b2ed3ddff40e63458f1aec7fa2d304f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-37b4c6eb",
+  "duration_ms": 819878,
+  "tokens": 9413378,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260504T153241-f5ad53de",
+  "prompt_file": ".ddx/executions/20260504T153241-f5ad53de/prompt.md",
+  "manifest_file": ".ddx/executions/20260504T153241-f5ad53de/manifest.json",
+  "result_file": ".ddx/executions/20260504T153241-f5ad53de/result.json",
+  "usage_file": ".ddx/executions/20260504T153241-f5ad53de/usage.json",
+  "started_at": "2026-05-04T15:33:35.364910831Z",
+  "finished_at": "2026-05-04T15:47:15.243206604Z"
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

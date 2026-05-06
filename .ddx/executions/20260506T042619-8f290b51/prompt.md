<bead-review>
  <bead id="ddx-f58797f1" iter=1>
    <title>work: wire strong-model intake decomposition into ddx work</title>
    <description>
PROBLEM
Plain `ddx work` must run decomposition automatically and it must use a strong model/power floor. Today the typed intake hook exists in agent code, but the production `ddx work` loop does not wire PreClaimIntakeHook or PostAttemptTriageHook, and the existing bead-lifecycle hook helpers dispatch through the resolved worker config without a MinPowerOverride. That means decomposition either does not run at all under `ddx work`, or would inherit the worker's cheap/standard route instead of using a strong splitter.

ROOT CAUSE WITH FILE:LINE
- cli/cmd/agent_cmd.go:1919 constructs ExecuteBeadLoopRuntime for the CLI work/execute-loop path but sets only PreClaimHook; it does not set PreClaimIntakeHook, PreDispatchLintHook, or PostAttemptTriageHook.
- cli/internal/server/workers.go:908 constructs ExecuteBeadLoopRuntime for server-managed workers but also omits PreClaimIntakeHook, PreDispatchLintHook, and PostAttemptTriageHook.
- cli/cmd/try.go:335 wires lint and triage hooks for `ddx try`, but that does not cover `ddx work`.
- cli/internal/agent/lint_hook.go:115 and cli/internal/agent/triage_hook.go:111 dispatch via AgentRunRuntime without MinPowerOverride, so quality/decomposition prompts inherit rcfg.MinPower().
- cli/internal/agent/types.go:90 and cli/internal/agent/service_run.go:180 show MinPowerOverride is available and reaches ServiceExecuteRequest.MinPower when set.

PROPOSED FIX
- Wire production `ddx work` / execute-loop paths to construct and pass PreClaimIntakeHook, PreDispatchLintHook, and PostAttemptTriageHook by default, matching the reliable factory policy.
- Add a strong-model/power policy for decomposition and other splitter decisions: the decomposition runner must set MinPowerOverride to the configured strong splitter floor, defaulting to the smart/top-power tier floor when no project override is set.
- Preserve passthrough constraints: do not mutate operator-supplied harness/provider/model, but if the strong MinPower cannot be satisfied under those constraints, record agent_power_unsatisfied and block for operator rather than falling back to weak decomposition.
- Server-managed workers and CLI `ddx work` must behave the same way.

NON-SCOPE
- Implementing the child-bead AC-map splitter itself; ddx-008d288f owns that behavior.
- Review adversarial pre-close gate wiring.
    </description>
    <acceptance>
1. CLI `ddx work` / execute-loop runtime wiring in cli/cmd/agent_cmd.go passes PreClaimIntakeHook, PreDispatchLintHook, and PostAttemptTriageHook by default.
2. Server-managed worker runtime wiring in cli/internal/server/workers.go passes the same hooks by default.
3. Decomposition/splitter dispatch uses AgentRunRuntime.MinPowerOverride with a strong splitter floor; TestDecompositionHook_UsesStrongMinPower verifies ServiceExecuteRequest.MinPower is above the normal cheap/default worker floor.
4. TestDDxWork_WiresPreClaimIntakeHook verifies plain `ddx work` triggers intake before Claim without requiring `ddx try` or a special flag.
5. TestServerWorker_WiresPreClaimIntakeHook verifies server-managed workers trigger the same intake path.
6. TestDecompositionHook_PreservesPassthroughConstraints verifies harness/provider/model passthrough values are not mutated when raising MinPower.
7. TestDecompositionHook_StrongPowerUnsatisfiedBlocks verifies unsatisfied strong power under hard passthrough constraints records agent_power_unsatisfied and does not run weak decomposition.
8. cd cli &amp;&amp; go test ./cmd/... ./internal/server/... ./internal/agent/... -run "TestDDxWork_WiresPreClaimIntakeHook|TestServerWorker_WiresPreClaimIntakeHook|TestDecompositionHook" -count=1 passes.
9. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, story:10, area:agent, area:cli, area:server, kind:feature, reliability, adr:023, adr:024</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T035119-9715e509/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T035119-9715e509/manifest.json</file>
    <file>.ddx/executions/20260506T035119-9715e509/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="748e596ded90c5264e3144947b6e707e394e0080">
<untrusted-data>
diff --git a/.ddx/executions/20260506T035119-9715e509/checks/production-reachability.json b/.ddx/executions/20260506T035119-9715e509/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T035119-9715e509/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T035119-9715e509/manifest.json b/.ddx/executions/20260506T035119-9715e509/manifest.json
new file mode 100644
index 00000000..c07c415e
--- /dev/null
+++ b/.ddx/executions/20260506T035119-9715e509/manifest.json
@@ -0,0 +1,44 @@
+{
+  "attempt_id": "20260506T035119-9715e509",
+  "bead_id": "ddx-f58797f1",
+  "base_rev": "ee9f902e7e27c63a14d9bbc47cd80d998b1e9387",
+  "created_at": "2026-05-06T03:51:22.260219882Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-f58797f1",
+    "title": "work: wire strong-model intake decomposition into ddx work",
+    "description": "PROBLEM\nPlain `ddx work` must run decomposition automatically and it must use a strong model/power floor. Today the typed intake hook exists in agent code, but the production `ddx work` loop does not wire PreClaimIntakeHook or PostAttemptTriageHook, and the existing bead-lifecycle hook helpers dispatch through the resolved worker config without a MinPowerOverride. That means decomposition either does not run at all under `ddx work`, or would inherit the worker's cheap/standard route instead of using a strong splitter.\n\nROOT CAUSE WITH FILE:LINE\n- cli/cmd/agent_cmd.go:1919 constructs ExecuteBeadLoopRuntime for the CLI work/execute-loop path but sets only PreClaimHook; it does not set PreClaimIntakeHook, PreDispatchLintHook, or PostAttemptTriageHook.\n- cli/internal/server/workers.go:908 constructs ExecuteBeadLoopRuntime for server-managed workers but also omits PreClaimIntakeHook, PreDispatchLintHook, and PostAttemptTriageHook.\n- cli/cmd/try.go:335 wires lint and triage hooks for `ddx try`, but that does not cover `ddx work`.\n- cli/internal/agent/lint_hook.go:115 and cli/internal/agent/triage_hook.go:111 dispatch via AgentRunRuntime without MinPowerOverride, so quality/decomposition prompts inherit rcfg.MinPower().\n- cli/internal/agent/types.go:90 and cli/internal/agent/service_run.go:180 show MinPowerOverride is available and reaches ServiceExecuteRequest.MinPower when set.\n\nPROPOSED FIX\n- Wire production `ddx work` / execute-loop paths to construct and pass PreClaimIntakeHook, PreDispatchLintHook, and PostAttemptTriageHook by default, matching the reliable factory policy.\n- Add a strong-model/power policy for decomposition and other splitter decisions: the decomposition runner must set MinPowerOverride to the configured strong splitter floor, defaulting to the smart/top-power tier floor when no project override is set.\n- Preserve passthrough constraints: do not mutate operator-supplied harness/provider/model, but if the strong MinPower cannot be satisfied under those constraints, record agent_power_unsatisfied and block for operator rather than falling back to weak decomposition.\n- Server-managed workers and CLI `ddx work` must behave the same way.\n\nNON-SCOPE\n- Implementing the child-bead AC-map splitter itself; ddx-008d288f owns that behavior.\n- Review adversarial pre-close gate wiring.",
+    "acceptance": "1. CLI `ddx work` / execute-loop runtime wiring in cli/cmd/agent_cmd.go passes PreClaimIntakeHook, PreDispatchLintHook, and PostAttemptTriageHook by default.\n2. Server-managed worker runtime wiring in cli/internal/server/workers.go passes the same hooks by default.\n3. Decomposition/splitter dispatch uses AgentRunRuntime.MinPowerOverride with a strong splitter floor; TestDecompositionHook_UsesStrongMinPower verifies ServiceExecuteRequest.MinPower is above the normal cheap/default worker floor.\n4. TestDDxWork_WiresPreClaimIntakeHook verifies plain `ddx work` triggers intake before Claim without requiring `ddx try` or a special flag.\n5. TestServerWorker_WiresPreClaimIntakeHook verifies server-managed workers trigger the same intake path.\n6. TestDecompositionHook_PreservesPassthroughConstraints verifies harness/provider/model passthrough values are not mutated when raising MinPower.\n7. TestDecompositionHook_StrongPowerUnsatisfiedBlocks verifies unsatisfied strong power under hard passthrough constraints records agent_power_unsatisfied and does not run weak decomposition.\n8. cd cli \u0026\u0026 go test ./cmd/... ./internal/server/... ./internal/agent/... -run \"TestDDxWork_WiresPreClaimIntakeHook|TestServerWorker_WiresPreClaimIntakeHook|TestDecompositionHook\" -count=1 passes.\n9. lefthook run pre-commit passes.",
+    "parent": "ddx-a9d130d0",
+    "labels": [
+      "phase:2",
+      "story:10",
+      "area:agent",
+      "area:cli",
+      "area:server",
+      "kind:feature",
+      "reliability",
+      "adr:023",
+      "adr:024"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T03:51:19Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T03:51:19.575267133Z",
+      "spec_id": "ADR-023"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T035119-9715e509",
+    "prompt": ".ddx/executions/20260506T035119-9715e509/prompt.md",
+    "manifest": ".ddx/executions/20260506T035119-9715e509/manifest.json",
+    "result": ".ddx/executions/20260506T035119-9715e509/result.json",
+    "checks": ".ddx/executions/20260506T035119-9715e509/checks.json",
+    "usage": ".ddx/executions/20260506T035119-9715e509/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-f58797f1-20260506T035119-9715e509"
+  },
+  "prompt_sha": "973d025296bdcbb9a66088f4660a9b1cdcd459c3f464ba59f4f635edbb267c47"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T035119-9715e509/result.json b/.ddx/executions/20260506T035119-9715e509/result.json
new file mode 100644
index 00000000..b897a2ec
--- /dev/null
+++ b/.ddx/executions/20260506T035119-9715e509/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-f58797f1",
+  "attempt_id": "20260506T035119-9715e509",
+  "base_rev": "ee9f902e7e27c63a14d9bbc47cd80d998b1e9387",
+  "result_rev": "565f04575dbb1ef2706bc2b9cd159fd2b25d3bb9",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-b036b4cf",
+  "duration_ms": 2089031,
+  "tokens": 34310698,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T035119-9715e509",
+  "prompt_file": ".ddx/executions/20260506T035119-9715e509/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T035119-9715e509/manifest.json",
+  "result_file": ".ddx/executions/20260506T035119-9715e509/result.json",
+  "usage_file": ".ddx/executions/20260506T035119-9715e509/usage.json",
+  "started_at": "2026-05-06T03:51:22.260623423Z",
+  "finished_at": "2026-05-06T04:26:11.292256144Z"
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

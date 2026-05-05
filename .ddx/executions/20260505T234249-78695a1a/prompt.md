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
    <file>.ddx/executions/20260505T234013-2c206bcf/manifest.json</file>
    <file>.ddx/executions/20260505T234013-2c206bcf/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="af16757c77cf1a9c02d8f8487a795817395681f3">
<untrusted-data>
diff --git a/.ddx/executions/20260505T234013-2c206bcf/manifest.json b/.ddx/executions/20260505T234013-2c206bcf/manifest.json
new file mode 100644
index 00000000..268908d0
--- /dev/null
+++ b/.ddx/executions/20260505T234013-2c206bcf/manifest.json
@@ -0,0 +1,41 @@
+{
+  "attempt_id": "20260505T234013-2c206bcf",
+  "bead_id": "ddx-9c81b20f",
+  "base_rev": "38f3c3837c4678b7ea76f122711dd78b87e7e518",
+  "created_at": "2026-05-05T23:40:16.475979871Z",
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
+      "claimed-at": "2026-05-05T23:40:13Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "427120",
+      "execute-loop-heartbeat-at": "2026-05-05T23:40:13.527893509Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T234013-2c206bcf",
+    "prompt": ".ddx/executions/20260505T234013-2c206bcf/prompt.md",
+    "manifest": ".ddx/executions/20260505T234013-2c206bcf/manifest.json",
+    "result": ".ddx/executions/20260505T234013-2c206bcf/result.json",
+    "checks": ".ddx/executions/20260505T234013-2c206bcf/checks.json",
+    "usage": ".ddx/executions/20260505T234013-2c206bcf/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9c81b20f-20260505T234013-2c206bcf"
+  },
+  "prompt_sha": "3ad7dc4de0130dede199a8c9694e77daf98f2493b879fe01a4507b60827b82dd"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T234013-2c206bcf/result.json b/.ddx/executions/20260505T234013-2c206bcf/result.json
new file mode 100644
index 00000000..0dea5a48
--- /dev/null
+++ b/.ddx/executions/20260505T234013-2c206bcf/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-9c81b20f",
+  "attempt_id": "20260505T234013-2c206bcf",
+  "base_rev": "38f3c3837c4678b7ea76f122711dd78b87e7e518",
+  "result_rev": "5f4c1ae09053488b4e132125728c72be97a70f4f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-dea6359d",
+  "duration_ms": 140709,
+  "tokens": 890785,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T234013-2c206bcf",
+  "prompt_file": ".ddx/executions/20260505T234013-2c206bcf/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T234013-2c206bcf/manifest.json",
+  "result_file": ".ddx/executions/20260505T234013-2c206bcf/result.json",
+  "usage_file": ".ddx/executions/20260505T234013-2c206bcf/usage.json",
+  "started_at": "2026-05-05T23:40:16.47638212Z",
+  "finished_at": "2026-05-05T23:42:37.186159614Z"
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

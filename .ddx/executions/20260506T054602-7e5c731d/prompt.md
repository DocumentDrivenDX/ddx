<bead-review>
  <bead id="ddx-07206d47" iter=1>
    <title>intake: safely rewrite actionable beads before claim</title>
    <description>
PROBLEM
Once typed intake exists, safe bead improvement needs its own narrow implementation slice. Without this, the system can identify a fixable bead but cannot improve it before an implementer claims it.

ROOT CAUSE WITH FILE:LINE
- cli/internal/agent/execute_bead_loop.go:680 currently records lint results but does not mutate bead description/acceptance before claim.
- cli/internal/agent/lint_hook.go:110 builds a lint prompt, not a safe rewrite contract with intent-preserving constraints.

PROPOSED FIX
- Handle actionable_but_rewritten by applying only safe, intent-preserving description/AC updates before Claim.
- Record a triage-rewritten/intake-rewritten event with before/after metadata and rationale.
- Reject unsafe rewrites that invent behavior, choose product semantics, expand scope, or remove acceptance criteria; route those to ambiguous_needs_human.

NON-SCOPE
- Decomposition/child creation.
- Depth cap handling.
    </description>
    <acceptance>
1. TestIntake_ActionableButRewritten_UpdatesBeforeClaim verifies description/AC updates land before Claim and implementation sees the revised bead.
2. TestIntake_RewriteRecordsEvent verifies the event includes rationale and changed fields without dumping unsafe full prompt text.
3. TestIntake_UnsafeRewriteBlocksForHuman verifies behavior-inventing or AC-dropping rewrites become ambiguous_needs_human and do not claim.
4. Existing lint-hook tests remain green.
5. cd cli &amp;&amp; go test ./internal/agent/... -run "TestIntake_.*Rewrite|TestLintHook" -count=1 passes.
6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, story:10, area:agent, kind:feature, reliability, adr:023, adr:024, from:ddx-f3bbcfce</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T053904-b30fc35d/checks/production-reachability.json</file>
    <file>.ddx/executions/20260506T053904-b30fc35d/manifest.json</file>
    <file>.ddx/executions/20260506T053904-b30fc35d/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3e2607b91c9f5d8c9e153e4735c928c1c4ce5d4b">
<untrusted-data>
diff --git a/.ddx/executions/20260506T053904-b30fc35d/checks/production-reachability.json b/.ddx/executions/20260506T053904-b30fc35d/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260506T053904-b30fc35d/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T053904-b30fc35d/manifest.json b/.ddx/executions/20260506T053904-b30fc35d/manifest.json
new file mode 100644
index 00000000..2adfa881
--- /dev/null
+++ b/.ddx/executions/20260506T053904-b30fc35d/manifest.json
@@ -0,0 +1,43 @@
+{
+  "attempt_id": "20260506T053904-b30fc35d",
+  "bead_id": "ddx-07206d47",
+  "base_rev": "6249fec84d260a5f02c27461a242534f1afd5b99",
+  "created_at": "2026-05-06T05:39:06.868308103Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-07206d47",
+    "title": "intake: safely rewrite actionable beads before claim",
+    "description": "PROBLEM\nOnce typed intake exists, safe bead improvement needs its own narrow implementation slice. Without this, the system can identify a fixable bead but cannot improve it before an implementer claims it.\n\nROOT CAUSE WITH FILE:LINE\n- cli/internal/agent/execute_bead_loop.go:680 currently records lint results but does not mutate bead description/acceptance before claim.\n- cli/internal/agent/lint_hook.go:110 builds a lint prompt, not a safe rewrite contract with intent-preserving constraints.\n\nPROPOSED FIX\n- Handle actionable_but_rewritten by applying only safe, intent-preserving description/AC updates before Claim.\n- Record a triage-rewritten/intake-rewritten event with before/after metadata and rationale.\n- Reject unsafe rewrites that invent behavior, choose product semantics, expand scope, or remove acceptance criteria; route those to ambiguous_needs_human.\n\nNON-SCOPE\n- Decomposition/child creation.\n- Depth cap handling.",
+    "acceptance": "1. TestIntake_ActionableButRewritten_UpdatesBeforeClaim verifies description/AC updates land before Claim and implementation sees the revised bead.\n2. TestIntake_RewriteRecordsEvent verifies the event includes rationale and changed fields without dumping unsafe full prompt text.\n3. TestIntake_UnsafeRewriteBlocksForHuman verifies behavior-inventing or AC-dropping rewrites become ambiguous_needs_human and do not claim.\n4. Existing lint-hook tests remain green.\n5. cd cli \u0026\u0026 go test ./internal/agent/... -run \"TestIntake_.*Rewrite|TestLintHook\" -count=1 passes.\n6. lefthook run pre-commit passes.",
+    "parent": "ddx-a9d130d0",
+    "labels": [
+      "phase:2",
+      "story:10",
+      "area:agent",
+      "kind:feature",
+      "reliability",
+      "adr:023",
+      "adr:024",
+      "from:ddx-f3bbcfce"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T05:39:03Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "execute-loop-heartbeat-at": "2026-05-06T05:39:03.974401394Z",
+      "spec_id": "ADR-023"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260506T053904-b30fc35d",
+    "prompt": ".ddx/executions/20260506T053904-b30fc35d/prompt.md",
+    "manifest": ".ddx/executions/20260506T053904-b30fc35d/manifest.json",
+    "result": ".ddx/executions/20260506T053904-b30fc35d/result.json",
+    "checks": ".ddx/executions/20260506T053904-b30fc35d/checks.json",
+    "usage": ".ddx/executions/20260506T053904-b30fc35d/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-07206d47-20260506T053904-b30fc35d"
+  },
+  "prompt_sha": "a20e8d17e0845ae394b204dfbe865af031408d0640ddaf873c8909845360037c"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T053904-b30fc35d/result.json b/.ddx/executions/20260506T053904-b30fc35d/result.json
new file mode 100644
index 00000000..db1aafbe
--- /dev/null
+++ b/.ddx/executions/20260506T053904-b30fc35d/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-07206d47",
+  "attempt_id": "20260506T053904-b30fc35d",
+  "base_rev": "6249fec84d260a5f02c27461a242534f1afd5b99",
+  "result_rev": "b81a6f83ccee035ad8b7e827961ce0bfd8edb368",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-549c8802",
+  "duration_ms": 406554,
+  "tokens": 7431608,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T053904-b30fc35d",
+  "prompt_file": ".ddx/executions/20260506T053904-b30fc35d/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T053904-b30fc35d/manifest.json",
+  "result_file": ".ddx/executions/20260506T053904-b30fc35d/result.json",
+  "usage_file": ".ddx/executions/20260506T053904-b30fc35d/usage.json",
+  "started_at": "2026-05-06T05:39:06.868658811Z",
+  "finished_at": "2026-05-06T05:45:53.423241699Z"
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

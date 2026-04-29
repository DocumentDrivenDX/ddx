<bead-review>
  <bead id="ddx-fba752b9" iter=1>
    <title>execute-loop: epics that decline with 'decompose me' should mark themselves blocked, not rely on ad-hoc cooldowns</title>
    <description>
Today, when execute-bead returns no_changes with rationale = 'this epic is too big, split it into sub-beads', the loop sets a short execute-loop-retry-after (a few hours) and keeps re-attempting the epic forever. The bead surface gives operators no good knob: 'unset retry-after' lets it re-burn, 'set retry-after = 30 days' is arbitrary, and there is no signal that the bead is structurally not a single-pass deliverable.

Observed in this session on ddx-fdd3ea36 (the routing point-release epic): two consecutive drain attempts produced identical no_changes recommendations to split into 9 sub-beads. The agent burned ~$1 of claude tokens on the second attempt re-deriving the same conclusion.

What is needed:

1. A first-class outcome from execute-bead: 'declined: needs decomposition' (distinct from no_changes/execution_failed). Carries the recommendation in a structured field, not buried in the no-changes-rationale string.

2. When the loop sees 'declined: needs decomposition', it should auto-set the bead to status=blocked (or a new status like needs-decomposition), record the recommended sub-beads as a bead-comment / event, and STOP re-attempting until a human or helix-evolve splits it.

3. Or: detect the loop — if the same bead has produced the same no_changes rationale N times in a row (e.g. 2), auto-mark it blocked with reason 'agent declined repeatedly; needs decomposition'.

4. Make 'execute-loop-retry-after' inspectable / settable via a first-class CLI flag rather than --set / --unset on a magic key. Document the field. The 'unset retry-after' shortcut should NOT exist — operators should clear it via 'ddx bead clear-cooldown' or similar.
    </description>
    <acceptance>
1. New ExecuteBeadStatus value or extra field captures 'declined: needs decomposition' as a structured outcome (not regex on rationale). 2. Loop policy: a bead that has produced 'declined: needs decomposition' is auto-set to status=blocked (or equivalent terminal-for-loop status) and is no longer in the dep-ready execution queue. 3. The recommended sub-beads from the decomposition rationale are captured as a structured event on the bead (kind:decomposition-recommendation), not just inline text. 4. CLI: a first-class command for inspecting and clearing execute-loop cooldowns (e.g. ddx bead cooldown show / clear). The current --set/--unset workflow becomes a power-user fallback. 5. Regression test: a fake executor that returns 'declined: needs decomposition' is invoked via the loop; assert (a) bead transitions to blocked after the first declination, (b) no second attempt happens, (c) decomposition recommendation is on the bead as a structured event. 6. Migration: documented in DDx CHANGELOG; existing 'execute-loop-retry-after' key continues to work as a power-user override but is deprecated for the decomposition case.
    </acceptance>
    <labels>execute-loop, beads, quality-of-life</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T003306-8f4ac0c1/manifest.json</file>
    <file>.ddx/executions/20260429T003306-8f4ac0c1/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1275d8417a3fce2493c9432d1a3b82e0f326d924">
diff --git a/.ddx/executions/20260429T003306-8f4ac0c1/manifest.json b/.ddx/executions/20260429T003306-8f4ac0c1/manifest.json
new file mode 100644
index 00000000..09b809de
--- /dev/null
+++ b/.ddx/executions/20260429T003306-8f4ac0c1/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260429T003306-8f4ac0c1",
+  "bead_id": "ddx-fba752b9",
+  "base_rev": "10a9df1094c6aca169d6c4c9e687b8eba310b48b",
+  "created_at": "2026-04-29T00:33:07.046752903Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-fba752b9",
+    "title": "execute-loop: epics that decline with 'decompose me' should mark themselves blocked, not rely on ad-hoc cooldowns",
+    "description": "Today, when execute-bead returns no_changes with rationale = 'this epic is too big, split it into sub-beads', the loop sets a short execute-loop-retry-after (a few hours) and keeps re-attempting the epic forever. The bead surface gives operators no good knob: 'unset retry-after' lets it re-burn, 'set retry-after = 30 days' is arbitrary, and there is no signal that the bead is structurally not a single-pass deliverable.\n\nObserved in this session on ddx-fdd3ea36 (the routing point-release epic): two consecutive drain attempts produced identical no_changes recommendations to split into 9 sub-beads. The agent burned ~$1 of claude tokens on the second attempt re-deriving the same conclusion.\n\nWhat is needed:\n\n1. A first-class outcome from execute-bead: 'declined: needs decomposition' (distinct from no_changes/execution_failed). Carries the recommendation in a structured field, not buried in the no-changes-rationale string.\n\n2. When the loop sees 'declined: needs decomposition', it should auto-set the bead to status=blocked (or a new status like needs-decomposition), record the recommended sub-beads as a bead-comment / event, and STOP re-attempting until a human or helix-evolve splits it.\n\n3. Or: detect the loop — if the same bead has produced the same no_changes rationale N times in a row (e.g. 2), auto-mark it blocked with reason 'agent declined repeatedly; needs decomposition'.\n\n4. Make 'execute-loop-retry-after' inspectable / settable via a first-class CLI flag rather than --set / --unset on a magic key. Document the field. The 'unset retry-after' shortcut should NOT exist — operators should clear it via 'ddx bead clear-cooldown' or similar.",
+    "acceptance": "1. New ExecuteBeadStatus value or extra field captures 'declined: needs decomposition' as a structured outcome (not regex on rationale). 2. Loop policy: a bead that has produced 'declined: needs decomposition' is auto-set to status=blocked (or equivalent terminal-for-loop status) and is no longer in the dep-ready execution queue. 3. The recommended sub-beads from the decomposition rationale are captured as a structured event on the bead (kind:decomposition-recommendation), not just inline text. 4. CLI: a first-class command for inspecting and clearing execute-loop cooldowns (e.g. ddx bead cooldown show / clear). The current --set/--unset workflow becomes a power-user fallback. 5. Regression test: a fake executor that returns 'declined: needs decomposition' is invoked via the loop; assert (a) bead transitions to blocked after the first declination, (b) no second attempt happens, (c) decomposition recommendation is on the bead as a structured event. 6. Migration: documented in DDx CHANGELOG; existing 'execute-loop-retry-after' key continues to work as a power-user override but is deprecated for the decomposition case.",
+    "labels": [
+      "execute-loop",
+      "beads",
+      "quality-of-life"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T00:33:06Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "56807",
+      "execute-loop-heartbeat-at": "2026-04-29T00:33:06.51930776Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T003306-8f4ac0c1",
+    "prompt": ".ddx/executions/20260429T003306-8f4ac0c1/prompt.md",
+    "manifest": ".ddx/executions/20260429T003306-8f4ac0c1/manifest.json",
+    "result": ".ddx/executions/20260429T003306-8f4ac0c1/result.json",
+    "checks": ".ddx/executions/20260429T003306-8f4ac0c1/checks.json",
+    "usage": ".ddx/executions/20260429T003306-8f4ac0c1/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-fba752b9-20260429T003306-8f4ac0c1"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T003306-8f4ac0c1/result.json b/.ddx/executions/20260429T003306-8f4ac0c1/result.json
new file mode 100644
index 00000000..1b981826
--- /dev/null
+++ b/.ddx/executions/20260429T003306-8f4ac0c1/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-fba752b9",
+  "attempt_id": "20260429T003306-8f4ac0c1",
+  "base_rev": "10a9df1094c6aca169d6c4c9e687b8eba310b48b",
+  "result_rev": "118dfebc579196452b215925eab49e804fabac2a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-095f4f57",
+  "duration_ms": 644056,
+  "tokens": 19617,
+  "cost_usd": 4.08637675,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T003306-8f4ac0c1",
+  "prompt_file": ".ddx/executions/20260429T003306-8f4ac0c1/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T003306-8f4ac0c1/manifest.json",
+  "result_file": ".ddx/executions/20260429T003306-8f4ac0c1/result.json",
+  "usage_file": ".ddx/executions/20260429T003306-8f4ac0c1/usage.json",
+  "started_at": "2026-04-29T00:33:07.047023652Z",
+  "finished_at": "2026-04-29T00:43:51.103569624Z"
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

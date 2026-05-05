<bead-review>
  <bead id="ddx-a9d130d0" iter=1>
    <title>EPIC: bead-lifecycle quality framework — make P7 mechanically true (lint + triage)</title>
    <description>
PROBLEM
Per RELIABILITY-PRINCIPLES bead ddx-06b77652 P7 ('bead = sufficient sub-agent prompt'), bead authoring quality is a dominant factor in execution success. Audit-1 (commit 86848d21) covered 20 beads; audit-2 (commit b2eb77d0) covered 108 beads; 88 of 108 scored ≤5 against the rubric. Evidence bead ddx-f339c399 documents one observed no_changes outcome (ddx-29058e2a, 2026-05-04) traced to bead-quality gap.

ROOT CAUSE
Today nothing mechanically prevents authoring a bead that's insufficient as a sub-agent prompt OR catches a non-clean execution outcome before it triggers cooldown. The 9 fail-prone layers in ddx try / ddx work (per RELIABILITY-PRINCIPLES) are mostly fail-closed; bead authoring quality has no observability or enforcement layer at all.

PROPOSED FRAMEWORK
A SKILL-based bead-lifecycle quality framework, layered onto the existing ddx skill (matching the bead-breakdown/ sub-skill pattern at .agents/skills/ddx/bead-breakdown/SKILL.md). Three modes within ONE sub-skill: lint (pre-dispatch), triage (post-attempt outcome classification), refine (suggest amendments). ddx try integrates via small Go hooks that compose the existing layer-1 runner library to invoke the skill.

NO new top-level skills. NO new ddx CLI verbs. NO LLM judgment baked into Go code.

CHILD BEADS (4 sequential phases)
1. ADR-023 + FEAT amendments (governance + policy)
2. Sub-skill ships to all 5 paths + curated examples
3. ddx try hook integration (warn-only mode) + OutcomeReason field
4. Migration: close stale dups + retrofit 40 HIGH beads (during warn-only; collects baseline)

After child #4 confirms baseline acceptable false-positive rate, BLOCK mode is enabled via config (no separate bead).

NON-SCOPE
- ddx work plan quality column (defer; informational, not blocking)
- ddx bead refine standalone command (defer; refine mode in skill is invocable via ddx run)
- Cross-project skill packaging (DDx-only first; package later per FEAT-015 project-local model)
- in_progress-as-eligible bug (separate concern; queue correctness, not bead quality)

GOVERNANCE
- ADR-023 at docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md (codex correction: not 03-decide/)
- FEAT-004 amend: bead validation hooks + waiver storage + tracker metadata (FEAT-004:150 already owns bead validation hooks)
- FEAT-010 amend: ddx try / ddx work hook ordering + retry classification + outcome records
- FEAT-011 amend: nested workflow-skill packaging (FEAT-011:180 stale — says DDx ships exactly one skill, but bead-breakdown/ exists)
- FEAT-001 amend ONLY if --force --reason CLI flag added (defer to child #3 if needed)
    </description>
    <acceptance>
1. All 4 child beads closed.
2. ADR-023 status = Accepted.
3. FEAT-004, FEAT-010, FEAT-011 amendments committed.
4. Sub-skill .agents/skills/ddx/bead-lifecycle/SKILL.md exists in all 5 shipped paths (codex: skills/ddx/, cli/internal/skills/ddx/, .agents/skills/ddx/, .claude/skills/ddx/, .ddx/skills/ddx/).
5. ddx try hooks fire in WARN-ONLY mode by default; BLOCK mode opt-in via config.
6. Baseline lint score distribution recorded post-migration; threshold for BLOCK mode chosen empirically.
7. Closed via final operator review confirming framework is operational.
    </acceptance>
    <notes>
decomposed into ddx-9210f95a, ddx-922b5b1f, ddx-e1a576a7, ddx-1107036a; open child ddx-e1a576a7 blocks completion
    </notes>
    <labels>phase:2, area:beads, area:agent, kind:platform, reliability, bead-quality, adr:023</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T041251-8d825867/manifest.json</file>
    <file>.ddx/executions/20260505T041251-8d825867/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1d02be0d7e302c682313331d51fadf4c3bee2e10">
diff --git a/.ddx/executions/20260505T041251-8d825867/manifest.json b/.ddx/executions/20260505T041251-8d825867/manifest.json
new file mode 100644
index 00000000..e8ef2f19
--- /dev/null
+++ b/.ddx/executions/20260505T041251-8d825867/manifest.json
@@ -0,0 +1,62 @@
+{
+  "attempt_id": "20260505T041251-8d825867",
+  "bead_id": "ddx-a9d130d0",
+  "base_rev": "40bad79b318e87464161768dc1baea8fba2bf794",
+  "created_at": "2026-05-05T04:12:53.513780801Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-a9d130d0",
+    "title": "EPIC: bead-lifecycle quality framework — make P7 mechanically true (lint + triage)",
+    "description": "PROBLEM\nPer RELIABILITY-PRINCIPLES bead ddx-06b77652 P7 ('bead = sufficient sub-agent prompt'), bead authoring quality is a dominant factor in execution success. Audit-1 (commit 86848d21) covered 20 beads; audit-2 (commit b2eb77d0) covered 108 beads; 88 of 108 scored ≤5 against the rubric. Evidence bead ddx-f339c399 documents one observed no_changes outcome (ddx-29058e2a, 2026-05-04) traced to bead-quality gap.\n\nROOT CAUSE\nToday nothing mechanically prevents authoring a bead that's insufficient as a sub-agent prompt OR catches a non-clean execution outcome before it triggers cooldown. The 9 fail-prone layers in ddx try / ddx work (per RELIABILITY-PRINCIPLES) are mostly fail-closed; bead authoring quality has no observability or enforcement layer at all.\n\nPROPOSED FRAMEWORK\nA SKILL-based bead-lifecycle quality framework, layered onto the existing ddx skill (matching the bead-breakdown/ sub-skill pattern at .agents/skills/ddx/bead-breakdown/SKILL.md). Three modes within ONE sub-skill: lint (pre-dispatch), triage (post-attempt outcome classification), refine (suggest amendments). ddx try integrates via small Go hooks that compose the existing layer-1 runner library to invoke the skill.\n\nNO new top-level skills. NO new ddx CLI verbs. NO LLM judgment baked into Go code.\n\nCHILD BEADS (4 sequential phases)\n1. ADR-023 + FEAT amendments (governance + policy)\n2. Sub-skill ships to all 5 paths + curated examples\n3. ddx try hook integration (warn-only mode) + OutcomeReason field\n4. Migration: close stale dups + retrofit 40 HIGH beads (during warn-only; collects baseline)\n\nAfter child #4 confirms baseline acceptable false-positive rate, BLOCK mode is enabled via config (no separate bead).\n\nNON-SCOPE\n- ddx work plan quality column (defer; informational, not blocking)\n- ddx bead refine standalone command (defer; refine mode in skill is invocable via ddx run)\n- Cross-project skill packaging (DDx-only first; package later per FEAT-015 project-local model)\n- in_progress-as-eligible bug (separate concern; queue correctness, not bead quality)\n\nGOVERNANCE\n- ADR-023 at docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md (codex correction: not 03-decide/)\n- FEAT-004 amend: bead validation hooks + waiver storage + tracker metadata (FEAT-004:150 already owns bead validation hooks)\n- FEAT-010 amend: ddx try / ddx work hook ordering + retry classification + outcome records\n- FEAT-011 amend: nested workflow-skill packaging (FEAT-011:180 stale — says DDx ships exactly one skill, but bead-breakdown/ exists)\n- FEAT-001 amend ONLY if --force --reason CLI flag added (defer to child #3 if needed)",
+    "acceptance": "1. All 4 child beads closed.\n2. ADR-023 status = Accepted.\n3. FEAT-004, FEAT-010, FEAT-011 amendments committed.\n4. Sub-skill .agents/skills/ddx/bead-lifecycle/SKILL.md exists in all 5 shipped paths (codex: skills/ddx/, cli/internal/skills/ddx/, .agents/skills/ddx/, .claude/skills/ddx/, .ddx/skills/ddx/).\n5. ddx try hooks fire in WARN-ONLY mode by default; BLOCK mode opt-in via config.\n6. Baseline lint score distribution recorded post-migration; threshold for BLOCK mode chosen empirically.\n7. Closed via final operator review confirming framework is operational.",
+    "parent": "ddx-e34994e2",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:agent",
+      "kind:platform",
+      "reliability",
+      "bead-quality",
+      "adr:023"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T04:12:51Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "19662",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-04T21:51:30.735535478Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "exit status 1\nresult_rev=d3d0589c2e69010a1493d8884e8357c06b3ef371\nbase_rev=d3d0589c2e69010a1493d8884e8357c06b3ef371\nretry_after=2026-05-05T03:51:31Z",
+          "created_at": "2026-05-04T21:51:31.450935691Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T04:12:51.238375625Z",
+      "execute-loop-last-detail": "exit status 1",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": "2026-05-05T03:51:31Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T041251-8d825867",
+    "prompt": ".ddx/executions/20260505T041251-8d825867/prompt.md",
+    "manifest": ".ddx/executions/20260505T041251-8d825867/manifest.json",
+    "result": ".ddx/executions/20260505T041251-8d825867/result.json",
+    "checks": ".ddx/executions/20260505T041251-8d825867/checks.json",
+    "usage": ".ddx/executions/20260505T041251-8d825867/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-a9d130d0-20260505T041251-8d825867"
+  },
+  "prompt_sha": "bf2f0ad5095373fc1133c4b74e3b15f7a3bccc3012ec1062e7feeb4b400b8ea1"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T041251-8d825867/result.json b/.ddx/executions/20260505T041251-8d825867/result.json
new file mode 100644
index 00000000..6edfce43
--- /dev/null
+++ b/.ddx/executions/20260505T041251-8d825867/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-a9d130d0",
+  "attempt_id": "20260505T041251-8d825867",
+  "base_rev": "40bad79b318e87464161768dc1baea8fba2bf794",
+  "result_rev": "be2ba3b7471cd3b8fd1a00527b3ab956c66b41d9",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-5a8966a2",
+  "duration_ms": 103782,
+  "tokens": 1558061,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T041251-8d825867",
+  "prompt_file": ".ddx/executions/20260505T041251-8d825867/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T041251-8d825867/manifest.json",
+  "result_file": ".ddx/executions/20260505T041251-8d825867/result.json",
+  "usage_file": ".ddx/executions/20260505T041251-8d825867/usage.json",
+  "started_at": "2026-05-05T04:12:53.514186759Z",
+  "finished_at": "2026-05-05T04:14:37.296378337Z"
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

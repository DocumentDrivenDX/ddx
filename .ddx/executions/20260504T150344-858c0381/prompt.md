<bead-review>
  <bead id="ddx-9210f95a" iter=1>
    <title>ADR-023 + FEAT-004/010/011 amendments: bead-lifecycle quality enforcement policy</title>
    <description>
PROBLEM
Per epic ddx-a9d130d0: bead-lifecycle quality framework needs an ADR documenting the policy + amendments to existing FEATs to formally adopt the lint/triage hooks and the sub-skill packaging change.

ROOT CAUSE
Today no policy artifact exists for bead-quality enforcement. RELIABILITY-PRINCIPLES bead ddx-06b77652 documents P7 as a principle but does not specify the enforcement mechanism, waiver model, staged rollout, or recovery UX. Existing FEAT docs are stale on relevant points:
- FEAT-011 at docs/helix/01-frame/features/FEAT-011-skills.md:180 says DDx ships exactly one skill, but .agents/skills/ddx/bead-breakdown/SKILL.md already demonstrates nested sub-skill packaging.
- FEAT-004 at docs/helix/01-frame/features/FEAT-004-beads.md:150 owns bead validation hooks but doesn't describe authoring-quality lint or post-attempt triage.
- FEAT-010 at docs/helix/01-frame/features/FEAT-010-executions.md:21 documents the layer-1/2/3 invocation model but doesn't spec hook insertion points for ddx try / ddx work.

PROPOSED FIX
1. Author ADR-023 at docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md covering:
   - Policy: lint pre-dispatch + triage post-attempt; both invoke the same bead-lifecycle sub-skill
   - Staged rollout: WARN-ONLY default → BLOCK opt-in after baseline confirms
   - Waiver model: rubric-level by bead-type (doc/epic/deletion skip certain criteria, per docs/helix/06-iterate/bead-authoring-template.md:219); per-bead lint-waiver:&lt;criterion&gt; label for rare exceptions; --force --reason records an event (does not silently bypass)
   - Fail-open hook behavior (P1 from RELIABILITY-PRINCIPLES): infrastructure failure proceeds; only valid low lint score blocks (in BLOCK mode)
   - Operator recovery UX: blocked dispatch prints missing fields + suggested ddx bead update commands

2. Amend FEAT-004 (beads): add §"Authoring quality lint" describing the rubric, sub-skill location, waiver storage (label-based), and tracker metadata (no new schema fields; lint output ephemeral in evidence dir).

3. Amend FEAT-010 (executions): add §"Quality hooks" describing PreDispatchLintHook + PostAttemptTriageHook insertion points in ExecuteBeadLoopRuntime; OutcomeReason field beside existing Disrupted in ExecuteBeadReport.

4. Amend FEAT-011 (skills): replace "ships exactly one skill" with the nested workflow-skill model demonstrated by bead-breakdown/, replay-bead/, etc. Document the 5 shipped-skill paths per codex review (skills/ddx/, cli/internal/skills/ddx/, .agents/skills/ddx/, .claude/skills/ddx/, .ddx/skills/ddx/).

5. Cross-link: ADR-023 references RELIABILITY-PRINCIPLES (ddx-06b77652) + evidence (ddx-f339c399) + bead-authoring-template.md as the canonical template.

NON-SCOPE
- New FEAT (codex: not warranted; existing FEATs cover the surface)
- ADR for the in_progress-eligibility bug (separate concern)
- Cross-project skill packaging (deferred per FEAT-015)
- Schema changes to bead JSON (waivers via labels; no new fields)
    </description>
    <acceptance>
1. docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md exists with the 5 sections enumerated in PROPOSED FIX (Policy, Staged rollout, Waiver model, Fail-open behavior, Recovery UX). Status: Proposed (Accepted after operator review).

2. FEAT-004-beads.md gains §"Authoring quality lint" (~50 lines) describing the lint hook + waiver storage. Cross-link to ADR-023.

3. FEAT-010-executions.md gains §"Quality hooks" (~50 lines) describing PreDispatchLintHook + PostAttemptTriageHook + OutcomeReason field. Cross-link to ADR-023.

4. FEAT-011-skills.md updated: stale "ships exactly one skill" replaced with nested workflow-skill model. Document the 5 shipped-skill paths. Cross-link to ADR-023.

5. ADR-023 cross-links: RELIABILITY-PRINCIPLES (ddx-06b77652), evidence (ddx-f339c399), bead-authoring-template.md, audits-1+2.

6. Conventional commit ending [&lt;this-bead-id&gt;]. Stage docs/helix/02-design/adr/ADR-023-*.md + docs/helix/01-frame/features/FEAT-{004,010,011}-*.md only.

7. lefthook run pre-commit passes.

8. Manual verification: 'grep -l ADR-023 docs/helix/' shows the ADR + 3 FEAT amendments.
    </acceptance>
    <labels>phase:2, area:beads, kind:design, reliability, bead-quality, adr:023</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260504T145446-7f69bc6f/manifest.json</file>
    <file>.ddx/executions/20260504T145446-7f69bc6f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b2efeefd256f02123f365c4032a09205a6b707aa">
diff --git a/.ddx/executions/20260504T145446-7f69bc6f/manifest.json b/.ddx/executions/20260504T145446-7f69bc6f/manifest.json
new file mode 100644
index 00000000..ca24db4b
--- /dev/null
+++ b/.ddx/executions/20260504T145446-7f69bc6f/manifest.json
@@ -0,0 +1,59 @@
+{
+  "attempt_id": "20260504T145446-7f69bc6f",
+  "bead_id": "ddx-9210f95a",
+  "base_rev": "2df2a24bbf8d1fd6d92d87bee54e6b4bc0e604ef",
+  "created_at": "2026-05-04T14:55:37.969351112Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9210f95a",
+    "title": "ADR-023 + FEAT-004/010/011 amendments: bead-lifecycle quality enforcement policy",
+    "description": "PROBLEM\nPer epic ddx-a9d130d0: bead-lifecycle quality framework needs an ADR documenting the policy + amendments to existing FEATs to formally adopt the lint/triage hooks and the sub-skill packaging change.\n\nROOT CAUSE\nToday no policy artifact exists for bead-quality enforcement. RELIABILITY-PRINCIPLES bead ddx-06b77652 documents P7 as a principle but does not specify the enforcement mechanism, waiver model, staged rollout, or recovery UX. Existing FEAT docs are stale on relevant points:\n- FEAT-011 at docs/helix/01-frame/features/FEAT-011-skills.md:180 says DDx ships exactly one skill, but .agents/skills/ddx/bead-breakdown/SKILL.md already demonstrates nested sub-skill packaging.\n- FEAT-004 at docs/helix/01-frame/features/FEAT-004-beads.md:150 owns bead validation hooks but doesn't describe authoring-quality lint or post-attempt triage.\n- FEAT-010 at docs/helix/01-frame/features/FEAT-010-executions.md:21 documents the layer-1/2/3 invocation model but doesn't spec hook insertion points for ddx try / ddx work.\n\nPROPOSED FIX\n1. Author ADR-023 at docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md covering:\n   - Policy: lint pre-dispatch + triage post-attempt; both invoke the same bead-lifecycle sub-skill\n   - Staged rollout: WARN-ONLY default → BLOCK opt-in after baseline confirms\n   - Waiver model: rubric-level by bead-type (doc/epic/deletion skip certain criteria, per docs/helix/06-iterate/bead-authoring-template.md:219); per-bead lint-waiver:\u003ccriterion\u003e label for rare exceptions; --force --reason records an event (does not silently bypass)\n   - Fail-open hook behavior (P1 from RELIABILITY-PRINCIPLES): infrastructure failure proceeds; only valid low lint score blocks (in BLOCK mode)\n   - Operator recovery UX: blocked dispatch prints missing fields + suggested ddx bead update commands\n\n2. Amend FEAT-004 (beads): add §\"Authoring quality lint\" describing the rubric, sub-skill location, waiver storage (label-based), and tracker metadata (no new schema fields; lint output ephemeral in evidence dir).\n\n3. Amend FEAT-010 (executions): add §\"Quality hooks\" describing PreDispatchLintHook + PostAttemptTriageHook insertion points in ExecuteBeadLoopRuntime; OutcomeReason field beside existing Disrupted in ExecuteBeadReport.\n\n4. Amend FEAT-011 (skills): replace \"ships exactly one skill\" with the nested workflow-skill model demonstrated by bead-breakdown/, replay-bead/, etc. Document the 5 shipped-skill paths per codex review (skills/ddx/, cli/internal/skills/ddx/, .agents/skills/ddx/, .claude/skills/ddx/, .ddx/skills/ddx/).\n\n5. Cross-link: ADR-023 references RELIABILITY-PRINCIPLES (ddx-06b77652) + evidence (ddx-f339c399) + bead-authoring-template.md as the canonical template.\n\nNON-SCOPE\n- New FEAT (codex: not warranted; existing FEATs cover the surface)\n- ADR for the in_progress-eligibility bug (separate concern)\n- Cross-project skill packaging (deferred per FEAT-015)\n- Schema changes to bead JSON (waivers via labels; no new fields)",
+    "acceptance": "1. docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md exists with the 5 sections enumerated in PROPOSED FIX (Policy, Staged rollout, Waiver model, Fail-open behavior, Recovery UX). Status: Proposed (Accepted after operator review).\n\n2. FEAT-004-beads.md gains §\"Authoring quality lint\" (~50 lines) describing the lint hook + waiver storage. Cross-link to ADR-023.\n\n3. FEAT-010-executions.md gains §\"Quality hooks\" (~50 lines) describing PreDispatchLintHook + PostAttemptTriageHook + OutcomeReason field. Cross-link to ADR-023.\n\n4. FEAT-011-skills.md updated: stale \"ships exactly one skill\" replaced with nested workflow-skill model. Document the 5 shipped-skill paths. Cross-link to ADR-023.\n\n5. ADR-023 cross-links: RELIABILITY-PRINCIPLES (ddx-06b77652), evidence (ddx-f339c399), bead-authoring-template.md, audits-1+2.\n\n6. Conventional commit ending [\u003cthis-bead-id\u003e]. Stage docs/helix/02-design/adr/ADR-023-*.md + docs/helix/01-frame/features/FEAT-{004,010,011}-*.md only.\n\n7. lefthook run pre-commit passes.\n\n8. Manual verification: 'grep -l ADR-023 docs/helix/' shows the ADR + 3 FEAT amendments.",
+    "parent": "ddx-a9d130d0",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "kind:design",
+      "reliability",
+      "bead-quality",
+      "adr:023"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-04T14:54:43Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3540416",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-05-04T14:44:19.4585742Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "erik",
+          "body": "creating isolated worktree: git worktree add: Preparing worktree (detached HEAD fb72b7a7)\nUpdating files:   0% (16/13985)\rUpdating files:   1% (140/13985)\rUpdating files:   2% (280/13985)\rUpdating files:   3% (420/13985)\rUpdating files:   4% (560/13985)\rUpdating files:   5% (700/13985)\rUpdating files:   6% (840/13985)\rUpdating files:   7% (979/13985)\rUpdating files:   7% (1087/13985)\rUpdating files:   8% (1119/13985)\rUpdating files:   9% (1259/13985)\rUpdating files:  10% (1399/13985)\rUpdating files:  11% (1539/13985)\rUpdating files:  12% (1679/13985)\rUpdating files:  13% (1819/13985)\rUpdating files:  13% (1953/13985)\rUpdating files:  14% (1958/13985)\rUpdating files:  15% (2098/13985)\rUpdating files:  16% (2238/13985)\rUpdating files:  17% (2378/13985)\rUpdating files:  18% (2518/13985)\rUpdating files:  19% (2658/13985)\rUpdating files:  20% (2797/13985)\rUpdating files:  21% (2937/13985)\rUpdating files:  21% (2938/13985)\rUpdating files:  22% (3077/13985)\rUpdating files:  23% (3217/13985)\rUpdating files:  24% (3357/13985)\rUpdating files:  25% (3497/13985)\rUpdating files:  26% (3637/13985)\rUpdating files:  27% (3776/13985)\rUpdating files:  28% (3916/13985)\rUpdating files:  29% (4056/13985)\rUpdating files:  29% (4149/13985)\rUpdating files:  30% (4196/13985)\rUpdating files:  31% (4336/13985)\rUpdating files:  32% (4476/13985)\rUpdating files:  33% (4616/13985)\rUpdating files:  34% (4755/13985)\rUpdating files:  35% (4895/13985)\rUpdating files:  36% (5035/13985)\rUpdating files:  37% (5175/13985)\rUpdating files:  37% (5176/13985)\rUpdating files:  38% (5315/13985)\rerror: unable to create file .ddx/attachments/ddx-b52fc7f2/events.jsonl: No space left on device\nfatal: cannot create directory at '.ddx/attachments/ddx-b56fdc74': No space left on device: exit status 128",
+          "created_at": "2026-05-04T14:46:23.731906048Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-04T14:54:43.901438473Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260504T145446-7f69bc6f",
+    "prompt": ".ddx/executions/20260504T145446-7f69bc6f/prompt.md",
+    "manifest": ".ddx/executions/20260504T145446-7f69bc6f/manifest.json",
+    "result": ".ddx/executions/20260504T145446-7f69bc6f/result.json",
+    "checks": ".ddx/executions/20260504T145446-7f69bc6f/checks.json",
+    "usage": ".ddx/executions/20260504T145446-7f69bc6f/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9210f95a-20260504T145446-7f69bc6f"
+  },
+  "prompt_sha": "b155a6b7e77ffdcb1b62ed26e4b95d9911b5973141c6e25107b61919ac71a096"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T145446-7f69bc6f/result.json b/.ddx/executions/20260504T145446-7f69bc6f/result.json
new file mode 100644
index 00000000..d5a20f93
--- /dev/null
+++ b/.ddx/executions/20260504T145446-7f69bc6f/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-9210f95a",
+  "attempt_id": "20260504T145446-7f69bc6f",
+  "base_rev": "2df2a24bbf8d1fd6d92d87bee54e6b4bc0e604ef",
+  "result_rev": "d79b256764ae184ec09fd95658e34e4c416f33c6",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-8ce23542",
+  "duration_ms": 293734,
+  "tokens": 1874750,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260504T145446-7f69bc6f",
+  "prompt_file": ".ddx/executions/20260504T145446-7f69bc6f/prompt.md",
+  "manifest_file": ".ddx/executions/20260504T145446-7f69bc6f/manifest.json",
+  "result_file": ".ddx/executions/20260504T145446-7f69bc6f/result.json",
+  "usage_file": ".ddx/executions/20260504T145446-7f69bc6f/usage.json",
+  "started_at": "2026-05-04T14:55:37.983397983Z",
+  "finished_at": "2026-05-04T15:00:31.718234283Z"
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

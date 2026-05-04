<bead-review>
  <bead id="ddx-922b5b1f" iter=1>
    <title>skill: bead-lifecycle sub-skill (lint + triage + refine modes) shipped to all 5 paths + 5-10 curated examples</title>
    <description>
PROBLEM
Per epic ddx-a9d130d0 + ADR-023 (filed as ddx-XXXX): a sub-skill is needed at .agents/skills/ddx/bead-lifecycle/ (and 4 other shipped paths) carrying the rubric, examples, and judge prompts for lint + triage + refine modes. ddx try will compose this sub-skill via the runner library.

ROOT CAUSE
.agents/skills/ddx/bead-breakdown/SKILL.md (existing) is the precedent for nested workflow sub-skills under the parent ddx skill. No equivalent exists for bead authoring quality. The bead-authoring-template.md at docs/helix/06-iterate/bead-authoring-template.md is the canonical template (per codex review) but only has 1 strong example + 1 weak example (line 98 of template).

PROPOSED FIX
1. Create sub-skill directory bead-lifecycle/ under each of 5 shipped paths (codex amendment): skills/ddx/, cli/internal/skills/ddx/, .agents/skills/ddx/, .claude/skills/ddx/, .ddx/skills/ddx/. SKILL.md + supporting files identical across all 5 (or symlinked if the build process supports it; check current bead-breakdown/ packaging).

2. SKILL.md frontmatter: name=bead-lifecycle, description="Score, classify, and refine ddx beads. Used by ddx try hooks pre-dispatch (lint mode) and post-attempt (triage mode); operator-invocable for refine mode."

3. SKILL.md body sections:
   - LINT MODE: input=bead JSON; output=JSON {score 0-8, rationale, suggested_fixes, waivers_applied}. References the 8-criterion rubric from bead-authoring-template.md.
   - TRIAGE MODE: input=bead + outcome event + session log excerpt; output=JSON {classification ∈ {already_satisfied, ambiguous, investigated_no_path, decomposed, operator_input_needed, routing, quota, transport, tests_red, merge_conflict, review_block, timeout}, recommended_action ∈ {refine_bead_and_retry, retry_with_more_context, file_children_and_supersede, escalate_to_operator, close_as_already_done, wait_and_retry, give_up}, rationale, suggested_amendments, suggested_followup_beads}.
   - REFINE MODE: input=bead + (optional) prior triage output; output=YAML diff suggesting amendments to bead description + AC.
   - WAIVER TABLE: per-bead-type criterion skips (doc-only skips named-tests + test-name; epic skips concrete-implementation; deletion skips wired-in-assertion). Codex: rubric-first, per-bead label override second.
   - 5-10 CURATED EXAMPLES under bead-lifecycle/examples/: code bug, feature, doc-only, epic, deletion/rename, no-op investigation, upstream/external work. Operator (epic owner ddx-a9d130d0) curates from existing closed beads; LLM does not pick.

4. Mode selection: invocation prompt MUST include "MODE: lint|triage|refine" as first line; SKILL.md instructs the LLM to read mode and apply only that mode's contract.

NON-SCOPE
- LLM judgment baked into Go (this bead is data + markdown only; no code)
- ddx try hook integration (separate child bead)
- Tooling to validate skill output JSON (deferred; LLMs are usually well-behaved with structured-output prompts)
- Cross-project skill packaging (DDx-only first per FEAT-015)
    </description>
    <acceptance>
1. Sub-skill bead-lifecycle/SKILL.md exists in all 5 shipped paths: skills/ddx/, cli/internal/skills/ddx/, .agents/skills/ddx/, .claude/skills/ddx/, .ddx/skills/ddx/. Identical content (or symlinked per existing bead-breakdown/ packaging convention).

2. SKILL.md frontmatter has name=bead-lifecycle + description matching the proposed wording.

3. SKILL.md body has 4 sections: LINT MODE, TRIAGE MODE, REFINE MODE, WAIVER TABLE. Each MODE section has explicit JSON output contract with named fields.

4. bead-lifecycle/examples/ directory has 5-10 example files (curated by epic owner from closed beads; one per bead-type at minimum).

5. WIRED-IN: parent ddx skill (.agents/skills/ddx/SKILL.md router section) gains a line pointing operators/agents at bead-lifecycle/ for "score a bead" / "triage a failed attempt" / "refine a bead" intents.

6. Manual verification: 'find . -path "*/bead-lifecycle/SKILL.md" -not -path "./.git/*"' returns 5 paths.

7. Manual verification: invoke skill via 'ddx agent run --harness=sonnet --text="MODE: lint\nUse the bead-lifecycle skill to lint bead ddx-1e516bc9. Return JSON only."' and observe valid JSON output with score field.

8. Conventional commit ending [&lt;this-bead-id&gt;]. Stage the 5 SKILL.md paths + bead-lifecycle/examples/* only.

9. lefthook run pre-commit passes (skill-schema check exists at lefthook config; will validate SKILL.md frontmatter).
    </acceptance>
    <labels>phase:2, area:beads, area:skills, kind:feature, reliability, bead-quality, adr:023</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260504T151256-31de7eb5/manifest.json</file>
    <file>.ddx/executions/20260504T151256-31de7eb5/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="3af8c162d43c34005e61b8333d09ebee47f603b2">
diff --git a/.ddx/executions/20260504T151256-31de7eb5/manifest.json b/.ddx/executions/20260504T151256-31de7eb5/manifest.json
new file mode 100644
index 00000000..caa25322
--- /dev/null
+++ b/.ddx/executions/20260504T151256-31de7eb5/manifest.json
@@ -0,0 +1,52 @@
+{
+  "attempt_id": "20260504T151256-31de7eb5",
+  "bead_id": "ddx-922b5b1f",
+  "base_rev": "9c35b9436ad0b9841435628189872a3d92fc7ad0",
+  "created_at": "2026-05-04T15:13:40.192478844Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-922b5b1f",
+    "title": "skill: bead-lifecycle sub-skill (lint + triage + refine modes) shipped to all 5 paths + 5-10 curated examples",
+    "description": "PROBLEM\nPer epic ddx-a9d130d0 + ADR-023 (filed as ddx-XXXX): a sub-skill is needed at .agents/skills/ddx/bead-lifecycle/ (and 4 other shipped paths) carrying the rubric, examples, and judge prompts for lint + triage + refine modes. ddx try will compose this sub-skill via the runner library.\n\nROOT CAUSE\n.agents/skills/ddx/bead-breakdown/SKILL.md (existing) is the precedent for nested workflow sub-skills under the parent ddx skill. No equivalent exists for bead authoring quality. The bead-authoring-template.md at docs/helix/06-iterate/bead-authoring-template.md is the canonical template (per codex review) but only has 1 strong example + 1 weak example (line 98 of template).\n\nPROPOSED FIX\n1. Create sub-skill directory bead-lifecycle/ under each of 5 shipped paths (codex amendment): skills/ddx/, cli/internal/skills/ddx/, .agents/skills/ddx/, .claude/skills/ddx/, .ddx/skills/ddx/. SKILL.md + supporting files identical across all 5 (or symlinked if the build process supports it; check current bead-breakdown/ packaging).\n\n2. SKILL.md frontmatter: name=bead-lifecycle, description=\"Score, classify, and refine ddx beads. Used by ddx try hooks pre-dispatch (lint mode) and post-attempt (triage mode); operator-invocable for refine mode.\"\n\n3. SKILL.md body sections:\n   - LINT MODE: input=bead JSON; output=JSON {score 0-8, rationale, suggested_fixes, waivers_applied}. References the 8-criterion rubric from bead-authoring-template.md.\n   - TRIAGE MODE: input=bead + outcome event + session log excerpt; output=JSON {classification ∈ {already_satisfied, ambiguous, investigated_no_path, decomposed, operator_input_needed, routing, quota, transport, tests_red, merge_conflict, review_block, timeout}, recommended_action ∈ {refine_bead_and_retry, retry_with_more_context, file_children_and_supersede, escalate_to_operator, close_as_already_done, wait_and_retry, give_up}, rationale, suggested_amendments, suggested_followup_beads}.\n   - REFINE MODE: input=bead + (optional) prior triage output; output=YAML diff suggesting amendments to bead description + AC.\n   - WAIVER TABLE: per-bead-type criterion skips (doc-only skips named-tests + test-name; epic skips concrete-implementation; deletion skips wired-in-assertion). Codex: rubric-first, per-bead label override second.\n   - 5-10 CURATED EXAMPLES under bead-lifecycle/examples/: code bug, feature, doc-only, epic, deletion/rename, no-op investigation, upstream/external work. Operator (epic owner ddx-a9d130d0) curates from existing closed beads; LLM does not pick.\n\n4. Mode selection: invocation prompt MUST include \"MODE: lint|triage|refine\" as first line; SKILL.md instructs the LLM to read mode and apply only that mode's contract.\n\nNON-SCOPE\n- LLM judgment baked into Go (this bead is data + markdown only; no code)\n- ddx try hook integration (separate child bead)\n- Tooling to validate skill output JSON (deferred; LLMs are usually well-behaved with structured-output prompts)\n- Cross-project skill packaging (DDx-only first per FEAT-015)",
+    "acceptance": "1. Sub-skill bead-lifecycle/SKILL.md exists in all 5 shipped paths: skills/ddx/, cli/internal/skills/ddx/, .agents/skills/ddx/, .claude/skills/ddx/, .ddx/skills/ddx/. Identical content (or symlinked per existing bead-breakdown/ packaging convention).\n\n2. SKILL.md frontmatter has name=bead-lifecycle + description matching the proposed wording.\n\n3. SKILL.md body has 4 sections: LINT MODE, TRIAGE MODE, REFINE MODE, WAIVER TABLE. Each MODE section has explicit JSON output contract with named fields.\n\n4. bead-lifecycle/examples/ directory has 5-10 example files (curated by epic owner from closed beads; one per bead-type at minimum).\n\n5. WIRED-IN: parent ddx skill (.agents/skills/ddx/SKILL.md router section) gains a line pointing operators/agents at bead-lifecycle/ for \"score a bead\" / \"triage a failed attempt\" / \"refine a bead\" intents.\n\n6. Manual verification: 'find . -path \"*/bead-lifecycle/SKILL.md\" -not -path \"./.git/*\"' returns 5 paths.\n\n7. Manual verification: invoke skill via 'ddx agent run --harness=sonnet --text=\"MODE: lint\\nUse the bead-lifecycle skill to lint bead ddx-1e516bc9. Return JSON only.\"' and observe valid JSON output with score field.\n\n8. Conventional commit ending [\u003cthis-bead-id\u003e]. Stage the 5 SKILL.md paths + bead-lifecycle/examples/* only.\n\n9. lefthook run pre-commit passes (skill-schema check exists at lefthook config; will validate SKILL.md frontmatter).",
+    "parent": "ddx-a9d130d0",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:skills",
+      "kind:feature",
+      "reliability",
+      "bead-quality",
+      "adr:023"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-04T15:12:56Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3551096",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "staging tracker: fatal: Unable to create '/home/erik/Projects/ddx/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository, or the lock file may be stale: exit status 128",
+          "created_at": "2026-05-04T15:12:01.707498243Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-04T15:12:56.389313873Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260504T151256-31de7eb5",
+    "prompt": ".ddx/executions/20260504T151256-31de7eb5/prompt.md",
+    "manifest": ".ddx/executions/20260504T151256-31de7eb5/manifest.json",
+    "result": ".ddx/executions/20260504T151256-31de7eb5/result.json",
+    "checks": ".ddx/executions/20260504T151256-31de7eb5/checks.json",
+    "usage": ".ddx/executions/20260504T151256-31de7eb5/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-922b5b1f-20260504T151256-31de7eb5"
+  },
+  "prompt_sha": "dc37c926dfc83668fc0171af9efa82adeea6bef0aecd7ec051a046af2012c757"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260504T151256-31de7eb5/result.json b/.ddx/executions/20260504T151256-31de7eb5/result.json
new file mode 100644
index 00000000..39f7b903
--- /dev/null
+++ b/.ddx/executions/20260504T151256-31de7eb5/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-922b5b1f",
+  "attempt_id": "20260504T151256-31de7eb5",
+  "base_rev": "9c35b9436ad0b9841435628189872a3d92fc7ad0",
+  "result_rev": "100939a021141297b16f198f7cc36d48e6587f2b",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-6b3018f9",
+  "duration_ms": 674019,
+  "tokens": 3908090,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260504T151256-31de7eb5",
+  "prompt_file": ".ddx/executions/20260504T151256-31de7eb5/prompt.md",
+  "manifest_file": ".ddx/executions/20260504T151256-31de7eb5/manifest.json",
+  "result_file": ".ddx/executions/20260504T151256-31de7eb5/result.json",
+  "usage_file": ".ddx/executions/20260504T151256-31de7eb5/usage.json",
+  "started_at": "2026-05-04T15:13:40.213446256Z",
+  "finished_at": "2026-05-04T15:24:54.233204964Z"
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

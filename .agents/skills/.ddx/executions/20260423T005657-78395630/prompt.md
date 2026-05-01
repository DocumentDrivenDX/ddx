<bead-review>
  <bead id="ddx-915240dd" iter=1>
    <title>routing: pick up agent endpoint-first redesign (upstream agent-0c6189f5)</title>
    <description>
Track the DDx-side pickup of the upstream endpoint-first routing redesign (agent-0c6189f5 in ~/Projects/agent tracker).

Until upstream ships the new endpoint-only config schema and runtime discovery, DDx's 32-test fake-migration children (ddx-68c372a6 through ddx-27e2b5ce) cannot drain on local models — the current named-profile 'vidar-omlx' vocabulary routes to dead endpoints and 404s on live ones (zero successes across a 30-attempt drain on 2026-04-21).

When agent-0c6189f5 lands:
1. Bump cli/go.mod to the new agent release.
2. Migrate .ddx/config.yaml from named profiles to endpoint blocks per the new schema.
3. Re-run 'ddx work --no-adaptive-min-tier --min-tier cheap --max-tier cheap' against the 32 ready fake-migration beads; expect them to resolve 'qwen3.6' → whatever live endpoint serves it and actually execute.
    </description>
    <acceptance>
DDx's config no longer names provider profiles in the agent.routing/agent.endpoints block (or equivalent post-redesign schema); drains against cheap-only routing land at live endpoints; 32 fake-migration children begin making progress on local models.
    </acceptance>
    <labels>area:agent, area:routing, kind:integration, phase:build, workstream:agent-upgrade, blocked-on-upstream</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="78a208d49e14c94d87c09e32028e2dac5231254b">
commit 78a208d49e14c94d87c09e32028e2dac5231254b
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 20:56:55 2026 -0400

    chore: add execution evidence [20260423T003944-]

diff --git a/.ddx/executions/20260423T003944-75ce1dd4/result.json b/.ddx/executions/20260423T003944-75ce1dd4/result.json
new file mode 100644
index 00000000..688576bd
--- /dev/null
+++ b/.ddx/executions/20260423T003944-75ce1dd4/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-915240dd",
+  "attempt_id": "20260423T003944-75ce1dd4",
+  "base_rev": "6717c709bbe151d5cad47343b799d82e86131c48",
+  "result_rev": "010b2855d3d8522a7970f97331a1078f54e6ac4d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-417d5f7b",
+  "duration_ms": 1029369,
+  "tokens": 17913383,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T003944-75ce1dd4",
+  "prompt_file": ".ddx/executions/20260423T003944-75ce1dd4/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T003944-75ce1dd4/manifest.json",
+  "result_file": ".ddx/executions/20260423T003944-75ce1dd4/result.json",
+  "usage_file": ".ddx/executions/20260423T003944-75ce1dd4/usage.json",
+  "started_at": "2026-04-23T00:39:45.014543994Z",
+  "finished_at": "2026-04-23T00:56:54.384130292Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

## Your task

Examine the diff and each acceptance-criteria (AC) item. For each item assign one grade:

- **APPROVE** — fully and correctly implemented; cite the specific file path and line that proves it.
- **REQUEST_CHANGES** — partially implemented or has fixable minor issues.
- **BLOCK** — not implemented, incorrectly implemented, or the diff is insufficient to evaluate.

Overall verdict rule:
- All items APPROVE → **APPROVE**
- Any item BLOCK → **BLOCK**
- Otherwise → **REQUEST_CHANGES**

## Required output format

Respond with a structured review using exactly this layout (replace placeholder text):

---
## Review: ddx-915240dd iter 1

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### AC Grades

| # | Item | Grade | Evidence |
|---|------|-------|----------|
| 1 | &lt;AC item text, max 60 chars&gt; | APPROVE | path/to/file.go:42 — brief note |
| 2 | &lt;AC item text, max 60 chars&gt; | BLOCK   | — not found in diff |

### Summary

&lt;1–3 sentences on overall implementation quality and any recurring theme in findings.&gt;

### Findings

&lt;Bullet list of REQUEST_CHANGES and BLOCK findings. Each finding must name the specific file, function, or test that is missing or wrong — specific enough for the next agent to act on without re-reading the entire diff. Omit this section entirely if verdict is APPROVE.&gt;
  </instructions>
</bead-review>

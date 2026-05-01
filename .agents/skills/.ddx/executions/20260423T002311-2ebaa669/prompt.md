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

  <diff rev="2ec57f26cf12b20b9837e12735c52b16ea81a18b">
commit 2ec57f26cf12b20b9837e12735c52b16ea81a18b
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 20:23:07 2026 -0400

    chore: add execution evidence [20260422T233901-]

diff --git a/.ddx/executions/20260422T233901-96b38324/result.json b/.ddx/executions/20260422T233901-96b38324/result.json
new file mode 100644
index 00000000..2b990e27
--- /dev/null
+++ b/.ddx/executions/20260422T233901-96b38324/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-915240dd",
+  "attempt_id": "20260422T233901-96b38324",
+  "base_rev": "6cb88892ab2c7fe00ff08853a1b0d429b8732128",
+  "result_rev": "eff53c130107d8b74a5f55e5fd7fab31a30d66d3",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a5f5d5c6",
+  "duration_ms": 2642201,
+  "tokens": 24642,
+  "cost_usd": 5.416315249999999,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T233901-96b38324",
+  "prompt_file": ".ddx/executions/20260422T233901-96b38324/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T233901-96b38324/manifest.json",
+  "result_file": ".ddx/executions/20260422T233901-96b38324/result.json",
+  "usage_file": ".ddx/executions/20260422T233901-96b38324/usage.json",
+  "started_at": "2026-04-22T23:39:02.117665887Z",
+  "finished_at": "2026-04-23T00:23:04.318734918Z"
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

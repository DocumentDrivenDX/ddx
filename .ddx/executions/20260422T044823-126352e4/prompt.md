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
    <labels>area:agent, area:routing, kind:integration, phase:build, workstream:agent-upgrade</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a0ead6f713734a177ac5ed18687b93a24f2152bb">
commit a0ead6f713734a177ac5ed18687b93a24f2152bb
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 00:48:21 2026 -0400

    chore: add execution evidence [20260422T042737-]

diff --git a/.ddx/executions/20260422T042737-8edaef14/manifest.json b/.ddx/executions/20260422T042737-8edaef14/manifest.json
new file mode 100644
index 00000000..0bbbaaab
--- /dev/null
+++ b/.ddx/executions/20260422T042737-8edaef14/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260422T042737-8edaef14",
+  "bead_id": "ddx-915240dd",
+  "base_rev": "f08e791ff3b81aa694f1cd4ba03efc84e0bd40b0",
+  "created_at": "2026-04-22T04:27:38.060294292Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-915240dd",
+    "title": "routing: pick up agent endpoint-first redesign (upstream agent-0c6189f5)",
+    "description": "Track the DDx-side pickup of the upstream endpoint-first routing redesign (agent-0c6189f5 in ~/Projects/agent tracker).\n\nUntil upstream ships the new endpoint-only config schema and runtime discovery, DDx's 32-test fake-migration children (ddx-68c372a6 through ddx-27e2b5ce) cannot drain on local models — the current named-profile 'vidar-omlx' vocabulary routes to dead endpoints and 404s on live ones (zero successes across a 30-attempt drain on 2026-04-21).\n\nWhen agent-0c6189f5 lands:\n1. Bump cli/go.mod to the new agent release.\n2. Migrate .ddx/config.yaml from named profiles to endpoint blocks per the new schema.\n3. Re-run 'ddx work --no-adaptive-min-tier --min-tier cheap --max-tier cheap' against the 32 ready fake-migration beads; expect them to resolve 'qwen3.6' → whatever live endpoint serves it and actually execute.",
+    "acceptance": "DDx's config no longer names provider profiles in the agent.routing/agent.endpoints block (or equivalent post-redesign schema); drains against cheap-only routing land at live endpoints; 32 fake-migration children begin making progress on local models.",
+    "labels": [
+      "area:agent",
+      "area:routing",
+      "kind:integration",
+      "phase:build",
+      "workstream:agent-upgrade"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-22T04:27:37Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1682344",
+      "execute-loop-heartbeat-at": "2026-04-22T04:27:37.624047411Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260422T042737-8edaef14",
+    "prompt": ".ddx/executions/20260422T042737-8edaef14/prompt.md",
+    "manifest": ".ddx/executions/20260422T042737-8edaef14/manifest.json",
+    "result": ".ddx/executions/20260422T042737-8edaef14/result.json",
+    "checks": ".ddx/executions/20260422T042737-8edaef14/checks.json",
+    "usage": ".ddx/executions/20260422T042737-8edaef14/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-915240dd-20260422T042737-8edaef14"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260422T042737-8edaef14/result.json b/.ddx/executions/20260422T042737-8edaef14/result.json
new file mode 100644
index 00000000..9f1fe623
--- /dev/null
+++ b/.ddx/executions/20260422T042737-8edaef14/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-915240dd",
+  "attempt_id": "20260422T042737-8edaef14",
+  "base_rev": "f08e791ff3b81aa694f1cd4ba03efc84e0bd40b0",
+  "result_rev": "a1cd7eb19828f3f2b70fabae0db0441a32f1f19f",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-8e13b5ee",
+  "duration_ms": 1243019,
+  "tokens": 14568911,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T042737-8edaef14",
+  "prompt_file": ".ddx/executions/20260422T042737-8edaef14/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T042737-8edaef14/manifest.json",
+  "result_file": ".ddx/executions/20260422T042737-8edaef14/result.json",
+  "usage_file": ".ddx/executions/20260422T042737-8edaef14/usage.json",
+  "started_at": "2026-04-22T04:27:38.060540333Z",
+  "finished_at": "2026-04-22T04:48:21.08020912Z"
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

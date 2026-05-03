<bead-review>
  <bead id="ddx-bbdd7564" iter=1>
    <title>Introduce selectable bead Backend interface in cli/internal/bead</title>
    <description>
Refactor cli/internal/bead to expose a Backend interface (per the TD from ddx-6bb51a52) so the existing JSONL+flock+git store becomes one implementation among many. No behavior change yet — JSONL backend remains default. Add a config knob (e.g. .ddx.yml beads.backend or DDX_BEAD_BACKEND env) to select a backend. Existing tests must keep passing unchanged.
    </description>
    <acceptance>
1. cli/internal/bead/backend.go defines the Backend interface (CRUD, claim, list/ready/blocked, dep ops, events append, archive split, JSONL export/import). 2. Current JSONL implementation refactored behind the interface; no caller changes outward behavior. 3. Backend selection plumbed via config + env (default jsonl). 4. go test ./cli/... passes. 5. chaos_test.go runs against the JSONL backend through the interface.
    </acceptance>
    <labels>phase:2, area:beads, area:storage, kind:refactor, backend-migration</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="7053b16ee1145a479aca0113c342f4a0dba70479">
commit 7053b16ee1145a479aca0113c342f4a0dba70479
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat May 2 22:56:45 2026 -0400

    chore: add execution evidence [20260503T024930-]

diff --git a/.ddx/executions/20260503T024930-354b52b3/result.json b/.ddx/executions/20260503T024930-354b52b3/result.json
new file mode 100644
index 00000000..08ce3297
--- /dev/null
+++ b/.ddx/executions/20260503T024930-354b52b3/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-bbdd7564",
+  "attempt_id": "20260503T024930-354b52b3",
+  "base_rev": "f895660f9e886086ae67c2e812ffdf8e873fb66f",
+  "result_rev": "4dc160548bd6cb87b5d7c7eda516ff1519679a64",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-8c4307c0",
+  "duration_ms": 430207,
+  "tokens": 19052,
+  "cost_usd": 3.26959875,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T024930-354b52b3",
+  "prompt_file": ".ddx/executions/20260503T024930-354b52b3/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T024930-354b52b3/manifest.json",
+  "result_file": ".ddx/executions/20260503T024930-354b52b3/result.json",
+  "usage_file": ".ddx/executions/20260503T024930-354b52b3/usage.json",
+  "started_at": "2026-05-03T02:49:31.899943224Z",
+  "finished_at": "2026-05-03T02:56:42.107206759Z"
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
## Review: ddx-bbdd7564 iter 1

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

<bead-review>
  <bead id="ddx-84daf44b" iter=1>
    <title>FEAT-008 P1: shared primitives (ConfirmDialog, TypedConfirmDialog, Tooltip)</title>
    <description>
Extract shared UI primitives used by 5+ callers: (a) &lt;ConfirmDialog&gt; — action label, destructive flag, summary slot, focus return on close, Escape cancel, aria-label = action label; (b) &lt;TypedConfirmDialog extends ConfirmDialog&gt; — gates confirm until user types expectedText (bead id / plugin name); (c) &lt;Tooltip&gt; — bits-ui wrapper. CRITICAL: for disabled-button tooltip, wrap the button in a &lt;span&gt; that owns the hover handler (disabled buttons don't fire mouse events). Keep native &lt;select&gt; for tier/role/project/model fields (Playwright .selectOption only works on native select). Use bits-ui Command for palette (Phase 4).
    </description>
    <acceptance>
Primitives exist in src/lib/components/; unit-test smoke passes; no consumer wired yet.
    </acceptance>
    <labels>ui, feat-008, phase-1</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a9e34a8f77f06311fe456b3aa181a200d9079f1d">
commit a9e34a8f77f06311fe456b3aa181a200d9079f1d
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 04:22:06 2026 -0400

    chore: add execution evidence [20260422T081417-]

diff --git a/.ddx/executions/20260422T081417-f5064b77/result.json b/.ddx/executions/20260422T081417-f5064b77/result.json
new file mode 100644
index 00000000..27cf93e6
--- /dev/null
+++ b/.ddx/executions/20260422T081417-f5064b77/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-84daf44b",
+  "attempt_id": "20260422T081417-f5064b77",
+  "base_rev": "1db845626fa2e3fde0adb257f408fa19544c5e78",
+  "result_rev": "808ccfdd730ea42fc4072160eb7e67fe2104fe67",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-5bd67cc3",
+  "duration_ms": 467632,
+  "tokens": 3257902,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T081417-f5064b77",
+  "prompt_file": ".ddx/executions/20260422T081417-f5064b77/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T081417-f5064b77/manifest.json",
+  "result_file": ".ddx/executions/20260422T081417-f5064b77/result.json",
+  "usage_file": ".ddx/executions/20260422T081417-f5064b77/usage.json",
+  "started_at": "2026-04-22T08:14:17.833907324Z",
+  "finished_at": "2026-04-22T08:22:05.466357095Z"
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
## Review: ddx-84daf44b iter 1

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

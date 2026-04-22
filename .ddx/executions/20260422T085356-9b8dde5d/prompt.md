<bead-review>
  <bead id="ddx-a308bd84" iter=1>
    <title>FEAT-008 P6: US-081a + US-083a documents SPA + plain/wysiwyg toggle</title>
    <description>
Rendered markdown: intercept intra-repo [](./path.md) → goto; same-doc #anchor → smooth-scroll; external → target=_blank rel=noopener. Back button preserves document state. Toggle is RADIOS (not a button). WYSIWYG editor has data-testid=wysiwyg-editor; Plain textarea labelled /plain markdown editor/i. WYSIWYG mode shows collapsible 'Frontmatter' panel ABOVE content (per FEAT-008 AC line 591-593). Plain mode shows raw markdown INCLUDING frontmatter inline. Single source-of-truth state in parent; edits survive mode switch. Save via documentWrite; updated ts refreshes from response.
    </description>
    <acceptance>
US-081a.a-c + US-083a.a-d green in e2e/documents.spec.ts; screenshots committed for /documents.
    </acceptance>
    <labels>ui, feat-008, phase-6, us-081a, us-083a</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="b1347b8a7ff05383875c00fcbd87175fd2a8a0dd">
commit b1347b8a7ff05383875c00fcbd87175fd2a8a0dd
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 04:53:54 2026 -0400

    chore: add execution evidence [20260422T083901-]

diff --git a/.ddx/executions/20260422T083901-a06a5e1a/result.json b/.ddx/executions/20260422T083901-a06a5e1a/result.json
new file mode 100644
index 00000000..6eada7b9
--- /dev/null
+++ b/.ddx/executions/20260422T083901-a06a5e1a/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-a308bd84",
+  "attempt_id": "20260422T083901-a06a5e1a",
+  "base_rev": "740377fb43c3bb08acee197e78a2bec8ca62f438",
+  "result_rev": "9369b92cca035eaf1de181237a723b1bf9468e85",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-9778688d",
+  "duration_ms": 891446,
+  "tokens": 7861962,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T083901-a06a5e1a",
+  "prompt_file": ".ddx/executions/20260422T083901-a06a5e1a/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T083901-a06a5e1a/manifest.json",
+  "result_file": ".ddx/executions/20260422T083901-a06a5e1a/result.json",
+  "usage_file": ".ddx/executions/20260422T083901-a06a5e1a/usage.json",
+  "started_at": "2026-04-22T08:39:02.033126058Z",
+  "finished_at": "2026-04-22T08:53:53.479446605Z"
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
## Review: ddx-a308bd84 iter 1

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

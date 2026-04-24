<bead-review>
  <bead id="ddx-71fbe968" iter=1>
    <title>Bead detail header: long bead ids overlap action buttons (shrink-0 prevents truncation)</title>
    <description>
## Observed

On the bead detail panel, a long bead id overflows and visually overlaps the Claim/Unclaim/Edit/Delete/Close buttons on the right. Reporter example: `.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1` (~60 chars).

## Root cause

`cli/internal/server/frontend/src/lib/components/BeadDetail.svelte:211-221` —

```svelte
&lt;div class="flex shrink-0 items-center justify-between border-b ... px-6 py-4"&gt;
  &lt;div class="flex min-w-0 items-center gap-3"&gt;
    &lt;span class="shrink-0 font-mono text-xs ..."&gt;{bead.id}&lt;/span&gt;       ← shrink-0 prevents truncation
    &lt;span class="shrink-0 font-medium ..."&gt;{bead.status}&lt;/span&gt;
    {#if bead.owner}
      &lt;span class="truncate text-xs ..."&gt;@ {bead.owner}&lt;/span&gt;
    {/if}
  &lt;/div&gt;
  &lt;div class="ml-3 flex shrink-0 items-center gap-2"&gt;
    [Claim/Unclaim] [Edit] [Delete] [Close]
  &lt;/div&gt;
&lt;/div&gt;
```

The bead-id span has `shrink-0`, so it never truncates regardless of length. The outer panel is `max-w-xl` (576px). A 60-char monospace id occupies roughly 400-500px on its own; combined with the status pill, owner chip, and the action-button group (also `shrink-0`), it pushes past the available width. Flex layout collapses by overlapping rather than wrapping, so the id draws on top of the buttons.

## Proposed direction

1. **Replace `shrink-0` on the bead-id span with `truncate min-w-0`.** The parent `&lt;div class="flex min-w-0 items-center gap-3"&gt;` already establishes a shrinkable container.
2. **Add `title={bead.id}` so hovering shows the full value** (important — truncating monospace ids from the right loses the distinguishing suffix).
3. **Add a copy-to-clipboard icon button** next to the id so operators can grab the full value with one click when the id is truncated. Small icon button, no extra visual weight.
4. **Sanity check:** the reporter's example starts with `.` and has the shape of an execute-bead worktree name. Worth briefly checking whether something is producing such a value as a bead id (worktree name leaking into bead creation?) — but that's a separate investigation. The UI still needs to degrade gracefully regardless.

## Out of scope

- Changing bead id generation or enforcing max length on ids.
- Redesign of the detail-panel header layout.
    </description>
    <acceptance>
**User story:** As a developer opening a bead with a long id, the id truncates cleanly without overlapping any action button, the full id is discoverable on hover and copyable with one click, and the header buttons remain fully clickable.

**Acceptance criteria:**

1. The bead-id span in `BeadDetail.svelte` uses `truncate min-w-0` (or equivalent) instead of `shrink-0`. The parent flex container keeps `min-w-0`.
2. The span has `title={bead.id}` so hovering reveals the full value.
3. A copy-to-clipboard icon button sits immediately after the id. Clicking copies the full id to the clipboard and surfaces a subtle confirmation (existing toast / inline check mark — whichever matches project conventions).
4. Action buttons (Claim/Unclaim, Edit, Delete, Close) remain fully visible and clickable regardless of id length. Status pill and owner chip remain visible; they may wrap below the id on narrow viewports but never overlap the action group.
5. Playwright e2e:
   - Seeds a bead with a 60-char id (e.g. `.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1`) and opens its detail panel.
   - Asserts the id element has a truncation class and that its rendered width is ≤ the parent container width minus the action-button group width.
   - Asserts the Delete button is clickable (`.click()` does not throw) and that its bounding rect does not intersect the id span's bounding rect.
   - Asserts hovering the id shows a tooltip (or reveals `title` attribute) with the full value, and clicking the copy button writes the full id to the clipboard.
6. Unit/visual: at `max-w-xl` width the header renders without horizontal scroll and without clipped action buttons for ids of length 8, 32, 60, and 120.
    </acceptance>
    <labels>feat-008, feat-004, ui, css</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a3a9887a4a5b098e2e986dff13d3b514c7b476f5">
commit a3a9887a4a5b098e2e986dff13d3b514c7b476f5
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri Apr 24 02:48:32 2026 -0400

    chore: add execution evidence [20260424T064449-]

diff --git a/.ddx/executions/20260424T064449-69011fdb/manifest.json b/.ddx/executions/20260424T064449-69011fdb/manifest.json
new file mode 100644
index 00000000..7edebcb9
--- /dev/null
+++ b/.ddx/executions/20260424T064449-69011fdb/manifest.json
@@ -0,0 +1,130 @@
+{
+  "attempt_id": "20260424T064449-69011fdb",
+  "bead_id": "ddx-71fbe968",
+  "base_rev": "20500643753015bed960be5ef4be8a385e8d5000",
+  "created_at": "2026-04-24T06:44:50.27595437Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-71fbe968",
+    "title": "Bead detail header: long bead ids overlap action buttons (shrink-0 prevents truncation)",
+    "description": "## Observed\n\nOn the bead detail panel, a long bead id overflows and visually overlaps the Claim/Unclaim/Edit/Delete/Close buttons on the right. Reporter example: `.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1` (~60 chars).\n\n## Root cause\n\n`cli/internal/server/frontend/src/lib/components/BeadDetail.svelte:211-221` —\n\n```svelte\n\u003cdiv class=\"flex shrink-0 items-center justify-between border-b ... px-6 py-4\"\u003e\n  \u003cdiv class=\"flex min-w-0 items-center gap-3\"\u003e\n    \u003cspan class=\"shrink-0 font-mono text-xs ...\"\u003e{bead.id}\u003c/span\u003e       ← shrink-0 prevents truncation\n    \u003cspan class=\"shrink-0 font-medium ...\"\u003e{bead.status}\u003c/span\u003e\n    {#if bead.owner}\n      \u003cspan class=\"truncate text-xs ...\"\u003e@ {bead.owner}\u003c/span\u003e\n    {/if}\n  \u003c/div\u003e\n  \u003cdiv class=\"ml-3 flex shrink-0 items-center gap-2\"\u003e\n    [Claim/Unclaim] [Edit] [Delete] [Close]\n  \u003c/div\u003e\n\u003c/div\u003e\n```\n\nThe bead-id span has `shrink-0`, so it never truncates regardless of length. The outer panel is `max-w-xl` (576px). A 60-char monospace id occupies roughly 400-500px on its own; combined with the status pill, owner chip, and the action-button group (also `shrink-0`), it pushes past the available width. Flex layout collapses by overlapping rather than wrapping, so the id draws on top of the buttons.\n\n## Proposed direction\n\n1. **Replace `shrink-0` on the bead-id span with `truncate min-w-0`.** The parent `\u003cdiv class=\"flex min-w-0 items-center gap-3\"\u003e` already establishes a shrinkable container.\n2. **Add `title={bead.id}` so hovering shows the full value** (important — truncating monospace ids from the right loses the distinguishing suffix).\n3. **Add a copy-to-clipboard icon button** next to the id so operators can grab the full value with one click when the id is truncated. Small icon button, no extra visual weight.\n4. **Sanity check:** the reporter's example starts with `.` and has the shape of an execute-bead worktree name. Worth briefly checking whether something is producing such a value as a bead id (worktree name leaking into bead creation?) — but that's a separate investigation. The UI still needs to degrade gracefully regardless.\n\n## Out of scope\n\n- Changing bead id generation or enforcing max length on ids.\n- Redesign of the detail-panel header layout.",
+    "acceptance": "**User story:** As a developer opening a bead with a long id, the id truncates cleanly without overlapping any action button, the full id is discoverable on hover and copyable with one click, and the header buttons remain fully clickable.\n\n**Acceptance criteria:**\n\n1. The bead-id span in `BeadDetail.svelte` uses `truncate min-w-0` (or equivalent) instead of `shrink-0`. The parent flex container keeps `min-w-0`.\n2. The span has `title={bead.id}` so hovering reveals the full value.\n3. A copy-to-clipboard icon button sits immediately after the id. Clicking copies the full id to the clipboard and surfaces a subtle confirmation (existing toast / inline check mark — whichever matches project conventions).\n4. Action buttons (Claim/Unclaim, Edit, Delete, Close) remain fully visible and clickable regardless of id length. Status pill and owner chip remain visible; they may wrap below the id on narrow viewports but never overlap the action group.\n5. Playwright e2e:\n   - Seeds a bead with a 60-char id (e.g. `.execute-bead-wt-ddx-0a651925-20260418T043148-1346e8a3-526efaf1`) and opens its detail panel.\n   - Asserts the id element has a truncation class and that its rendered width is ≤ the parent container width minus the action-button group width.\n   - Asserts the Delete button is clickable (`.click()` does not throw) and that its bounding rect does not intersect the id span's bounding rect.\n   - Asserts hovering the id shows a tooltip (or reveals `title` attribute) with the full value, and clicking the copy button writes the full id to the clipboard.\n6. Unit/visual: at `max-w-xl` width the header renders without horizontal scroll and without clipped action buttons for ids of length 8, 32, 60, and 120.",
+    "labels": [
+      "feat-008",
+      "feat-004",
+      "ui",
+      "css"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-24T06:44:49Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"omlx-vidar-1235\",\"resolved_model\":\"qwen/qwen3.6-35b-a3b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:02:21.566954818Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=omlx-vidar-1235 model=qwen/qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen/qwen3.6-35b-a3b probe=ok\nagent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen/qwen3.6-35b-a3b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-22T21:02:21.770166278Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"codex/gpt-5.4\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:02:24.380876847Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=codex/gpt-5.4"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness=claude model=codex/gpt-5.4 probe=ok\nunsupported model \"codex/gpt-5.4\" for harness \"claude\"; supported models: sonnet, opus, claude-sonnet-4-6",
+          "created_at": "2026-04-22T21:02:24.576133818Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"gemini\",\"resolved_model\":\"minimax/minimax-m2.7\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:02:27.128089387Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=gemini model=minimax/minimax-m2.7"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness=gemini model=minimax/minimax-m2.7 probe=ok\nunsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+          "created_at": "2026-04-22T21:02:27.328867059Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen/qwen3.6-35b-a3b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2016},{\"tier\":\"standard\",\"harness\":\"claude\",\"model\":\"codex/gpt-5.4\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2012},{\"tier\":\"smart\",\"harness\":\"gemini\",\"model\":\"minimax/minimax-m2.7\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2011}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-22T21:02:27.389846764Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=3 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite\ntier=smart\nprobe_result=ok\nresult_rev=4a8cb64f59bb73d590e16886afed2e8ffcc96ef3\nbase_rev=4a8cb64f59bb73d590e16886afed2e8ffcc96ef3\nretry_after=2026-04-23T03:02:27Z",
+          "created_at": "2026-04-22T21:02:27.563433391Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-04-23T06:28:53.378084824Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-04-23T06:29:57.750279403Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "pre-execute-bead checkpoint: staging changes: exit status 128",
+          "created_at": "2026-04-23T07:19:48.296639241Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-24T06:44:49.779780134Z",
+      "execute-loop-last-detail": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+      "execute-loop-last-status": "execution_failed",
+      "feature": "FEAT-008"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260424T064449-69011fdb",
+    "prompt": ".ddx/executions/20260424T064449-69011fdb/prompt.md",
+    "manifest": ".ddx/executions/20260424T064449-69011fdb/manifest.json",
+    "result": ".ddx/executions/20260424T064449-69011fdb/result.json",
+    "checks": ".ddx/executions/20260424T064449-69011fdb/checks.json",
+    "usage": ".ddx/executions/20260424T064449-69011fdb/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-71fbe968-20260424T064449-69011fdb"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260424T064449-69011fdb/result.json b/.ddx/executions/20260424T064449-69011fdb/result.json
new file mode 100644
index 00000000..0d8f69b4
--- /dev/null
+++ b/.ddx/executions/20260424T064449-69011fdb/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-71fbe968",
+  "attempt_id": "20260424T064449-69011fdb",
+  "base_rev": "20500643753015bed960be5ef4be8a385e8d5000",
+  "result_rev": "4d32b893a63c84d3315e2454b5f217a824250b28",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-78023e02",
+  "duration_ms": 221122,
+  "tokens": 12146,
+  "cost_usd": 1.6262872500000005,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260424T064449-69011fdb",
+  "prompt_file": ".ddx/executions/20260424T064449-69011fdb/prompt.md",
+  "manifest_file": ".ddx/executions/20260424T064449-69011fdb/manifest.json",
+  "result_file": ".ddx/executions/20260424T064449-69011fdb/result.json",
+  "usage_file": ".ddx/executions/20260424T064449-69011fdb/usage.json",
+  "started_at": "2026-04-24T06:44:50.276340997Z",
+  "finished_at": "2026-04-24T06:48:31.39846691Z"
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
## Review: ddx-71fbe968 iter 1

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

<bead-review>
  <bead id="ddx-4cecb7d5" iter=1>
    <title>FEAT-008 P9: US-098 plugins list + detail + install/update/uninstall</title>
    <description>
plugins/+page.svelte registry cards (name, version, type, description, status badge). plugins/[name]/+page.svelte manifest region renders as literal YAML so 'name: helix' / 'version: 1.4.2' appear in DOM. Skills, prompts, templates regions. Install dialog: scope radio (global|project), disk estimate with UNITS ('800 KB' not raw bytes); confirm → pluginDispatch({action: 'install', scope}). Uninstall: ConfirmDialog → pluginDispatch({action: 'uninstall'}). Update card shows both versions; action → pluginDispatch({action: 'update'}). Commit screenshots.
    </description>
    <acceptance>
e2e/plugins.spec.ts 4/4 green; manifest in YAML form; screenshots committed.
    </acceptance>
    <labels>ui, feat-008, phase-9, us-098</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1dbbaa5482499b20803daed484b845a39ee28f41">
commit 1dbbaa5482499b20803daed484b845a39ee28f41
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 05:14:55 2026 -0400

    chore: add execution evidence [20260422T085958-]

diff --git a/.ddx/executions/20260422T085958-39f3e3e4/result.json b/.ddx/executions/20260422T085958-39f3e3e4/result.json
new file mode 100644
index 00000000..b3d37c22
--- /dev/null
+++ b/.ddx/executions/20260422T085958-39f3e3e4/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-4cecb7d5",
+  "attempt_id": "20260422T085958-39f3e3e4",
+  "base_rev": "24d5e66bc568c2685bb1a2216e40cce03fd1518b",
+  "result_rev": "9f0fae69ec4c70e443812413e3c2c4934de4ff8c",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-68fcf300",
+  "duration_ms": 895482,
+  "tokens": 10788049,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T085958-39f3e3e4",
+  "prompt_file": ".ddx/executions/20260422T085958-39f3e3e4/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T085958-39f3e3e4/manifest.json",
+  "result_file": ".ddx/executions/20260422T085958-39f3e3e4/result.json",
+  "usage_file": ".ddx/executions/20260422T085958-39f3e3e4/usage.json",
+  "started_at": "2026-04-22T08:59:59.18159646Z",
+  "finished_at": "2026-04-22T09:14:54.664515404Z"
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
## Review: ddx-4cecb7d5 iter 1

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

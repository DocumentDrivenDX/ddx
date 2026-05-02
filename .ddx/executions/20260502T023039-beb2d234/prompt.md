<bead-review>
  <bead id="ddx-5b3629fb" iter=1>
    <title>Rewrite docs/helix/01-frame/principles.md with 10 domain principles + software-factory preface</title>
    <description>
Replace existing 8 principles in docs/helix/01-frame/principles.md with the 10 locked domain principles. Add preface paragraph framing them as 'factory-floor design choices given that DDx is a document-driven software factory.' Each principle: headline, supporting paragraph (~150 words), DDx response, ddx.depends_on linking to RSCH-NNN. Cross-reference at top: 'For internal engineering principles, see docs/dev/engineering-principles.md.' Locked headlines: 1) Spec-first development. 2) Executable specifications. 3) Audit trail required. 4) Context is king. 5) Work is a DAG. 6) Right-size the model. 7) Avoid vendor lock-in. 8) Drift is debt. 9) Least privilege for agents. 10) Inspect and adapt.
    </description>
    <acceptance>
1. principles.md has all 10 headline principles with paragraphs and DDx response sections. 2. Software-factory preface paragraph added. 3. Cross-link to engineering-principles.md present. 4. ddx.depends_on lists RSCH-001 through RSCH-010 across the principles. 5. 'grep -c "^### " docs/helix/01-frame/principles.md' returns 10. 6. No reference to Dun.
    </acceptance>
    <labels>site-redesign, area:specs, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="afabb4cd9ae652c01a5be9dae9081166b6f8a87f">
commit afabb4cd9ae652c01a5be9dae9081166b6f8a87f
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:30:37 2026 -0400

    chore: add execution evidence [20260502T022846-]

diff --git a/.ddx/executions/20260502T022846-c148a5a8/manifest.json b/.ddx/executions/20260502T022846-c148a5a8/manifest.json
new file mode 100644
index 00000000..590c5973
--- /dev/null
+++ b/.ddx/executions/20260502T022846-c148a5a8/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T022846-c148a5a8",
+  "bead_id": "ddx-5b3629fb",
+  "base_rev": "b3f9bcfee15f6c593297a6dcf9baa0cc5889c8f5",
+  "created_at": "2026-05-02T02:28:47.970617193Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-5b3629fb",
+    "title": "Rewrite docs/helix/01-frame/principles.md with 10 domain principles + software-factory preface",
+    "description": "Replace existing 8 principles in docs/helix/01-frame/principles.md with the 10 locked domain principles. Add preface paragraph framing them as 'factory-floor design choices given that DDx is a document-driven software factory.' Each principle: headline, supporting paragraph (~150 words), DDx response, ddx.depends_on linking to RSCH-NNN. Cross-reference at top: 'For internal engineering principles, see docs/dev/engineering-principles.md.' Locked headlines: 1) Spec-first development. 2) Executable specifications. 3) Audit trail required. 4) Context is king. 5) Work is a DAG. 6) Right-size the model. 7) Avoid vendor lock-in. 8) Drift is debt. 9) Least privilege for agents. 10) Inspect and adapt.",
+    "acceptance": "1. principles.md has all 10 headline principles with paragraphs and DDx response sections. 2. Software-factory preface paragraph added. 3. Cross-link to engineering-principles.md present. 4. ddx.depends_on lists RSCH-001 through RSCH-010 across the principles. 5. 'grep -c \"^### \" docs/helix/01-frame/principles.md' returns 10. 6. No reference to Dun.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:specs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:28:46Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:28:46.561066298Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T022846-c148a5a8",
+    "prompt": ".ddx/executions/20260502T022846-c148a5a8/prompt.md",
+    "manifest": ".ddx/executions/20260502T022846-c148a5a8/manifest.json",
+    "result": ".ddx/executions/20260502T022846-c148a5a8/result.json",
+    "checks": ".ddx/executions/20260502T022846-c148a5a8/checks.json",
+    "usage": ".ddx/executions/20260502T022846-c148a5a8/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-5b3629fb-20260502T022846-c148a5a8"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T022846-c148a5a8/result.json b/.ddx/executions/20260502T022846-c148a5a8/result.json
new file mode 100644
index 00000000..ca8585bc
--- /dev/null
+++ b/.ddx/executions/20260502T022846-c148a5a8/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-5b3629fb",
+  "attempt_id": "20260502T022846-c148a5a8",
+  "base_rev": "b3f9bcfee15f6c593297a6dcf9baa0cc5889c8f5",
+  "result_rev": "b0cca464c0cc551abb09ee5fe1fbdabdece748d6",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-6bf87702",
+  "duration_ms": 107239,
+  "tokens": 6131,
+  "cost_usd": 0.454329,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T022846-c148a5a8",
+  "prompt_file": ".ddx/executions/20260502T022846-c148a5a8/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T022846-c148a5a8/manifest.json",
+  "result_file": ".ddx/executions/20260502T022846-c148a5a8/result.json",
+  "usage_file": ".ddx/executions/20260502T022846-c148a5a8/usage.json",
+  "started_at": "2026-05-02T02:28:47.970905901Z",
+  "finished_at": "2026-05-02T02:30:35.210587951Z"
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
## Review: ddx-5b3629fb iter 1

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

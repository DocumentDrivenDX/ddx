<bead-review>
  <bead id="ddx-4dab2bfd" iter=1>
    <title>Create website/content/docs/concepts/software-factory.md explaining the metaphor</title>
    <description>
Create website/content/docs/concepts/software-factory.md explaining: what 'software factory' means, what DDx inherits from the 2000s Microsoft tradition (Greenfield/Short), what the 2026 spec-driven movement adds (spec-kit, Kiro), what DDx specifically adds (bounded contexts, evidence on disk, project-local install), what DDx intentionally does not do (methodology lock-in). Cross-link to REF-007, REF-005, REF-006.
    </description>
    <acceptance>
1. concepts/software-factory.md exists. 2. Contains sections: definition, lineage, what DDx inherits, what DDx adds, what DDx avoids. 3. Cross-links to relevant REF artifacts. 4. cd website &amp;&amp; hugo builds successfully.
    </acceptance>
    <labels>site-redesign, area:website, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="f75dbac607e3520ee884848f92bee3e989328f06">
commit f75dbac607e3520ee884848f92bee3e989328f06
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:45:08 2026 -0400

    chore: add execution evidence [20260502T024258-]

diff --git a/.ddx/executions/20260502T024258-a514ed7d/manifest.json b/.ddx/executions/20260502T024258-a514ed7d/manifest.json
new file mode 100644
index 00000000..751e1fc5
--- /dev/null
+++ b/.ddx/executions/20260502T024258-a514ed7d/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T024258-a514ed7d",
+  "bead_id": "ddx-4dab2bfd",
+  "base_rev": "c8b838a872d9d2e6e26ac53369d2fb3d4ad6ecde",
+  "created_at": "2026-05-02T02:43:00.206019859Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-4dab2bfd",
+    "title": "Create website/content/docs/concepts/software-factory.md explaining the metaphor",
+    "description": "Create website/content/docs/concepts/software-factory.md explaining: what 'software factory' means, what DDx inherits from the 2000s Microsoft tradition (Greenfield/Short), what the 2026 spec-driven movement adds (spec-kit, Kiro), what DDx specifically adds (bounded contexts, evidence on disk, project-local install), what DDx intentionally does not do (methodology lock-in). Cross-link to REF-007, REF-005, REF-006.",
+    "acceptance": "1. concepts/software-factory.md exists. 2. Contains sections: definition, lineage, what DDx inherits, what DDx adds, what DDx avoids. 3. Cross-links to relevant REF artifacts. 4. cd website \u0026\u0026 hugo builds successfully.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:website",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:42:58Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:42:58.826441909Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T024258-a514ed7d",
+    "prompt": ".ddx/executions/20260502T024258-a514ed7d/prompt.md",
+    "manifest": ".ddx/executions/20260502T024258-a514ed7d/manifest.json",
+    "result": ".ddx/executions/20260502T024258-a514ed7d/result.json",
+    "checks": ".ddx/executions/20260502T024258-a514ed7d/checks.json",
+    "usage": ".ddx/executions/20260502T024258-a514ed7d/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-4dab2bfd-20260502T024258-a514ed7d"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T024258-a514ed7d/result.json b/.ddx/executions/20260502T024258-a514ed7d/result.json
new file mode 100644
index 00000000..b9d295ac
--- /dev/null
+++ b/.ddx/executions/20260502T024258-a514ed7d/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-4dab2bfd",
+  "attempt_id": "20260502T024258-a514ed7d",
+  "base_rev": "c8b838a872d9d2e6e26ac53369d2fb3d4ad6ecde",
+  "result_rev": "36192f9db05151a438d66aa16b568775745ec05e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-15019450",
+  "duration_ms": 124839,
+  "tokens": 6781,
+  "cost_usd": 0.9533327499999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T024258-a514ed7d",
+  "prompt_file": ".ddx/executions/20260502T024258-a514ed7d/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T024258-a514ed7d/manifest.json",
+  "result_file": ".ddx/executions/20260502T024258-a514ed7d/result.json",
+  "usage_file": ".ddx/executions/20260502T024258-a514ed7d/usage.json",
+  "started_at": "2026-05-02T02:43:00.206302359Z",
+  "finished_at": "2026-05-02T02:45:05.045563295Z"
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
## Review: ddx-4dab2bfd iter 1

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

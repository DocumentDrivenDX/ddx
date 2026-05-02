<bead-review>
  <bead id="ddx-377ab11f" iter=1>
    <title>Author ~25 new REF-NNN external-reference artifacts for principle research backing</title>
    <description>
Create REF artifacts under docs/helix/00-discover/references/ for sources cited by RSCH-001..RSCH-010 and the software-factory framing. Required REFs (numbering picks up after migrated REF-001..004): REF-005 GitHub Spec Kit (github.com/github/spec-kit), REF-006 AWS Kiro (kiro.dev), REF-007 Greenfield+Short Software Factories (Microsoft 2004), REF-008 Liu et al Lost in the Middle (TACL 2024), REF-009 Chroma 2025 Context Rot study, REF-010 Sheridan+Verplank 1978 Levels of Automation, REF-011 OWASP LLM Top 10 (2025), REF-012 EchoLeak CVE-2025-32711, REF-013 MAST 2025 Multi-Agent Failure Taxonomy, REF-014 Helland Data on the Outside vs Inside (ACM Queue), REF-015 FrugalGPT (cost-tier routing), REF-016 Self-Refine (NeurIPS 2023), REF-017 Knuth Literate Programming, REF-018 EvalPlus, REF-019 BDD/Cucumber lineage, REF-020 Kleppmann DDIA, REF-021 Fowler event sourcing, REF-022 CPM/PERT (1957), REF-023 Bazel docs (DAG), REF-024 Dask docs (task graphs), REF-025 LSP overview (m+n), REF-026 Saltzer+Schroeder 1975 (least privilege), REF-027 Anthropic Building Effective Agents, REF-028 doc-drift study (Springer 2023), REF-029 multi-model routing 2026 industry consensus, REF-030 OCI/POSIX/SQL portability lineage. Each: minimal frontmatter (id, title, kind=reference, source_url, source_author OR organization, accessed, summary 1-3 sentences, tags). Use the existing docs/resources/spec-driven.md format as a guide.
    </description>
    <acceptance>
1. docs/helix/00-discover/references/ contains REF-005 through REF-030 (~26 new artifacts). 2. Each has the required frontmatter fields. 3. Run 'ls docs/helix/00-discover/references/REF-0*.md | wc -l' returns &gt;= 30 (REF-001..030). 4. ddx doc audit reports no missing-id errors on these artifacts.
    </acceptance>
    <labels>site-redesign, area:specs, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a39f07619764a00d7c9571a836597fe29fa748a9">
commit a39f07619764a00d7c9571a836597fe29fa748a9
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 22:21:26 2026 -0400

    chore: add execution evidence [20260502T021807-]

diff --git a/.ddx/executions/20260502T021807-7bf72c03/manifest.json b/.ddx/executions/20260502T021807-7bf72c03/manifest.json
new file mode 100644
index 00000000..e38c5669
--- /dev/null
+++ b/.ddx/executions/20260502T021807-7bf72c03/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260502T021807-7bf72c03",
+  "bead_id": "ddx-377ab11f",
+  "base_rev": "23b4ae8c1192dd24349d0f26114ad531028f9bf1",
+  "created_at": "2026-05-02T02:18:09.325600646Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-377ab11f",
+    "title": "Author ~25 new REF-NNN external-reference artifacts for principle research backing",
+    "description": "Create REF artifacts under docs/helix/00-discover/references/ for sources cited by RSCH-001..RSCH-010 and the software-factory framing. Required REFs (numbering picks up after migrated REF-001..004): REF-005 GitHub Spec Kit (github.com/github/spec-kit), REF-006 AWS Kiro (kiro.dev), REF-007 Greenfield+Short Software Factories (Microsoft 2004), REF-008 Liu et al Lost in the Middle (TACL 2024), REF-009 Chroma 2025 Context Rot study, REF-010 Sheridan+Verplank 1978 Levels of Automation, REF-011 OWASP LLM Top 10 (2025), REF-012 EchoLeak CVE-2025-32711, REF-013 MAST 2025 Multi-Agent Failure Taxonomy, REF-014 Helland Data on the Outside vs Inside (ACM Queue), REF-015 FrugalGPT (cost-tier routing), REF-016 Self-Refine (NeurIPS 2023), REF-017 Knuth Literate Programming, REF-018 EvalPlus, REF-019 BDD/Cucumber lineage, REF-020 Kleppmann DDIA, REF-021 Fowler event sourcing, REF-022 CPM/PERT (1957), REF-023 Bazel docs (DAG), REF-024 Dask docs (task graphs), REF-025 LSP overview (m+n), REF-026 Saltzer+Schroeder 1975 (least privilege), REF-027 Anthropic Building Effective Agents, REF-028 doc-drift study (Springer 2023), REF-029 multi-model routing 2026 industry consensus, REF-030 OCI/POSIX/SQL portability lineage. Each: minimal frontmatter (id, title, kind=reference, source_url, source_author OR organization, accessed, summary 1-3 sentences, tags). Use the existing docs/resources/spec-driven.md format as a guide.",
+    "acceptance": "1. docs/helix/00-discover/references/ contains REF-005 through REF-030 (~26 new artifacts). 2. Each has the required frontmatter fields. 3. Run 'ls docs/helix/00-discover/references/REF-0*.md | wc -l' returns \u003e= 30 (REF-001..030). 4. ddx doc audit reports no missing-id errors on these artifacts.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:specs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T02:18:07Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "execute-loop-heartbeat-at": "2026-05-02T02:18:07.832554887Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T021807-7bf72c03",
+    "prompt": ".ddx/executions/20260502T021807-7bf72c03/prompt.md",
+    "manifest": ".ddx/executions/20260502T021807-7bf72c03/manifest.json",
+    "result": ".ddx/executions/20260502T021807-7bf72c03/result.json",
+    "checks": ".ddx/executions/20260502T021807-7bf72c03/checks.json",
+    "usage": ".ddx/executions/20260502T021807-7bf72c03/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-377ab11f-20260502T021807-7bf72c03"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T021807-7bf72c03/result.json b/.ddx/executions/20260502T021807-7bf72c03/result.json
new file mode 100644
index 00000000..35d3bb87
--- /dev/null
+++ b/.ddx/executions/20260502T021807-7bf72c03/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-377ab11f",
+  "attempt_id": "20260502T021807-7bf72c03",
+  "base_rev": "23b4ae8c1192dd24349d0f26114ad531028f9bf1",
+  "result_rev": "298a9446e2f46951c57b1af7e401e9d46ec7479e",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-f73a6c03",
+  "duration_ms": 195505,
+  "tokens": 13194,
+  "cost_usd": 1.3714827499999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T021807-7bf72c03",
+  "prompt_file": ".ddx/executions/20260502T021807-7bf72c03/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T021807-7bf72c03/manifest.json",
+  "result_file": ".ddx/executions/20260502T021807-7bf72c03/result.json",
+  "usage_file": ".ddx/executions/20260502T021807-7bf72c03/usage.json",
+  "started_at": "2026-05-02T02:18:09.32591527Z",
+  "finished_at": "2026-05-02T02:21:24.831172951Z"
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
## Review: ddx-377ab11f iter 1

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

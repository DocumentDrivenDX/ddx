<bead-review>
  <bead id="ddx-b9747ee3" iter=1>
    <title>registry: package.yaml schema failure suppresses structural audits</title>
    <description>
When LoadPackageManifest rejects a package.yaml for missing required fields (e.g. description), it returns nil for the package. AuditInstalledEntry then falls back to &amp;Package{}, which has empty Install.Skills. As a result collectSkillRoots returns nothing and auditSkillRoot is never called, so structural problems like missing SKILL.md files are silently swallowed whenever the manifest also has a schema problem.

The two audit dimensions should be independent: a malformed manifest should not blind us to install-tree problems.

Fix sketch: when LoadPackageManifest fails validation but the YAML at least parsed, still construct a *Package from raw (accepting Install etc.) so structural audits can proceed. Alternatively split manifest validation and structure audit so the latter does not depend on a fully valid *Package.

Surfaced while finishing the WIP for c2203cc3 — TestDoctorPluginsFlagReportsSchemaAndSymlinkIssues had to be split into two tests because no single fixture could trigger both diagnostics.
    </description>
    <acceptance>
Doctor reports both manifest schema and structural issues for a single broken plugin in one diagnostic pass. The split tests in cli/cmd/doctor_plugins_test.go can be re-merged into one combined test.
    </acceptance>
    <labels>area:registry, kind:bug, phase:build</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="327eacc2a3608e2cfe1214fc5826acae78c1ce27">
commit 327eacc2a3608e2cfe1214fc5826acae78c1ce27
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sun Apr 19 19:06:15 2026 -0400

    chore: add execution evidence [20260419T225658-]

diff --git a/.ddx/executions/20260419T225658-fe573518/manifest.json b/.ddx/executions/20260419T225658-fe573518/manifest.json
new file mode 100644
index 00000000..1814943c
--- /dev/null
+++ b/.ddx/executions/20260419T225658-fe573518/manifest.json
@@ -0,0 +1,55 @@
+{
+  "attempt_id": "20260419T225658-fe573518",
+  "bead_id": "ddx-b9747ee3",
+  "base_rev": "eca4845f4e778434d065bef0a15963428fb9e812",
+  "created_at": "2026-04-19T22:56:58.899901345Z",
+  "requested": {
+    "harness": "agent",
+    "model": "minimax/minimax-m2.7",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b9747ee3",
+    "title": "registry: package.yaml schema failure suppresses structural audits",
+    "description": "When LoadPackageManifest rejects a package.yaml for missing required fields (e.g. description), it returns nil for the package. AuditInstalledEntry then falls back to \u0026Package{}, which has empty Install.Skills. As a result collectSkillRoots returns nothing and auditSkillRoot is never called, so structural problems like missing SKILL.md files are silently swallowed whenever the manifest also has a schema problem.\n\nThe two audit dimensions should be independent: a malformed manifest should not blind us to install-tree problems.\n\nFix sketch: when LoadPackageManifest fails validation but the YAML at least parsed, still construct a *Package from raw (accepting Install etc.) so structural audits can proceed. Alternatively split manifest validation and structure audit so the latter does not depend on a fully valid *Package.\n\nSurfaced while finishing the WIP for c2203cc3 — TestDoctorPluginsFlagReportsSchemaAndSymlinkIssues had to be split into two tests because no single fixture could trigger both diagnostics.",
+    "acceptance": "Doctor reports both manifest schema and structural issues for a single broken plugin in one diagnostic pass. The split tests in cli/cmd/doctor_plugins_test.go can be re-merged into one combined test.",
+    "labels": [
+      "area:registry",
+      "kind:bug",
+      "phase:build"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-19T22:56:41Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "721226",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"lmstudio\",\"resolved_model\":\"qwen3.5-27b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-19T22:56:50.283609232Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=lmstudio model=qwen3.5-27b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=lmstudio model=qwen3.5-27b probe=ok\nagent: native config provider \"lmstudio\": config: unknown provider \"lmstudio\"",
+          "created_at": "2026-04-19T22:56:50.469185336Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-19T22:56:41.364088838Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260419T225658-fe573518",
+    "prompt": ".ddx/executions/20260419T225658-fe573518/prompt.md",
+    "manifest": ".ddx/executions/20260419T225658-fe573518/manifest.json",
+    "result": ".ddx/executions/20260419T225658-fe573518/result.json",
+    "checks": ".ddx/executions/20260419T225658-fe573518/checks.json",
+    "usage": ".ddx/executions/20260419T225658-fe573518/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b9747ee3-20260419T225658-fe573518"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260419T225658-fe573518/result.json b/.ddx/executions/20260419T225658-fe573518/result.json
new file mode 100644
index 00000000..6a0a80a6
--- /dev/null
+++ b/.ddx/executions/20260419T225658-fe573518/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-b9747ee3",
+  "attempt_id": "20260419T225658-fe573518",
+  "base_rev": "eca4845f4e778434d065bef0a15963428fb9e812",
+  "result_rev": "9a04044e298c14506b7011b6fdf5c6b2516e5c17",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "agent",
+  "model": "minimax/minimax-m2.7-20260318",
+  "session_id": "eb-a1c1ad19",
+  "duration_ms": 555565,
+  "tokens": 2672128,
+  "cost_usd": 0.26069878,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260419T225658-fe573518",
+  "prompt_file": ".ddx/executions/20260419T225658-fe573518/prompt.md",
+  "manifest_file": ".ddx/executions/20260419T225658-fe573518/manifest.json",
+  "result_file": ".ddx/executions/20260419T225658-fe573518/result.json",
+  "usage_file": ".ddx/executions/20260419T225658-fe573518/usage.json",
+  "started_at": "2026-04-19T22:56:58.900176387Z",
+  "finished_at": "2026-04-19T23:06:14.466006751Z"
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
## Review: ddx-b9747ee3 iter 1

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

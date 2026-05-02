<bead-review>
  <bead id="ddx-94311dcf" iter=1>
    <title>Update FEAT-005 to register research and reference artifact kinds</title>
    <description>
Add 'research' (RSCH-NNN, docs/helix/00-discover/research/) and 'reference' (REF-NNN, docs/helix/00-discover/references/) to FEAT-005's Common Artifact Types table. Per codex review: docgraph parser doesn't validate prefixes today (cli/internal/docgraph/frontmatter.go:12), so this is docs-only. Identity (id, depends_on) lives under ddx: block; REF metadata fields (source_url, accessed, source_author) are accepted as inert by current parser.
    </description>
    <acceptance>
1. docs/helix/01-frame/features/FEAT-005-artifacts.md adds 'research' and 'reference' rows to Common Artifact Types table with locations and ID prefixes. 2. Frontmatter schema example shows REF metadata fields outside ddx: block. 3. No CLI code change needed (verify by running 'cd cli &amp;&amp; go test ./internal/docgraph/...' — should pass without modification). 4. Run 'rg -n RSCH-NNN docs/helix/01-frame/features/FEAT-005-artifacts.md' returns at least one match.
    </acceptance>
    <labels>site-redesign, area:specs, kind:doc</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="03605769e5fe0616c972b2d54f04ef9e0675cc9c">
commit 03605769e5fe0616c972b2d54f04ef9e0675cc9c
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Fri May 1 21:59:02 2026 -0400

    chore: add execution evidence [20260502T015811-]

diff --git a/.ddx/executions/20260502T015811-e4f4eb12/manifest.json b/.ddx/executions/20260502T015811-e4f4eb12/manifest.json
new file mode 100644
index 00000000..9093b394
--- /dev/null
+++ b/.ddx/executions/20260502T015811-e4f4eb12/manifest.json
@@ -0,0 +1,71 @@
+{
+  "attempt_id": "20260502T015811-e4f4eb12",
+  "bead_id": "ddx-94311dcf",
+  "base_rev": "e8b57aecb5a4f25b0ae19b7f9859287a3e1fb1a8",
+  "created_at": "2026-05-02T01:58:13.788049045Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-94311dcf",
+    "title": "Update FEAT-005 to register research and reference artifact kinds",
+    "description": "Add 'research' (RSCH-NNN, docs/helix/00-discover/research/) and 'reference' (REF-NNN, docs/helix/00-discover/references/) to FEAT-005's Common Artifact Types table. Per codex review: docgraph parser doesn't validate prefixes today (cli/internal/docgraph/frontmatter.go:12), so this is docs-only. Identity (id, depends_on) lives under ddx: block; REF metadata fields (source_url, accessed, source_author) are accepted as inert by current parser.",
+    "acceptance": "1. docs/helix/01-frame/features/FEAT-005-artifacts.md adds 'research' and 'reference' rows to Common Artifact Types table with locations and ID prefixes. 2. Frontmatter schema example shows REF metadata fields outside ddx: block. 3. No CLI code change needed (verify by running 'cd cli \u0026\u0026 go test ./internal/docgraph/...' — should pass without modification). 4. Run 'rg -n RSCH-NNN docs/helix/01-frame/features/FEAT-005-artifacts.md' returns at least one match.",
+    "parent": "ddx-629ec5b4",
+    "labels": [
+      "site-redesign",
+      "area:specs",
+      "kind:doc"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T01:58:11Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1170049",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:39.304567091Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-05-02T01:36:39.428036406Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-05-02T01:36:39.538559236Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "erik",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-05-02T01:36:39.754391444Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-02T01:58:11.081518935Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T015811-e4f4eb12",
+    "prompt": ".ddx/executions/20260502T015811-e4f4eb12/prompt.md",
+    "manifest": ".ddx/executions/20260502T015811-e4f4eb12/manifest.json",
+    "result": ".ddx/executions/20260502T015811-e4f4eb12/result.json",
+    "checks": ".ddx/executions/20260502T015811-e4f4eb12/checks.json",
+    "usage": ".ddx/executions/20260502T015811-e4f4eb12/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-94311dcf-20260502T015811-e4f4eb12"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T015811-e4f4eb12/result.json b/.ddx/executions/20260502T015811-e4f4eb12/result.json
new file mode 100644
index 00000000..c42d0cb7
--- /dev/null
+++ b/.ddx/executions/20260502T015811-e4f4eb12/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-94311dcf",
+  "attempt_id": "20260502T015811-e4f4eb12",
+  "base_rev": "e8b57aecb5a4f25b0ae19b7f9859287a3e1fb1a8",
+  "result_rev": "1920801c72aacc3be622cec0f0afd3af8254f572",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-b1aabff9",
+  "duration_ms": 44815,
+  "tokens": 2655,
+  "cost_usd": 0.39148025,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T015811-e4f4eb12",
+  "prompt_file": ".ddx/executions/20260502T015811-e4f4eb12/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T015811-e4f4eb12/manifest.json",
+  "result_file": ".ddx/executions/20260502T015811-e4f4eb12/result.json",
+  "usage_file": ".ddx/executions/20260502T015811-e4f4eb12/usage.json",
+  "started_at": "2026-05-02T01:58:13.788614336Z",
+  "finished_at": "2026-05-02T01:58:58.6043107Z"
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
## Review: ddx-94311dcf iter 1

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

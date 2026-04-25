<bead-review>
  <bead id="ddx-3984a48e" iter=1>
    <title>migrate remaining cli/cmd/* to git.Command (Phase 2b)</title>
    <description>
Migrate remaining cli/cmd/ git callsites to git.Command. Excludes install.go which is in B5.

Files: agent_cmd.go, agent_workers.go, checkpoint.go, status.go, bead.go, doc.go, bead_review.go, log.go.

Each: replace exec.Command("git", args...) with git.Command(ctx, dir, args...). Pass appropriate dir (cmd.Dir value, or '-C &lt;path&gt;' arg target).
    </description>
    <acceptance>
No bare exec.Command("git"...) in cli/cmd/ except in test helpers. All existing cli/cmd/ tests pass.
    </acceptance>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="cc1ddd7b893ca9927a2621ae10524c7433809ca1">
commit cc1ddd7b893ca9927a2621ae10524c7433809ca1
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 25 06:11:38 2026 -0400

    chore: add execution evidence [20260425T100318-]

diff --git a/.ddx/executions/20260425T100318-82da6e40/manifest.json b/.ddx/executions/20260425T100318-82da6e40/manifest.json
new file mode 100644
index 00000000..51baa87d
--- /dev/null
+++ b/.ddx/executions/20260425T100318-82da6e40/manifest.json
@@ -0,0 +1,115 @@
+{
+  "attempt_id": "20260425T100318-82da6e40",
+  "bead_id": "ddx-3984a48e",
+  "base_rev": "a8b09f69dc9cfb48be368512f0d230d857afd1fd",
+  "created_at": "2026-04-25T10:03:19.397896761Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-3984a48e",
+    "title": "migrate remaining cli/cmd/* to git.Command (Phase 2b)",
+    "description": "Migrate remaining cli/cmd/ git callsites to git.Command. Excludes install.go which is in B5.\n\nFiles: agent_cmd.go, agent_workers.go, checkpoint.go, status.go, bead.go, doc.go, bead_review.go, log.go.\n\nEach: replace exec.Command(\"git\", args...) with git.Command(ctx, dir, args...). Pass appropriate dir (cmd.Dir value, or '-C \u003cpath\u003e' arg target).",
+    "acceptance": "No bare exec.Command(\"git\"...) in cli/cmd/ except in test helpers. All existing cli/cmd/ tests pass.",
+    "parent": "ddx-64ac553a",
+    "metadata": {
+      "blocked-by": "ddx-aa8a5fb3",
+      "claimed-at": "2026-04-25T10:03:18Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "196235",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-24T03:43:20.898782913Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260424T033612-690dec51\",\"harness\":\"claude\",\"input_tokens\":84,\"output_tokens\":22875,\"total_tokens\":22959,\"cost_usd\":3.933810749999999,\"duration_ms\":427720,\"exit_code\":0}",
+          "created_at": "2026-04-24T03:43:20.972888215Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=22959 cost_usd=3.9338"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-04-24T03:43:23.582865192Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "REQUEST_CHANGES\n`cli/cmd/init.go:307` — `gitAdd := exec.Command(\"git\", \"add\", ...)` not migrated to `gitpkg.Command`.\nartifact: .ddx/executions/20260424T033612-690dec51/reviewer-stream.log",
+          "created_at": "2026-04-24T03:43:46.699889514Z",
+          "kind": "review",
+          "source": "ddx agent execute-loop",
+          "summary": "REQUEST_CHANGES"
+        },
+        {
+          "actor": "",
+          "body": "",
+          "created_at": "2026-04-24T03:43:46.77343477Z",
+          "kind": "reopen",
+          "source": "",
+          "summary": "review: REQUEST_CHANGES"
+        },
+        {
+          "actor": "ddx",
+          "body": "post-merge review: REQUEST_CHANGES\n`cli/cmd/init.go:307` — `gitAdd := exec.Command(\"git\", \"add\", ...)` not migrated to `gitpkg.Command`.\n`cli/cmd/init.go:314` — `gitCommit := exec.Command(\"git\", \"commit\", ...)` not migrated.\n`cli/cmd/init.go:326` — `gitWorktreeCfg := exec.Command(\"git\", \"config\", ...)` not migrated.\n`cli/cmd/init.go:711` — `gitCmd := exec.Command(\"git\", \"rev-parse\", \"--git-dir\")` not migrated.\nEither migrate these four callsites in `init.go`, or amend the bead AC/description to explicitly exclude `init.go` alongside `install.go`.\nresult_rev=c9bdf7823a76efb8d97a660bf83ceca4d756dfa4\nbase_rev=73d93746bb31d18103b0905c99ae6f7ffc274b8e",
+          "created_at": "2026-04-24T03:43:46.832588527Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_request_changes"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-25T02:23:33.379722256Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness= model= probe=no viable provider\nno viable harness found",
+          "created_at": "2026-04-25T02:23:33.461942879Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "skipped"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"standard\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0},{\"tier\":\"smart\",\"status\":\"skipped\",\"cost_usd\":0,\"duration_ms\":0}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-25T02:23:33.528959878Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=2 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "execute-loop: all tiers exhausted — no viable provider found",
+          "created_at": "2026-04-25T02:23:33.655422041Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-25T10:03:18.962201708Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260425T100318-82da6e40",
+    "prompt": ".ddx/executions/20260425T100318-82da6e40/prompt.md",
+    "manifest": ".ddx/executions/20260425T100318-82da6e40/manifest.json",
+    "result": ".ddx/executions/20260425T100318-82da6e40/result.json",
+    "checks": ".ddx/executions/20260425T100318-82da6e40/checks.json",
+    "usage": ".ddx/executions/20260425T100318-82da6e40/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-3984a48e-20260425T100318-82da6e40"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260425T100318-82da6e40/result.json b/.ddx/executions/20260425T100318-82da6e40/result.json
new file mode 100644
index 00000000..6fb85a0b
--- /dev/null
+++ b/.ddx/executions/20260425T100318-82da6e40/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-3984a48e",
+  "attempt_id": "20260425T100318-82da6e40",
+  "base_rev": "a8b09f69dc9cfb48be368512f0d230d857afd1fd",
+  "result_rev": "72212ce9e37f1e2fa96caa7403f15971f267d710",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-98e55c14",
+  "duration_ms": 497700,
+  "tokens": 14304,
+  "cost_usd": 2.40514675,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260425T100318-82da6e40",
+  "prompt_file": ".ddx/executions/20260425T100318-82da6e40/prompt.md",
+  "manifest_file": ".ddx/executions/20260425T100318-82da6e40/manifest.json",
+  "result_file": ".ddx/executions/20260425T100318-82da6e40/result.json",
+  "usage_file": ".ddx/executions/20260425T100318-82da6e40/usage.json",
+  "started_at": "2026-04-25T10:03:19.39823147Z",
+  "finished_at": "2026-04-25T10:11:37.098622306Z"
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
## Review: ddx-3984a48e iter 1

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

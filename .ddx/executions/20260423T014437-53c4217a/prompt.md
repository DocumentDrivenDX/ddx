<bead-review>
  <bead id="ddx-05b4cc9d" iter=1>
    <title>workersByProject compares project id to project path: per-project workers view always empty</title>
    <description>
## Observed

User clicks "Drain queue" on the project home page. Mutation returns success with a worker id. Navigating to `/nodes/.../projects/&lt;id&gt;/workers` shows no workers — an empty table.

Verified on disk: the worker record exists and is running.

```
$ cat .ddx/workers/worker-20260422T205846-f23f/status.json
{
  "id": "worker-20260422T205846-f23f",
  "kind": "execute-loop",
  "state": "running",
  "project_root": "/Users/erik/Projects/ddx",
  ...
}
```

So the dispatch worked, but the per-project workers list filters it out.

## Root cause

`cli/internal/server/state_graphql.go:156` in `GetWorkersGraphQL`:

```go
if projectID != "" &amp;&amp; rec.ProjectRoot != projectID {
    continue
}
```

- `projectID` is the GraphQL argument — a project **id** like `proj-96d7ea83`.
- `rec.ProjectRoot` is the worker record field — a project **path** like `/Users/erik/Projects/ddx`.

These are different representations and never match. With any non-empty `projectID`, every worker is filtered out.

The global `workers` resolver (`resolver_agent.go:9`) passes `""` and works correctly. Only the per-project path (`workersByProject` → `GetWorkersGraphQL(projectID)`) is broken. The user's flow (dispatch from project home → navigate to project workers) hits exactly this path.

Note: `GetBeadSnapshots` (state_graphql.go:49-105) handles the same concept correctly by iterating `s.Projects` and filtering on `proj.ID != projectID` — comparing id to id. The worker resolver does not use that pattern.

## Proposed direction

Two options; prefer (1) for minimal change, (2) for long-term clarity.

### Option 1 — Resolve id → path at the filter site

```go
expectedPath := ""
if projectID != "" {
    proj, ok := s.GetProjectByID(projectID)
    if !ok {
        return nil // unknown project id → no workers
    }
    expectedPath = proj.Path
}
...
if expectedPath != "" &amp;&amp; rec.ProjectRoot != expectedPath {
    continue
}
```

### Option 2 — Store project id on the worker record

Add `ProjectID string` to `WorkerRecord`. Populate at creation time in `WorkerManager.StartExecuteLoop` via `s.GetProjectByPath(effectiveRoot)`. Filter on `rec.ProjectID != projectID`. Cleaner — the worker record carries its own identity and the query doesn't depend on runtime resolution. Requires a tiny migration (for older records missing the field, fall back to path-resolve as in Option 1).

Recommend Option 2. It removes the representation mismatch for good and is cheap.

## Scope note

This is a one-line functional bug with outsized impact: it's the reason the Workers page looks broken whenever used as the dashboard intends. Prioritizing P1.

## Out of scope

- Workers pane lifecycle controls (`ddx-69789664`).
- Cross-project workers view — the global `workers` query already handles that.
    </description>
    <acceptance>
**User story:** As an operator, clicking Drain queue on a project home page starts a worker that I can immediately see in that project's Workers view, with its state and current bead updating live.

**Acceptance criteria:**

1. **Root-cause fix.** The `workersByProject(projectID:)` resolver returns all workers whose target project matches the requested id. Both Option 1 (id→path resolution) and Option 2 (store project id on record) satisfy this; implementation chooses one with a one-line justification in the PR description.

2. **Regression test — project-scoped query.**
   - Seed a ServerState with two registered projects (different paths and ids).
   - Start one worker for each project.
   - `workersByProject(projectID: &lt;id-1&gt;)` returns exactly worker 1.
   - `workersByProject(projectID: &lt;id-2&gt;)` returns exactly worker 2.
   - `workers` (global) returns both.
   - Unknown project id returns empty list, not an error.

3. **Regression test — backward compat.**
   - A worker record on disk without a `ProjectID` field (simulating a pre-migration record) is correctly matched by the path-resolution fallback. Only required if Option 2 is chosen.

4. **End-to-end validation.**
   - Playwright: navigate to project home, click Drain, confirm, wait 2s, navigate to Workers tab. Assert exactly one new row with `state: running` and matching worker id.
   - Same test run against a second project asserts that project's Workers tab does not show project-1's worker.

5. **No regression to unrelated surfaces.**
   - Global `workers` query still returns all workers.
   - Worker detail page still loads by worker id.
   - Workers live-progress subscription still patches rows in place.

6. **Cross-reference.**
   - Note on `ddx-69789664` (Workers lifecycle controls): that bead's acceptance assumed Workers pane worked correctly. Point to this bead as the prerequisite for those tests to be meaningful.
    </acceptance>
    <labels>feat-008, feat-010, bug, graphql</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="db522b465be628c687fd4d4d13cdcc3083a3e43a">
commit db522b465be628c687fd4d4d13cdcc3083a3e43a
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 21:44:35 2026 -0400

    chore: add execution evidence [20260423T013618-]

diff --git a/.ddx/executions/20260423T013618-08d9a387/manifest.json b/.ddx/executions/20260423T013618-08d9a387/manifest.json
new file mode 100644
index 00000000..6fa3ca86
--- /dev/null
+++ b/.ddx/executions/20260423T013618-08d9a387/manifest.json
@@ -0,0 +1,106 @@
+{
+  "attempt_id": "20260423T013618-08d9a387",
+  "bead_id": "ddx-05b4cc9d",
+  "base_rev": "e2af37707a5e3fa56849fc22c1bd0a9f2689874e",
+  "created_at": "2026-04-23T01:36:19.510227689Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-05b4cc9d",
+    "title": "workersByProject compares project id to project path: per-project workers view always empty",
+    "description": "## Observed\n\nUser clicks \"Drain queue\" on the project home page. Mutation returns success with a worker id. Navigating to `/nodes/.../projects/\u003cid\u003e/workers` shows no workers — an empty table.\n\nVerified on disk: the worker record exists and is running.\n\n```\n$ cat .ddx/workers/worker-20260422T205846-f23f/status.json\n{\n  \"id\": \"worker-20260422T205846-f23f\",\n  \"kind\": \"execute-loop\",\n  \"state\": \"running\",\n  \"project_root\": \"/Users/erik/Projects/ddx\",\n  ...\n}\n```\n\nSo the dispatch worked, but the per-project workers list filters it out.\n\n## Root cause\n\n`cli/internal/server/state_graphql.go:156` in `GetWorkersGraphQL`:\n\n```go\nif projectID != \"\" \u0026\u0026 rec.ProjectRoot != projectID {\n    continue\n}\n```\n\n- `projectID` is the GraphQL argument — a project **id** like `proj-96d7ea83`.\n- `rec.ProjectRoot` is the worker record field — a project **path** like `/Users/erik/Projects/ddx`.\n\nThese are different representations and never match. With any non-empty `projectID`, every worker is filtered out.\n\nThe global `workers` resolver (`resolver_agent.go:9`) passes `\"\"` and works correctly. Only the per-project path (`workersByProject` → `GetWorkersGraphQL(projectID)`) is broken. The user's flow (dispatch from project home → navigate to project workers) hits exactly this path.\n\nNote: `GetBeadSnapshots` (state_graphql.go:49-105) handles the same concept correctly by iterating `s.Projects` and filtering on `proj.ID != projectID` — comparing id to id. The worker resolver does not use that pattern.\n\n## Proposed direction\n\nTwo options; prefer (1) for minimal change, (2) for long-term clarity.\n\n### Option 1 — Resolve id → path at the filter site\n\n```go\nexpectedPath := \"\"\nif projectID != \"\" {\n    proj, ok := s.GetProjectByID(projectID)\n    if !ok {\n        return nil // unknown project id → no workers\n    }\n    expectedPath = proj.Path\n}\n...\nif expectedPath != \"\" \u0026\u0026 rec.ProjectRoot != expectedPath {\n    continue\n}\n```\n\n### Option 2 — Store project id on the worker record\n\nAdd `ProjectID string` to `WorkerRecord`. Populate at creation time in `WorkerManager.StartExecuteLoop` via `s.GetProjectByPath(effectiveRoot)`. Filter on `rec.ProjectID != projectID`. Cleaner — the worker record carries its own identity and the query doesn't depend on runtime resolution. Requires a tiny migration (for older records missing the field, fall back to path-resolve as in Option 1).\n\nRecommend Option 2. It removes the representation mismatch for good and is cheap.\n\n## Scope note\n\nThis is a one-line functional bug with outsized impact: it's the reason the Workers page looks broken whenever used as the dashboard intends. Prioritizing P1.\n\n## Out of scope\n\n- Workers pane lifecycle controls (`ddx-69789664`).\n- Cross-project workers view — the global `workers` query already handles that.",
+    "acceptance": "**User story:** As an operator, clicking Drain queue on a project home page starts a worker that I can immediately see in that project's Workers view, with its state and current bead updating live.\n\n**Acceptance criteria:**\n\n1. **Root-cause fix.** The `workersByProject(projectID:)` resolver returns all workers whose target project matches the requested id. Both Option 1 (id→path resolution) and Option 2 (store project id on record) satisfy this; implementation chooses one with a one-line justification in the PR description.\n\n2. **Regression test — project-scoped query.**\n   - Seed a ServerState with two registered projects (different paths and ids).\n   - Start one worker for each project.\n   - `workersByProject(projectID: \u003cid-1\u003e)` returns exactly worker 1.\n   - `workersByProject(projectID: \u003cid-2\u003e)` returns exactly worker 2.\n   - `workers` (global) returns both.\n   - Unknown project id returns empty list, not an error.\n\n3. **Regression test — backward compat.**\n   - A worker record on disk without a `ProjectID` field (simulating a pre-migration record) is correctly matched by the path-resolution fallback. Only required if Option 2 is chosen.\n\n4. **End-to-end validation.**\n   - Playwright: navigate to project home, click Drain, confirm, wait 2s, navigate to Workers tab. Assert exactly one new row with `state: running` and matching worker id.\n   - Same test run against a second project asserts that project's Workers tab does not show project-1's worker.\n\n5. **No regression to unrelated surfaces.**\n   - Global `workers` query still returns all workers.\n   - Worker detail page still loads by worker id.\n   - Workers live-progress subscription still patches rows in place.\n\n6. **Cross-reference.**\n   - Note on `ddx-69789664` (Workers lifecycle controls): that bead's acceptance assumed Workers pane worked correctly. Point to this bead as the prerequisite for those tests to be meaningful.",
+    "labels": [
+      "feat-008",
+      "feat-010",
+      "bug",
+      "graphql"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-23T01:36:18Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2988370",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"omlx-vidar-1235\",\"resolved_model\":\"qwen/qwen3.6-35b-a3b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:01:46.806289388Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=omlx-vidar-1235 model=qwen/qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen/qwen3.6-35b-a3b probe=ok\nagent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen/qwen3.6-35b-a3b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-22T21:01:46.999250196Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"codex/gpt-5.4\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:01:49.582175554Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=codex/gpt-5.4"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness=claude model=codex/gpt-5.4 probe=ok\nunsupported model \"codex/gpt-5.4\" for harness \"claude\"; supported models: sonnet, opus, claude-sonnet-4-6",
+          "created_at": "2026-04-22T21:01:49.779797439Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"gemini\",\"resolved_model\":\"minimax/minimax-m2.7\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T21:01:52.453839668Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=gemini model=minimax/minimax-m2.7"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness=gemini model=minimax/minimax-m2.7 probe=ok\nunsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+          "created_at": "2026-04-22T21:01:52.658751167Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen/qwen3.6-35b-a3b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2026},{\"tier\":\"standard\",\"harness\":\"claude\",\"model\":\"codex/gpt-5.4\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2010},{\"tier\":\"smart\",\"harness\":\"gemini\",\"model\":\"minimax/minimax-m2.7\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2014}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-22T21:01:52.719667497Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=3 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite\ntier=smart\nprobe_result=ok\nresult_rev=84b9ea6ccc517076dc1221fc6aec1312d49119b9\nbase_rev=84b9ea6ccc517076dc1221fc6aec1312d49119b9\nretry_after=2026-04-23T03:01:52Z",
+          "created_at": "2026-04-22T21:01:52.894604164Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-23T01:36:18.957750399Z",
+      "execute-loop-last-detail": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+      "execute-loop-last-status": "execution_failed",
+      "feature": "FEAT-008"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260423T013618-08d9a387",
+    "prompt": ".ddx/executions/20260423T013618-08d9a387/prompt.md",
+    "manifest": ".ddx/executions/20260423T013618-08d9a387/manifest.json",
+    "result": ".ddx/executions/20260423T013618-08d9a387/result.json",
+    "checks": ".ddx/executions/20260423T013618-08d9a387/checks.json",
+    "usage": ".ddx/executions/20260423T013618-08d9a387/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-05b4cc9d-20260423T013618-08d9a387"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260423T013618-08d9a387/result.json b/.ddx/executions/20260423T013618-08d9a387/result.json
new file mode 100644
index 00000000..ddc3db76
--- /dev/null
+++ b/.ddx/executions/20260423T013618-08d9a387/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-05b4cc9d",
+  "attempt_id": "20260423T013618-08d9a387",
+  "base_rev": "e2af37707a5e3fa56849fc22c1bd0a9f2689874e",
+  "result_rev": "818b5c9e398896fe42b52c74a09cd71c2ffa4414",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-ad4bb894",
+  "duration_ms": 494927,
+  "tokens": 21833,
+  "cost_usd": 3.845802,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T013618-08d9a387",
+  "prompt_file": ".ddx/executions/20260423T013618-08d9a387/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T013618-08d9a387/manifest.json",
+  "result_file": ".ddx/executions/20260423T013618-08d9a387/result.json",
+  "usage_file": ".ddx/executions/20260423T013618-08d9a387/usage.json",
+  "started_at": "2026-04-23T01:36:19.510858271Z",
+  "finished_at": "2026-04-23T01:44:34.438839839Z"
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
## Review: ddx-05b4cc9d iter 1

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

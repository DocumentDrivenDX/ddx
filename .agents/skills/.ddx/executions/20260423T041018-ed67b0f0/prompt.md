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

  <diff rev="082ae59e77f25d60b092aa62ff877681bd61ed27">
commit 082ae59e77f25d60b092aa62ff877681bd61ed27
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Thu Apr 23 00:10:16 2026 -0400

    chore: add execution evidence [20260423T040532-]

diff --git a/.ddx/executions/20260423T040532-52637c0a/result.json b/.ddx/executions/20260423T040532-52637c0a/result.json
new file mode 100644
index 00000000..de5741cb
--- /dev/null
+++ b/.ddx/executions/20260423T040532-52637c0a/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-05b4cc9d",
+  "attempt_id": "20260423T040532-52637c0a",
+  "base_rev": "b6de5367a0d8d08acb497c0dcb20f5397d4caf64",
+  "result_rev": "5f234aad8d51f5f01a5ad3d8107164b46edb3cb7",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-e68dc2b6",
+  "duration_ms": 282433,
+  "tokens": 2621313,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T040532-52637c0a",
+  "prompt_file": ".ddx/executions/20260423T040532-52637c0a/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T040532-52637c0a/manifest.json",
+  "result_file": ".ddx/executions/20260423T040532-52637c0a/result.json",
+  "usage_file": ".ddx/executions/20260423T040532-52637c0a/usage.json",
+  "started_at": "2026-04-23T04:05:32.865936239Z",
+  "finished_at": "2026-04-23T04:10:15.299448626Z"
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

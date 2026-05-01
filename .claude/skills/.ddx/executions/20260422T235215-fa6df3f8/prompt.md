<bead-review>
  <bead id="ddx-ad0db8fd" iter=1>
    <title>Bead detail GraphQL resolver scans every project and every bead on each request</title>
    <description>
## Observed

Opening a bead in the sidepane on `/nodes/.../projects/.../beads?q=test` takes 5+ seconds. The beads list renders quickly; the delay is on single-bead lookup.

## Root cause

`cli/internal/server/graphql/resolver.go:93-100` — the `Bead(id)` resolver calls `r.State.GetBeadSnapshots("", "", "", "")` with no filters, which at `cli/internal/server/state_graphql.go:49-105` walks **every project**, constructs a `bead.Store` per project, calls `ReadAll()` (full directory scan + JSONL parse of every bead file), then iterates the result in Go to find one matching ID.

So every sidepane open = `O(projects × beads_per_project)` disk reads + parses, for a lookup that should be `O(1)` or `O(log n)`.

`bead.Store` already exposes `Get(id)` (`cli/internal/bead/store.go:450`), but the resolver doesn't use it — nor does it know which project the bead lives in, because the GraphQL schema exposes `bead(id:)` without a project scope.

## Proposed direction (not prescriptive)

Any of these would resolve it; pick based on measurements:

1. **Project scope the query.** Change schema to `bead(projectId:, id:)` (or accept a composite global id) and call `Store.Get` directly.
2. **Maintain an in-memory index** `beadID → projectID` refreshed on bead mutations and on a debounced filesystem watcher; resolver does one map lookup + one `Store.Get`.
3. **Short-TTL snapshot cache** behind `GetBeadSnapshots`, invalidated on bead writes.

Before picking: add a baseline measurement harness (see acceptance) so we know the actual p50/p95 before and after, not just anecdote.
    </description>
    <acceptance>
**User story:** As a developer browsing the web UI beads view, I want clicking a bead to open its detail pane within ~200ms on a repo with hundreds of beads across several projects, so the UI feels interactive.

**Acceptance criteria:**

1. A benchmark/harness exists (Go benchmark or scripted GraphQL client) that measures end-to-end `bead(id:)` latency against a fixture repo with ≥ 5 projects and ≥ 500 beads total. Output is a p50/p95 number, not a pass/fail bool.
2. Baseline is recorded in the bead notes (actual number measured on the current `main` binary at the time of fix).
3. After the fix, p95 for `bead(id:)` on the fixture is ≤ 50ms in-process / ≤ 200ms over HTTP on a dev laptop.
4. The fix does not introduce a stale-read regression: writing a bead via `ddx bead update` and then fetching it via GraphQL returns the new value on the next request (verified by a test that does write-then-read).
5. Concurrency-safe: an integration test spawns N concurrent `bead(id:)` queries interleaved with `bead update` mutations and asserts no panics, no torn reads, and consistent responses.
6. Playwright e2e: open `/nodes/.../beads`, click a bead row, assert the sidepane shows the bead title within 1s (generous budget; guards against regressions not the p95).
    </acceptance>
    <labels>feat-008, feat-004, perf, graphql</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="120e0ded9a6ba73dee88b5141f034de9c53435c0">
commit 120e0ded9a6ba73dee88b5141f034de9c53435c0
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 19:52:09 2026 -0400

    chore: add execution evidence [20260422T233904-]

diff --git a/.ddx/executions/20260422T233904-05fcb934/manifest.json b/.ddx/executions/20260422T233904-05fcb934/manifest.json
new file mode 100644
index 00000000..274733c2
--- /dev/null
+++ b/.ddx/executions/20260422T233904-05fcb934/manifest.json
@@ -0,0 +1,106 @@
+{
+  "attempt_id": "20260422T233904-05fcb934",
+  "bead_id": "ddx-ad0db8fd",
+  "base_rev": "b0b7e6cfdcbb4e8b3acfa52f040ac98117b3e5ad",
+  "created_at": "2026-04-22T23:39:05.072725646Z",
+  "requested": {
+    "harness": "codex",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-ad0db8fd",
+    "title": "Bead detail GraphQL resolver scans every project and every bead on each request",
+    "description": "## Observed\n\nOpening a bead in the sidepane on `/nodes/.../projects/.../beads?q=test` takes 5+ seconds. The beads list renders quickly; the delay is on single-bead lookup.\n\n## Root cause\n\n`cli/internal/server/graphql/resolver.go:93-100` — the `Bead(id)` resolver calls `r.State.GetBeadSnapshots(\"\", \"\", \"\", \"\")` with no filters, which at `cli/internal/server/state_graphql.go:49-105` walks **every project**, constructs a `bead.Store` per project, calls `ReadAll()` (full directory scan + JSONL parse of every bead file), then iterates the result in Go to find one matching ID.\n\nSo every sidepane open = `O(projects × beads_per_project)` disk reads + parses, for a lookup that should be `O(1)` or `O(log n)`.\n\n`bead.Store` already exposes `Get(id)` (`cli/internal/bead/store.go:450`), but the resolver doesn't use it — nor does it know which project the bead lives in, because the GraphQL schema exposes `bead(id:)` without a project scope.\n\n## Proposed direction (not prescriptive)\n\nAny of these would resolve it; pick based on measurements:\n\n1. **Project scope the query.** Change schema to `bead(projectId:, id:)` (or accept a composite global id) and call `Store.Get` directly.\n2. **Maintain an in-memory index** `beadID → projectID` refreshed on bead mutations and on a debounced filesystem watcher; resolver does one map lookup + one `Store.Get`.\n3. **Short-TTL snapshot cache** behind `GetBeadSnapshots`, invalidated on bead writes.\n\nBefore picking: add a baseline measurement harness (see acceptance) so we know the actual p50/p95 before and after, not just anecdote.",
+    "acceptance": "**User story:** As a developer browsing the web UI beads view, I want clicking a bead to open its detail pane within ~200ms on a repo with hundreds of beads across several projects, so the UI feels interactive.\n\n**Acceptance criteria:**\n\n1. A benchmark/harness exists (Go benchmark or scripted GraphQL client) that measures end-to-end `bead(id:)` latency against a fixture repo with ≥ 5 projects and ≥ 500 beads total. Output is a p50/p95 number, not a pass/fail bool.\n2. Baseline is recorded in the bead notes (actual number measured on the current `main` binary at the time of fix).\n3. After the fix, p95 for `bead(id:)` on the fixture is ≤ 50ms in-process / ≤ 200ms over HTTP on a dev laptop.\n4. The fix does not introduce a stale-read regression: writing a bead via `ddx bead update` and then fetching it via GraphQL returns the new value on the next request (verified by a test that does write-then-read).\n5. Concurrency-safe: an integration test spawns N concurrent `bead(id:)` queries interleaved with `bead update` mutations and asserts no panics, no torn reads, and consistent responses.\n6. Playwright e2e: open `/nodes/.../beads`, click a bead row, assert the sidepane shows the bead title within 1s (generous budget; guards against regressions not the p95).",
+    "labels": [
+      "feat-008",
+      "feat-004",
+      "perf",
+      "graphql"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-22T23:39:04Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "2988370",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"omlx-vidar-1235\",\"resolved_model\":\"qwen/qwen3.6-35b-a3b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T20:59:05.950521722Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=omlx-vidar-1235 model=qwen/qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=cheap harness=agent model=qwen/qwen3.6-35b-a3b probe=ok\nagent: provider error: openai: POST \"http://vidar:1235/v1/chat/completions\": 404 Not Found {\"message\":\"Model 'qwen/qwen3.6-35b-a3b' not found. Available models: Qwen3.5-122B-A10B-RAM-100GB-MLX, MiniMax-M2.5-MLX-4bit, Qwen3-Coder-Next-MLX-4bit, gemma-4-31B-it-MLX-4bit, Qwen3.5-27B-4bit, Qwen3.5-27B-Claude-4.6-Opus-Distilled-MLX-4bit, Qwen3.6-35B-A3B-4bit, Qwen3.6-35B-A3B-nvfp4, gpt-oss-20b-MXFP4-Q8\",\"type\":\"not_found_error\",\"param\":null,\"code\":null}",
+          "created_at": "2026-04-22T20:59:06.140939145Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"codex/gpt-5.4\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T20:59:08.987836547Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=codex/gpt-5.4"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=standard harness=claude model=codex/gpt-5.4 probe=ok\nunsupported model \"codex/gpt-5.4\" for harness \"claude\"; supported models: sonnet, opus, claude-sonnet-4-6",
+          "created_at": "2026-04-22T20:59:09.194779063Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"gemini\",\"resolved_model\":\"minimax/minimax-m2.7\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-22T20:59:11.82744163Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=gemini model=minimax/minimax-m2.7"
+        },
+        {
+          "actor": "ddx",
+          "body": "tier=smart harness=gemini model=minimax/minimax-m2.7 probe=ok\nunsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+          "created_at": "2026-04-22T20:59:12.015250183Z",
+          "kind": "tier-attempt",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"tiers_attempted\":[{\"tier\":\"cheap\",\"harness\":\"agent\",\"model\":\"qwen/qwen3.6-35b-a3b\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2323},{\"tier\":\"standard\",\"harness\":\"claude\",\"model\":\"codex/gpt-5.4\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2012},{\"tier\":\"smart\",\"harness\":\"gemini\",\"model\":\"minimax/minimax-m2.7\",\"status\":\"execution_failed\",\"cost_usd\":0,\"duration_ms\":2015}],\"winning_tier\":\"exhausted\",\"total_cost_usd\":0,\"wasted_cost_usd\":0}",
+          "created_at": "2026-04-22T20:59:12.073529736Z",
+          "kind": "escalation-summary",
+          "source": "ddx agent execute-loop",
+          "summary": "winning_tier=exhausted attempts=3 total_cost_usd=0.0000 wasted_cost_usd=0.0000"
+        },
+        {
+          "actor": "ddx",
+          "body": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite\ntier=smart\nprobe_result=ok\nresult_rev=c8e08156ecc62d9a084ed706a7182ab79974f3f6\nbase_rev=c8e08156ecc62d9a084ed706a7182ab79974f3f6\nretry_after=2026-04-23T02:59:12Z",
+          "created_at": "2026-04-22T20:59:12.243974198Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-22T23:39:04.554409849Z",
+      "execute-loop-last-detail": "escalation exhausted: unsupported model \"minimax/minimax-m2.7\" for harness \"gemini\"; supported models: gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite",
+      "execute-loop-last-status": "execution_failed",
+      "feature": "FEAT-008"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260422T233904-05fcb934",
+    "prompt": ".ddx/executions/20260422T233904-05fcb934/prompt.md",
+    "manifest": ".ddx/executions/20260422T233904-05fcb934/manifest.json",
+    "result": ".ddx/executions/20260422T233904-05fcb934/result.json",
+    "checks": ".ddx/executions/20260422T233904-05fcb934/checks.json",
+    "usage": ".ddx/executions/20260422T233904-05fcb934/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-ad0db8fd-20260422T233904-05fcb934"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260422T233904-05fcb934/result.json b/.ddx/executions/20260422T233904-05fcb934/result.json
new file mode 100644
index 00000000..48623dd7
--- /dev/null
+++ b/.ddx/executions/20260422T233904-05fcb934/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-ad0db8fd",
+  "attempt_id": "20260422T233904-05fcb934",
+  "base_rev": "b0b7e6cfdcbb4e8b3acfa52f040ac98117b3e5ad",
+  "result_rev": "5a05c1780b8ef2fa231e121c17c5bd94469bc238",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-27f3085b",
+  "duration_ms": 782210,
+  "tokens": 8742438,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T233904-05fcb934",
+  "prompt_file": ".ddx/executions/20260422T233904-05fcb934/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T233904-05fcb934/manifest.json",
+  "result_file": ".ddx/executions/20260422T233904-05fcb934/result.json",
+  "usage_file": ".ddx/executions/20260422T233904-05fcb934/usage.json",
+  "started_at": "2026-04-22T23:39:05.073116479Z",
+  "finished_at": "2026-04-22T23:52:07.283676862Z"
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
## Review: ddx-ad0db8fd iter 1

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

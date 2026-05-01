<bead-review>
  <bead id="ddx-2ceb02fa" iter=1>
    <title>Sessions UI is blind to post-2026-04-19 activity: service.Execute path no longer writes sessions.jsonl</title>
    <description>
## Observed

The Sessions page at `/nodes/.../projects/.../sessions` shows nothing newer than **2026-04-19**, three days ago. ddx is definitely still running — `.ddx/executions/` contains bundles from 2026-04-22 (today), `agent-s-*.jsonl` from 2026-04-21, `agent-loop-*.jsonl` from 2026-04-20. The UI simply cannot see them.

## Root cause

The Sessions GraphQL resolver reads exclusively from `.ddx/agent-logs/sessions.jsonl` (`cli/internal/server/state_graphql.go:504-516`). That file stopped being appended on 2026-04-19 19:46.

The only writer is `Runner.logSession` (`cli/internal/agent/runner.go:847-914`). It is called from four sites today:

- `cli/internal/agent/runner.go:229` (Runner.Run — legacy)
- `cli/internal/agent/agent_runner_service.go:216`
- `cli/internal/agent/claude_stream.go:516`
- `cli/internal/agent/virtual.go:206`

Production execute-bead / execute-loop no longer goes through any of them. The Runner-to-service refactor series (`5b933103`, `280f58e0`, `74eceda9`, `7cc1eb58`, `e05c785e`, `9f79f037`) routed the common path through `service.Execute` directly and did not carry forward the sessions.jsonl append. The last sessions.jsonl write lines up with the day that refactor set became default.

## Secondary issue (same fix should address)

`sessions.jsonl` is 397 MB for only 895 entries because each entry embeds the full `result.Output` (see `SessionEntry.Response` at `runner.go:879`). Growth is unbounded, there is no rotation, and `GetAgentSessionsGraphQL` (`state_graphql.go:476-490`) reads the whole file, parses every entry, and sorts in memory on every query. Even if we just restore the writer, the feed is fragile and will become a perf problem again quickly — relevant to `ddx-ad0db8fd` (bead detail was also O(N) scan).

## Proposed direction

Do not simply paper over by re-plumbing `logSession` into `service.Execute`. The right fix is:

1. **Decide the source of truth for "a session".** Current candidates on disk:
   - `.ddx/executions/&lt;ts&gt;-&lt;hash&gt;/{manifest.json,prompt.md,result.json}` — already written per execute-bead attempt.
   - `.ddx/agent-logs/agent-*.jsonl` and `agent-s-*.jsonl` — per-run streams from the agent harness.
   - Bead evidence events (kind:cost already written per attempt, see `execute_bead.go:492`).
   A session record should be derivable from a combination of execution bundles + agent-log streams, not from a separately-appended aggregate. The aggregate should be an **index**, not a **copy**.

2. **Introduce a lightweight index.** A thin `.ddx/agent-logs/sessions.index.jsonl` containing `{id, projectID, beadID, harness, model, startedAt, endedAt, durationMs, cost, tokens, outcome, bundlePath}` — pointer fields only, no embedded response bodies. Existing `sessions.jsonl` is deprecated to a read-only legacy file during a grace period.

3. **Write the index from `service.Execute`** (or a wrapper the execute-bead / run / execute-loop paths all share). One place to touch. Unit test guards that every production code path that executes an agent appends exactly one index row.

4. **GraphQL reads the index, streams bodies on demand.** `sessions { edges { node { ... } } }` reads the index. A new `session(id: ID!) { prompt, response, stderr }` lazy-loads the heavy fields from the execution bundle or agent-log file when the user expands a row.

5. **Legacy sessions.jsonl compatibility.** The index can be seeded once from the existing 397 MB file so historical data is not lost, then the old file is not read again. One-shot migration command (`ddx agent log reindex` or similar).

## Out of scope

- Replaying or regenerating sessions for the 2026-04-19 → 2026-04-22 gap. If the evidence exists in executions/agent-logs we can backfill the index; if not, accept the gap and cap it with a dated marker in the UI.
- A full telemetry rework. This bead just restores visibility and fixes the shape.
    </description>
    <acceptance>
**User story:** As an operator, I expect the Sessions page to reflect reality — if I ran `ddx work` today, today's sessions appear within seconds. And I expect the feed not to silently break again the next time the agent execution path is refactored.

**Acceptance criteria:**

1. **Session-index source of truth.** A single writer (invoked from `service.Execute` or a thin wrapper that every production agent-execution path calls) appends one line per completed agent invocation to `.ddx/agent-logs/sessions.index.jsonl`. Fields are pointer-only: id, projectID, beadID, harness, surface, model, startedAt, endedAt, durationMs, cost, tokens, outcome, exitCode, bundlePath (relative), nativeLogRef (relative). No prompt or response bodies embedded.

2. **Single-call-site guarantee.** Unit test enumerates every production code path that executes an agent (execute-bead, execute-loop, `ddx agent run`, quorum, replay if still present) and asserts each appends exactly one index row. If a new path is added later without wiring this up, the test fails.

3. **Visibility restored.** GraphQL `sessions` query reads `sessions.index.jsonl`. Running `ddx work` (or any agent-execution path) against a fixture project causes the new session to appear in a GraphQL query within 2 seconds of completion. E2E test demonstrates this.

4. **Body lazy-load.** A new resolver `session(id: ID!) { prompt response stderr }` loads the heavy fields on demand from the execution bundle (`.ddx/executions/&lt;ts&gt;-&lt;hash&gt;/`) or the agent-log path, without reading every session in the index.

5. **Legacy migration.** A one-shot command (e.g. `ddx agent log reindex`) seeds the index from the existing `.ddx/agent-logs/sessions.jsonl` (397 MB corpus on the current repo), preserving historical sessions. Integration test uses a fixture `sessions.jsonl` and asserts the index has one row per legacy entry with all pointer fields populated.

6. **Deprecated-file behavior.** After migration, the legacy `sessions.jsonl` is not read by the server. `ddx agent usage` and related CLI commands (`cli/cmd/agent_usage.go:197,246,348`, `cli/cmd/status.go:179`) are updated to read from the index too, verified by their existing tests.

7. **Perf sanity.** The GraphQL `sessions` list resolves in ≤100ms p95 on a fixture with 10,000 index entries; current behavior times out or OOMs. Benchmark captured in bead notes as baseline + after-fix.

8. **UI note.** If there is a gap in the index (e.g. the 2026-04-19 → 2026-04-22 gap on the current repo, assuming backfill from executions/agent-logs is not feasible), the Sessions page surfaces a single-line banner "No sessions recorded between X and Y" so the gap is not silently invisible. If backfill is feasible, do it instead and skip the banner.

9. **Playwright e2e.**
   - Seeds a fixture project with `.ddx/executions/` bundles from both before and after a simulated gap.
   - Runs the migration command.
   - Opens `/sessions`, asserts the most recent session timestamp matches the latest bundle, asserts a clicked row lazy-loads prompt/response from the bundle rather than from an embedded payload.
   - Asserts the feed updates live (within 2s) after a new simulated session is written.
    </acceptance>
    <labels>feat-008, feat-010, feat-006, perf, observability</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="c711065220665fca2f7dea00bf0a74273ea76df2">
commit c711065220665fca2f7dea00bf0a74273ea76df2
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 20:39:22 2026 -0400

    chore: add execution evidence [20260422T235239-]

diff --git a/.ddx/executions/20260422T235239-6ef17121/result.json b/.ddx/executions/20260422T235239-6ef17121/result.json
new file mode 100644
index 00000000..baa84bdf
--- /dev/null
+++ b/.ddx/executions/20260422T235239-6ef17121/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-2ceb02fa",
+  "attempt_id": "20260422T235239-6ef17121",
+  "base_rev": "4b85d7e0d37f2c8b70088d43b0767cb3718bcb7a",
+  "result_rev": "99033c89bda9b7a8508c488fe2e5ed5993e0f61d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-92854c3a",
+  "duration_ms": 2801084,
+  "tokens": 51236564,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260422T235239-6ef17121",
+  "prompt_file": ".ddx/executions/20260422T235239-6ef17121/prompt.md",
+  "manifest_file": ".ddx/executions/20260422T235239-6ef17121/manifest.json",
+  "result_file": ".ddx/executions/20260422T235239-6ef17121/result.json",
+  "usage_file": ".ddx/executions/20260422T235239-6ef17121/usage.json",
+  "started_at": "2026-04-22T23:52:40.216964934Z",
+  "finished_at": "2026-04-23T00:39:21.301315804Z"
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
## Review: ddx-2ceb02fa iter 1

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

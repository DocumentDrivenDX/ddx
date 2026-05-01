<bead-review>
  <bead id="ddx-69789664" iter=1>
    <title>Workers screen: clarify purpose vs Sessions and add lifecycle controls</title>
    <description>
## Observed

`/nodes/.../projects/.../workers` renders a table of workers but has no buttons, no controls, and no explanation of how it differs from `/sessions`. The layout (`cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/workers/+layout.svelte`) shows ID / Kind / State / Current Bead / Attempts — read-only. Clicking a row drills into live-streaming logs and recent events, but offers no actions there either.

This raised a fair question: **what is this screen for, and how is it different from Sessions?** The answer isn't in the UI.

## Ground truth (from the CLI)

- **Worker** = a persistent execute-loop process draining the bead queue (`ddx work` / `ddx agent execute-loop`). Has a lifecycle: running / idle / stopped / error. Has a *current bead* and cumulative attempts/successes/failures. Can be stopped via `ddx agent workers stop`. Long-lived.
- **Session** = a historical record of one agent invocation (harness, model, bead, tokens, cost, outcome, start/end). Immutable after completion. One worker produces many sessions over its lifetime.

So: Workers are *processes you operate*. Sessions are *events you review*. The current UI exposes both as flat tables and lets the reader guess which is which.

## Gaps

1. **No disambiguation.** No page-level copy, no link between the two, no worker→sessions navigation ("show sessions produced by this worker"), no session→worker backlink.
2. **No lifecycle controls on Workers.** The CLI has `ddx agent workers stop` and `ddx work` (start). The UI has neither button. No stop, start, restart, or "cancel current bead" affordance. The State column is a spectator sport.
3. **No worker start UX.** There is no way to start a queue drain from the UI — users must drop to a terminal.
4. **Unclear empty state.** "No workers found." does not say whether that is normal (nothing running) or broken (server can't see workers). No hint about how to start one.
5. **No worker details beyond logs.** Worker detail page shows live phase + log tail + tool calls but not: worker config (project path, profile/harness, started-at, last-heartbeat), resource usage, or the list of sessions this worker has produced.

## Proposed direction

Split into two deliverables — do them together if we're touching this screen anyway, but they are independently landable:

### A. Information architecture (copy + navigation)

- Add a short header description to each page: "Workers drain the bead queue. Sessions are the history of what they ran."
- Mutual links: Workers page links to "Recent sessions →", Sessions page links to "Workers →".
- On worker detail: a "Sessions" tab or section listing sessions where `session.workerId == this.id` (requires session records to carry workerID; check whether they do).
- On session detail (expandable row already exists): if the session has a workerID, surface it as a link back to the worker.

### B. Lifecycle controls

- **Stop** button per running worker row → calls a GraphQL mutation that invokes the equivalent of `ddx agent workers stop &lt;id&gt;`. Confirms before acting.
- **Start worker** button in the page header → opens a small form (project, profile/harness, effort, optional bead filter) and calls a `startWorker` mutation that shells out to `ddx work` in the background. The form should default to sane values and be submittable in one click for the common case.
- **Cancel current bead** action on the worker detail page — signals the worker to abandon the in-flight bead without killing the loop. (Only if CLI primitive exists; otherwise descope and note in the bead.)
- All mutations must be idempotent and log an audit record viewable in the UI (who did it, when). Reuse existing session-event plumbing if possible.

### Out of scope

- Rewriting execute-loop semantics.
- Cross-project worker orchestration.
- Auth / permissions model for who can stop workers (assume single-operator for now; file a follow-up if multi-operator becomes real).
    </description>
    <acceptance>
**User story:** As an operator looking at the DDx web UI, I want to understand at a glance what Workers are vs Sessions, and I want to stop a misbehaving worker or start a new queue drain without dropping to a terminal, so the UI is a real operator surface instead of a read-only status page.

**Acceptance criteria:**

1. **Copy + IA.** Workers page header includes a one-sentence description distinguishing it from Sessions. Sessions page likewise. Each links to the other. Playwright asserts both descriptions are present and the cross-links navigate.

2. **Worker → Sessions navigation.** On worker detail, a "Sessions" section lists sessions produced by that worker (join on `session.workerId`). Empty state reads "No sessions recorded yet." If session records do not yet carry a worker ID, that gap is filled as part of this bead and backfilled for existing sessions in the current run (no historical backfill required).

3. **Session → Worker backlink.** Expanded session row shows worker ID as a clickable link when present.

4. **Stop worker.** Each row with `state == 'running'` shows a Stop button. Clicking prompts for confirm, then fires a GraphQL mutation that stops the worker. UI updates within 2s via the existing subscription/poll path. Integration test asserts: button visibility is gated on state, confirm dialog is required, and mutation invokes the same code path as `ddx agent workers stop`.

5. **Start worker.** A "Start worker" button in the Workers page header opens a form (harness/profile, effort, optional label filter). Submitting fires a `startWorker` mutation. On success the new worker appears in the list within 2s. Form validation prevents empty submissions. The code path reuses `ddx work` / execute-loop internals — not a separate implementation.

6. **Audit trail.** Every lifecycle action (start, stop, cancel) is recorded as a timestamped event visible on the worker detail page, including the actor identifier the server has for the requester.

7. **No regressions.** Existing live log streaming, recent events, and phase overrides continue to work. Playwright smoke of the worker detail page passes.

8. **Playwright e2e.**
   - Navigates to Workers page, asserts header description.
   - Clicks "Start worker", fills form, submits, asserts new row appears.
   - Clicks Stop on that row, confirms, asserts row state transitions to `stopped`.
   - Clicks into the stopped worker, asserts the Sessions section renders (or the empty state).
   - Asserts the "Sessions" link in the worker page header navigates to the Sessions page.

9. **Design review sign-off.** Before implementation begins, a short design note (one page or less) in `docs/helix/02-design/` captures the Workers-vs-Sessions mental model, the chosen controls, and any CLI primitives that need to exist. Implementation PR links to that note.
    </acceptance>
    <labels>feat-008, feat-010, feat-006, ui, design</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="01b480acc775fc5256b0489142fb61e0ea635041">
commit 01b480acc775fc5256b0489142fb61e0ea635041
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 22:49:47 2026 -0400

    chore: add execution evidence [20260423T021809-]

diff --git a/.ddx/executions/20260423T021809-c3ed3f02/result.json b/.ddx/executions/20260423T021809-c3ed3f02/result.json
new file mode 100644
index 00000000..bee797ab
--- /dev/null
+++ b/.ddx/executions/20260423T021809-c3ed3f02/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-69789664",
+  "attempt_id": "20260423T021809-c3ed3f02",
+  "base_rev": "29b8ae2f5c38d4f486c499cdd8efc78bcd386141",
+  "result_rev": "b22b81cb6d072ca2591af79a969fb4a9b4b32613",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-a045f55d",
+  "duration_ms": 1896612,
+  "tokens": 19951722,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T021809-c3ed3f02",
+  "prompt_file": ".ddx/executions/20260423T021809-c3ed3f02/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T021809-c3ed3f02/manifest.json",
+  "result_file": ".ddx/executions/20260423T021809-c3ed3f02/result.json",
+  "usage_file": ".ddx/executions/20260423T021809-c3ed3f02/usage.json",
+  "started_at": "2026-04-23T02:18:09.753641611Z",
+  "finished_at": "2026-04-23T02:49:46.366624931Z"
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
## Review: ddx-69789664 iter 1

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

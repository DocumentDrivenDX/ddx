<bead-review>
  <bead id="ddx-7de0ce80" iter=1>
    <title>Plugins page doesn't refresh after update/install: worker completes, card still shows outdated</title>
    <description>
## Observed

On `/nodes/.../projects/.../plugins`, clicking Update on the outdated `helix` card:

1. Fires the mutation — works.
2. Shows a link to the dispatched worker — helpful.
3. Worker completes and the plugin is actually updated on disk — verified.
4. **Card still shows `update-available`.** The UI never closes the loop.

The user has no in-UI signal that the update succeeded. They must reload the page (or navigate away and back) to see the new state. Worse, nothing prevents them from clicking the button again during the gap — firing a second update.

## Root cause

`cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/plugins/+page.svelte:74-89` —

```js
async function dispatchPlugin(name, action, scope = 'project') {
  const result = await client.request(PLUGIN_DISPATCH_MUTATION, { name, action, scope });
  workerId = result.pluginDispatch.id;
}
```

That is the entire post-dispatch behavior. `data.plugins` is the snapshot from `+page.ts` at load time. There is no:

- subscription to the dispatched worker's progress,
- polling of the plugin list,
- refetch on worker terminal event,
- optimistic card state while the worker runs,
- per-card in-flight tracking (only a single `workerId: string | null` at module scope, which means dispatching a second plugin overwrites the first worker link).

The workers layout already has `subscribeWorkerProgress(workerID, cb)` (`cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/workers/+layout.svelte:22-30`) — the plugins page just doesn't use it.

## Proposed direction

Two changes, smallest first:

### Fix A — Close the loop on the dispatched worker.

- Per-card in-flight tracking: replace the singleton `workerId` with a `Map&lt;pluginName, workerId&gt;`. Each card shows its own worker link + in-flight state; clicking Install/Update is disabled while that plugin is in flight.
- Subscribe to `subscribeWorkerProgress` for each in-flight plugin. On a terminal phase event (`completed`, `failed`, etc. — whatever the existing worker event vocabulary uses), refetch the plugins list and clear that plugin's in-flight entry.
- If the existing worker-progress stream doesn't emit a terminal event with enough detail, fall back to polling `plugins { name status installedVersion }` every 2s while any card is in flight, stopping when none are.

### Fix B — Optimistic card state (optional polish).

- While a card is in-flight, show "Updating…" / "Installing…" with a spinner in place of the status badge, and disable the action button. This is orthogonal to Fix A and can ship with it.

### Fix C — Persistent failure surfacing.

- Currently `dispatchError` is a single string. If the worker terminates with a failure phase, surface that per-card (error badge + tooltip with detail link to the worker page) rather than relying on `dispatchError` which only covers the synchronous mutation error.

## Out of scope

- Designing new worker terminal events. Use whatever the workers layout already consumes.
- Cancelling an in-flight plugin update — follow-up if needed.
- Bulk update ("update all outdated"). Follow-up.
    </description>
    <acceptance>
**User story:** As a developer on the plugins page, I want clicking Update or Install to give me continuous feedback until the operation completes, and I want the card to reflect the new installed state automatically — without a page reload.

**Acceptance criteria:**

1. **Per-card in-flight tracking.** Clicking Update/Install records the workerId under the plugin's name in a `Map&lt;string, string&gt;`. The same plugin cannot be dispatched again while it is in flight (button disabled; tooltip shows the worker link). A different plugin can be dispatched concurrently and tracks its own worker.

2. **Worker-driven refresh.** For each in-flight plugin, the page subscribes to that worker's progress via the existing `subscribeWorkerProgress` channel. On a terminal event, the page refetches the plugins list and clears that plugin's entry from the in-flight map. Within 2s of worker terminal event, the card reflects the new status (e.g., `update-available` → `installed`, new `installedVersion`).

3. **Fallback polling.** If no terminal event arrives within 30s of dispatch, the page begins polling `plugins { name status installedVersion }` every 2s until the card's status or installedVersion changes, then stops. Integration test simulates a stream dropout and asserts the card eventually refreshes via the poll path.

4. **In-flight visual state.** While a card is in flight, its status badge is replaced by a spinner + "Updating…" or "Installing…" text. The action button is disabled with a tooltip pointing to the worker.

5. **Failure surfacing.** If the worker terminates in a failure phase, the card shows an error badge + tooltip "Update failed — view worker" linking to the worker page. The card's status/version remains whatever it was before dispatch (no false success). The global `dispatchError` stays for synchronous mutation errors only.

6. **Concurrent dispatch.** Dispatching Update on two different cards at once shows two active worker links (a row or small stack in the header instead of the single slot), and each card tracks its own state. Playwright asserts.

7. **No extra refetch overhead.** Refetch is scoped to the plugins list, not the full page data. Manual verification is enough; no specific perf test required.

8. **Playwright e2e.**
   - Seeds a fixture with one outdated plugin.
   - Clicks Update.
   - Asserts the card enters the in-flight visual state and the action button is disabled.
   - Simulates the worker completing successfully.
   - Asserts within 2s the card shows `installed` and a newer `installedVersion`.
   - Runs a second scenario where the simulated worker fails; asserts the failure badge and that status stays `update-available`.

9. **No regressions.** Install-from-registry flow (the ConfirmDialog path) adopts the same in-flight tracking; existing Playwright install flow still passes.
    </acceptance>
    <labels>feat-008, plugins, ui</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="0e938c9f884c38b588e60827aaff176603103f20">
commit 0e938c9f884c38b588e60827aaff176603103f20
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 23:15:42 2026 -0400

    chore: add execution evidence [20260423T030311-]

diff --git a/.ddx/executions/20260423T030311-161fbf53/result.json b/.ddx/executions/20260423T030311-161fbf53/result.json
new file mode 100644
index 00000000..61426d55
--- /dev/null
+++ b/.ddx/executions/20260423T030311-161fbf53/result.json
@@ -0,0 +1,21 @@
+{
+  "bead_id": "ddx-7de0ce80",
+  "attempt_id": "20260423T030311-161fbf53",
+  "base_rev": "5ee80367ffeece02bc77ce34ec790d56a828faf2",
+  "result_rev": "efd5bacbfe989e8d55d8013ab7058c8ed93dbb34",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "session_id": "eb-853af38c",
+  "duration_ms": 748797,
+  "tokens": 6707296,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T030311-161fbf53",
+  "prompt_file": ".ddx/executions/20260423T030311-161fbf53/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T030311-161fbf53/manifest.json",
+  "result_file": ".ddx/executions/20260423T030311-161fbf53/result.json",
+  "usage_file": ".ddx/executions/20260423T030311-161fbf53/usage.json",
+  "started_at": "2026-04-23T03:03:12.323053938Z",
+  "finished_at": "2026-04-23T03:15:41.120818271Z"
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
## Review: ddx-7de0ce80 iter 1

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

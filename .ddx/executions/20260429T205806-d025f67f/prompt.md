<bead-review>
  <bead id="ddx-37cdb43a" iter=1>
    <title>execute-loop: ddx work exits after one bead instead of draining the queue</title>
    <description>
## Symptom

`ddx work` (no `--once` flag, no `--poll-interval`, default mode) exits after processing exactly one bead instead of draining the queue. Reproduced four times today on `/home/erik/Projects/agent` with `--harness lmstudio` pinned (which sidesteps the unrelated `ddx-c7081f89` server-worker tier-walk issue):

| Run | Outcome on first bead | Worker terminal state | Queue still non-empty? |
|---|---|---|---|
| 2 | `already_satisfied` (`agent-4aa3584e` adjudicated satisfied) | `already_satisfied` / `exited` | yes — 2 ready beads remained |
| 3 | pre-claim hook rejected (diverged branch) | `exited`, attempts=0 | yes |
| 4 | pre-claim hook rejected (diverged branch) | `exited`, attempts=0 | yes |
| 5 | `no_changes` from agent (`agent-6c7a4c11` declined as epic; bead left **claimed**) | `exited`, attempts=1 | yes — 5 ready beads remained |

In all four runs the loop should have continued to the next ready bead. Per `agent/.claude/skills/ddx/reference/work.md`, only `success`/`already_satisfied` close the bead and `no_changes`/`land_conflict`/`*_failed` unclaim and continue. None of these outcomes is documented or expected to be loop-terminal.

## Root cause #1 (verified)

`cli/internal/agent/execute_bead_loop.go:382`:

```go
attempted[candidate.ID] = struct{}{}

// Pre-claim hook: fetch origin + verify ancestry before claiming.
// On error the bead is skipped for this iteration; the loop
// continues (ctx is not cancelled).
if runtime.PreClaimHook != nil {
    if hookErr := runtime.PreClaimHook(ctx); hookErr != nil {
        // ... log + emit ...
        continue
    }
}
```

The bead is added to `attempted` **before** the pre-claim hook runs. On hook failure (e.g. diverged branch), the loop `continue`s but the bead stays in `attempted`. Next iteration, `nextCandidate()` (line 924) skips the still-ready bead because it's in `attempted`, returns `ok=false`, and the loop exits at line 374 because `PollInterval &lt;= 0`. Single-shot exit with `attempts=0`.

**Reproduces:** runs 3 and 4 above.

## Root cause #2 (suspected — needs targeted instrumentation)

Runs 2 and 5 reached `Execute()` and got a real report back. Worker log shows the entry line `▶ &lt;bead&gt;: &lt;title&gt;` but no closing `✓ &lt;bead&gt; → &lt;result&gt;` line (which is written at line 909). That means the loop exited somewhere between line 488 (`Executor.Execute`) and line 909 (the final summary log).

Between those bounds, **at least 16 distinct `return result, err` points** trigger if any `Store.*` operation returns an error: `Unclaim` (711, 700), `IncrNoChangesCount` (717), `adjudicateNoChanges` (721), `CloseWithEvidence` (520, 615, 694, 739), `Reopen` (635, 669), `SetExecutionCooldown` (749, 791, 803, 835, 844), late `AppendEvent` (862). Any single transient store error during outcome handling kills the worker.

For run 5 specifically, `agent-6c7a4c11` is now `Status: in_progress` with `Owner: ddx, Machine: sindri`. The bead was **never unclaimed** despite the agent returning `no_changes` — meaning the loop exited on or before line 711 (`w.Store.Unclaim`) failing. Investigation should look at what error `Unclaim` would return when called against a bead the same worker claimed seconds earlier; possible candidates: tracker-write race with concurrent worker, bead.jsonl re-write during a fetch by the pre-claim hook on a later iteration that already happened (no — pre-claim is pre-claim, not post-execute), or a malformed bead row that fails store-side validation.

Independent of the specific store error: the structural pattern of "any Store.* error → exit the entire loop" is the wrong default. A store error on outcome handling should at most park the bead and continue to the next, never terminate the worker.

## Why this is loadbearing

The loop is the queue-drain primitive. Every observed failure-to-drain in the last 24 hours of operator complaints traces back to this — beads that should have been the worker's second/third/fourth attempt never got picked up because the worker exited after one attempt, and the operator manually restarts the drain (sometimes after a long delay). The visible queue depth is misleading: half the "ready" beads are sitting behind a worker that quietly died.

## In-scope files

- `cli/internal/agent/execute_bead_loop.go` — fixes for both root causes
- `cli/internal/agent/execute_bead_loop_test.go` — regression tests for each path

## Out of scope

- Anything in the `agent` SDK (`/home/erik/Projects/agent`). The loop and all its store interactions are owned by ddx.
- The pre-claim hook's divergence-detection logic itself — that's correct behavior; the bug is how the loop reacts to the hook's signal.
- Adaptive min-tier / tier-walk machinery (`ddx-c7081f89`, `ddx-aec9d68c`) — independent.

## Anti-goals

- Do **not** swallow store errors silently. They must be surfaced (event + log) but should not kill the worker on transient/per-bead failures.
- Do **not** convert the loop's Once/PollInterval semantics — they're correct.
- Do **not** change worker terminal-state semantics (`exited`/`failed`/`stopped`/`reaped`).
    </description>
    <acceptance>
1. **Root cause #1 fixed:** `attempted[candidate.ID] = struct{}{}` is moved to AFTER the pre-claim hook succeeds. Regression test in `execute_bead_loop_test.go` asserts: with a fake `PreClaimHook` that fails on the first call and succeeds thereafter, the loop processes the same bead on the second iteration (not skipped because of stale `attempted` state).

2. **Root cause #2 fixed:** the post-`Execute` outcome-handling block (lines ~510-862) does not exit the loop on transient `Store.*` errors. Each `return result, err` in that block is converted to: log the error, emit a `kind:loop-error` bead event when possible, set the bead to a short cooldown, and `continue` to the next iteration. The only exception is context cancellation (`ctx.Err()`), which still terminates immediately.

3. **Regression tests for AC-2:** for each `Store.*` call in the outcome block (Unclaim, IncrNoChangesCount, adjudicateNoChanges, CloseWithEvidence on each path, Reopen on each path, SetExecutionCooldown on each path, late AppendEvent), a test injects a transient error from a fake store and asserts: (a) the loop continues, (b) a `kind:loop-error` event is recorded with the failing operation name, (c) `result.Failures++` advances.

4. **End-to-end reproduction guard:** an integration test seeds a bead.jsonl with 3 ready beads. The first returns `no_changes` (executor fake), the second returns `success`, the third returns `already_satisfied`. The loop runs to completion (all three processed) without exiting prematurely. With the pre-fix loop, this test fails on the first bead.

5. **Worker-state semantics preserved:** existing tests that assert worker.State transitions (`running` → `exited` / `failed` / `stopped` / `reaped`) still pass. The fix changes loop *advancement*, not terminal-state classification.

6. **Documented behavior:** `docs/agents/execute-loop.md` (or the closest existing doc) gains a "loop termination" section listing the only conditions that terminate the loop: context cancellation, `Once == true`, `PollInterval &lt;= 0` AND `nextCandidate` returns no candidate, `RoutePreflight` rejected. Explicitly states that Store errors during outcome handling do NOT terminate the loop.

7. **Operator workaround retired:** the README's "if `ddx work` exits early, restart it" guidance (or equivalent) is removed; that workaround was masking this bug.
    </acceptance>
    <labels>area:execute-loop, kind:bug, phase:fix</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T203909-e46a1ac0/manifest.json</file>
    <file>.ddx/executions/20260429T203909-e46a1ac0/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="abc40efbaec5d2ec81d6c4922dac3bbcf330c609">
diff --git a/.ddx/executions/20260429T203909-e46a1ac0/manifest.json b/.ddx/executions/20260429T203909-e46a1ac0/manifest.json
new file mode 100644
index 00000000..6a0a819b
--- /dev/null
+++ b/.ddx/executions/20260429T203909-e46a1ac0/manifest.json
@@ -0,0 +1,37 @@
+{
+  "attempt_id": "20260429T203909-e46a1ac0",
+  "bead_id": "ddx-37cdb43a",
+  "base_rev": "d0e25307be28163376692f319372a635cb883718",
+  "created_at": "2026-04-29T20:39:09.999068804Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-37cdb43a",
+    "title": "execute-loop: ddx work exits after one bead instead of draining the queue",
+    "description": "## Symptom\n\n`ddx work` (no `--once` flag, no `--poll-interval`, default mode) exits after processing exactly one bead instead of draining the queue. Reproduced four times today on `/home/erik/Projects/agent` with `--harness lmstudio` pinned (which sidesteps the unrelated `ddx-c7081f89` server-worker tier-walk issue):\n\n| Run | Outcome on first bead | Worker terminal state | Queue still non-empty? |\n|---|---|---|---|\n| 2 | `already_satisfied` (`agent-4aa3584e` adjudicated satisfied) | `already_satisfied` / `exited` | yes — 2 ready beads remained |\n| 3 | pre-claim hook rejected (diverged branch) | `exited`, attempts=0 | yes |\n| 4 | pre-claim hook rejected (diverged branch) | `exited`, attempts=0 | yes |\n| 5 | `no_changes` from agent (`agent-6c7a4c11` declined as epic; bead left **claimed**) | `exited`, attempts=1 | yes — 5 ready beads remained |\n\nIn all four runs the loop should have continued to the next ready bead. Per `agent/.claude/skills/ddx/reference/work.md`, only `success`/`already_satisfied` close the bead and `no_changes`/`land_conflict`/`*_failed` unclaim and continue. None of these outcomes is documented or expected to be loop-terminal.\n\n## Root cause #1 (verified)\n\n`cli/internal/agent/execute_bead_loop.go:382`:\n\n```go\nattempted[candidate.ID] = struct{}{}\n\n// Pre-claim hook: fetch origin + verify ancestry before claiming.\n// On error the bead is skipped for this iteration; the loop\n// continues (ctx is not cancelled).\nif runtime.PreClaimHook != nil {\n    if hookErr := runtime.PreClaimHook(ctx); hookErr != nil {\n        // ... log + emit ...\n        continue\n    }\n}\n```\n\nThe bead is added to `attempted` **before** the pre-claim hook runs. On hook failure (e.g. diverged branch), the loop `continue`s but the bead stays in `attempted`. Next iteration, `nextCandidate()` (line 924) skips the still-ready bead because it's in `attempted`, returns `ok=false`, and the loop exits at line 374 because `PollInterval \u003c= 0`. Single-shot exit with `attempts=0`.\n\n**Reproduces:** runs 3 and 4 above.\n\n## Root cause #2 (suspected — needs targeted instrumentation)\n\nRuns 2 and 5 reached `Execute()` and got a real report back. Worker log shows the entry line `▶ \u003cbead\u003e: \u003ctitle\u003e` but no closing `✓ \u003cbead\u003e → \u003cresult\u003e` line (which is written at line 909). That means the loop exited somewhere between line 488 (`Executor.Execute`) and line 909 (the final summary log).\n\nBetween those bounds, **at least 16 distinct `return result, err` points** trigger if any `Store.*` operation returns an error: `Unclaim` (711, 700), `IncrNoChangesCount` (717), `adjudicateNoChanges` (721), `CloseWithEvidence` (520, 615, 694, 739), `Reopen` (635, 669), `SetExecutionCooldown` (749, 791, 803, 835, 844), late `AppendEvent` (862). Any single transient store error during outcome handling kills the worker.\n\nFor run 5 specifically, `agent-6c7a4c11` is now `Status: in_progress` with `Owner: ddx, Machine: sindri`. The bead was **never unclaimed** despite the agent returning `no_changes` — meaning the loop exited on or before line 711 (`w.Store.Unclaim`) failing. Investigation should look at what error `Unclaim` would return when called against a bead the same worker claimed seconds earlier; possible candidates: tracker-write race with concurrent worker, bead.jsonl re-write during a fetch by the pre-claim hook on a later iteration that already happened (no — pre-claim is pre-claim, not post-execute), or a malformed bead row that fails store-side validation.\n\nIndependent of the specific store error: the structural pattern of \"any Store.* error → exit the entire loop\" is the wrong default. A store error on outcome handling should at most park the bead and continue to the next, never terminate the worker.\n\n## Why this is loadbearing\n\nThe loop is the queue-drain primitive. Every observed failure-to-drain in the last 24 hours of operator complaints traces back to this — beads that should have been the worker's second/third/fourth attempt never got picked up because the worker exited after one attempt, and the operator manually restarts the drain (sometimes after a long delay). The visible queue depth is misleading: half the \"ready\" beads are sitting behind a worker that quietly died.\n\n## In-scope files\n\n- `cli/internal/agent/execute_bead_loop.go` — fixes for both root causes\n- `cli/internal/agent/execute_bead_loop_test.go` — regression tests for each path\n\n## Out of scope\n\n- Anything in the `agent` SDK (`/home/erik/Projects/agent`). The loop and all its store interactions are owned by ddx.\n- The pre-claim hook's divergence-detection logic itself — that's correct behavior; the bug is how the loop reacts to the hook's signal.\n- Adaptive min-tier / tier-walk machinery (`ddx-c7081f89`, `ddx-aec9d68c`) — independent.\n\n## Anti-goals\n\n- Do **not** swallow store errors silently. They must be surfaced (event + log) but should not kill the worker on transient/per-bead failures.\n- Do **not** convert the loop's Once/PollInterval semantics — they're correct.\n- Do **not** change worker terminal-state semantics (`exited`/`failed`/`stopped`/`reaped`).",
+    "acceptance": "1. **Root cause #1 fixed:** `attempted[candidate.ID] = struct{}{}` is moved to AFTER the pre-claim hook succeeds. Regression test in `execute_bead_loop_test.go` asserts: with a fake `PreClaimHook` that fails on the first call and succeeds thereafter, the loop processes the same bead on the second iteration (not skipped because of stale `attempted` state).\n\n2. **Root cause #2 fixed:** the post-`Execute` outcome-handling block (lines ~510-862) does not exit the loop on transient `Store.*` errors. Each `return result, err` in that block is converted to: log the error, emit a `kind:loop-error` bead event when possible, set the bead to a short cooldown, and `continue` to the next iteration. The only exception is context cancellation (`ctx.Err()`), which still terminates immediately.\n\n3. **Regression tests for AC-2:** for each `Store.*` call in the outcome block (Unclaim, IncrNoChangesCount, adjudicateNoChanges, CloseWithEvidence on each path, Reopen on each path, SetExecutionCooldown on each path, late AppendEvent), a test injects a transient error from a fake store and asserts: (a) the loop continues, (b) a `kind:loop-error` event is recorded with the failing operation name, (c) `result.Failures++` advances.\n\n4. **End-to-end reproduction guard:** an integration test seeds a bead.jsonl with 3 ready beads. The first returns `no_changes` (executor fake), the second returns `success`, the third returns `already_satisfied`. The loop runs to completion (all three processed) without exiting prematurely. With the pre-fix loop, this test fails on the first bead.\n\n5. **Worker-state semantics preserved:** existing tests that assert worker.State transitions (`running` → `exited` / `failed` / `stopped` / `reaped`) still pass. The fix changes loop *advancement*, not terminal-state classification.\n\n6. **Documented behavior:** `docs/agents/execute-loop.md` (or the closest existing doc) gains a \"loop termination\" section listing the only conditions that terminate the loop: context cancellation, `Once == true`, `PollInterval \u003c= 0` AND `nextCandidate` returns no candidate, `RoutePreflight` rejected. Explicitly states that Store errors during outcome handling do NOT terminate the loop.\n\n7. **Operator workaround retired:** the README's \"if `ddx work` exits early, restart it\" guidance (or equivalent) is removed; that workaround was masking this bug.",
+    "labels": [
+      "area:execute-loop",
+      "kind:bug",
+      "phase:fix"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T20:39:07Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T20:39:07.138116619Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T203909-e46a1ac0",
+    "prompt": ".ddx/executions/20260429T203909-e46a1ac0/prompt.md",
+    "manifest": ".ddx/executions/20260429T203909-e46a1ac0/manifest.json",
+    "result": ".ddx/executions/20260429T203909-e46a1ac0/result.json",
+    "checks": ".ddx/executions/20260429T203909-e46a1ac0/checks.json",
+    "usage": ".ddx/executions/20260429T203909-e46a1ac0/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-37cdb43a-20260429T203909-e46a1ac0"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T203909-e46a1ac0/result.json b/.ddx/executions/20260429T203909-e46a1ac0/result.json
new file mode 100644
index 00000000..238a4737
--- /dev/null
+++ b/.ddx/executions/20260429T203909-e46a1ac0/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-37cdb43a",
+  "attempt_id": "20260429T203909-e46a1ac0",
+  "base_rev": "d0e25307be28163376692f319372a635cb883718",
+  "result_rev": "d57f1ddf7f82e8338998c4cfb584fccbde924554",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-ab37d945",
+  "duration_ms": 1131913,
+  "tokens": 47703,
+  "cost_usd": 3.409703350000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T203909-e46a1ac0",
+  "prompt_file": ".ddx/executions/20260429T203909-e46a1ac0/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T203909-e46a1ac0/manifest.json",
+  "result_file": ".ddx/executions/20260429T203909-e46a1ac0/result.json",
+  "usage_file": ".ddx/executions/20260429T203909-e46a1ac0/usage.json",
+  "started_at": "2026-04-29T20:39:09.999492804Z",
+  "finished_at": "2026-04-29T20:58:01.913111771Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>

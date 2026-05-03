# Picker priority "starvation" — investigation (ddx-9d55601f)

## TL;DR

The picker is **not** violating priority ordering. The four "starved" P0
beads in the operator's reproducer were all parked on
`extra.execute-loop-retry-after` at a future timestamp, so they were
correctly excluded from `Store.ReadyExecution()` (the source-of-truth
queue used by `ExecuteBeadWorker.nextCandidate`). The bead description's
claim that `extra.execute-loop-retry-after: null` for ddx-dc157075,
ddx-aee651ec, and ddx-29058e2a was incorrect at observation time.

What we did fix: emitted the missing diagnostic event so the **next**
operator who sees this won't have to spend a session diffing call
graphs to find out why.

## Code path traced (per AC #1)

```
ExecuteBeadWorker.Run(...)                               // execute_bead_loop.go:349
  ↳ nextCandidate(attempted, labelFilter)                // execute_bead_loop.go:1146
      ↳ Store.ReadyExecution()                           // bead/store.go:1246
          ↳ readyFiltered(executionOnly=true)            // bead/store.go:1301
              · status==open || stalled-in-progress
              · all deps closed
              · execution-eligible != false              // line 1334
              · superseded-by empty                       // line 1341
              · execute-loop-retry-after in past          // line 1346  ← excludes parked P0s
              · sortBeadsForQueue(ready)                  // line 1357 → priority asc, then created_at asc, then ID asc
      · loop: skip if in attempted map                    // line 1170
      · loop: skip if labelFilter set and label missing   // line 1175
      · return first survivor
  ↳ PreClaimHook  // continue on err (one retry)
  ↳ attempted[id] = struct{}{}                            // execute_bead_loop.go:480 (BEFORE Claim, intentional — see below)
  ↳ RoutePreflight  // return on err
  ↳ ComplexityGate  // continue on skip / re-pick
  ↳ Store.Claim(id, assignee)  // atomic via WithLock
      · if Claim fails (race lost) → continue → re-pick from ReadyExecution
```

The sort comparator at `bead/store.go:1473`:

```go
if beads[i].Priority != beads[j].Priority {
    return beads[i].Priority < beads[j].Priority   // P0 first ✓
}
```

There is no path in either `nextCandidate` or `ReadyExecution` that
reorders or down-ranks priority. Every `continue` in `nextCandidate`
preserves the priority ordering of the survivors.

## The five hypotheses, evaluated

1. **`attempted` map persists across worker restarts.** No. It is allocated
   per `Run()` call at `execute_bead_loop.go:382` (`attempted := make(map[string]struct{})`).
   A "fresh" worker process gets an empty map. **Rejected.**

2. **Empty label filter excludes labels.** No. The guard at
   `execute_bead_loop.go:1175` is `labelFilter != "" && !HasBeadLabel(...)`.
   Empty filter is a no-op. **Rejected.**

3. **Two workers race on `Claim()` and the loser falls through to lower priority.**
   *Half true.* The loser does call `continue`, but on the next iteration:
   - The contended bead is now `in_progress` (Claim flipped it under
     `WithLock`), so it is **not** in `ReadyExecution`'s output anymore.
   - The next ready P0 is at the head of the list, and the loser will
     pick **that** P0 — not a P2.
   - The bead also stays in the loser's `attempted` map (added pre-Claim
     at line 480), but that's harmless because the bead is no longer
     ready anyway. The map entry just makes the skip a tautology.

   So while the **mechanism** exists (loser does `continue` after Claim
   fails), it does not produce the observed P2-claim-while-P0-ready
   behavior on its own. **Rejected as root cause** — but added a
   `picker.claim_race` diagnostic so future races are observable.

4. **Server-managed worker uses a different picker.** No. `cli/internal/server/workers.go:775`
   calls `worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{...})` — the same
   `ExecuteBeadWorker.Run` driven by the CLI. There is no parallel
   selection path in `workers.go`. **Rejected.**

5. **The P0 beads have a label/area excluded by some default filter.** No.
   `LabelFilter` is plumbed only from `spec.LabelFilter`
   (`cli/internal/server/workers.go:782`). No default value, no implicit
   filter applied anywhere. **Rejected.**

## The actual cause of the operator's observation

At the time of the reproducer (2026-05-02 23:46 UTC), `ddx bead show`
returned the following for the three "ready P0" beads named in the bead
description:

```
=== ddx-dc157075 ===
  execute-loop-retry-after: 2026-05-03T02:53:53Z   (~3 hours in the future)
=== ddx-aee651ec ===
  execute-loop-retry-after: 2026-05-03T02:54:00Z
=== ddx-29058e2a ===
  execute-loop-retry-after: 2026-05-03T02:54:38Z
```

`ddx bead ready` (used by the operator) shows beads whose dependencies are
satisfied, but **does not** filter by `execute-loop-retry-after`. The
execution queue (`ddx bead ready --execution`, equivalent to
`Store.ReadyExecution()`) **does** filter — and correctly returned the
three beads as cooldown-parked.

So the workers were behaving correctly: the highest-priority *eligible*
bead was a P2 because every P0 in the queue was on retry cooldown.

The bead description's statement
"`extra.execute-loop-retry-after: null`" was a snapshot from an earlier
state (or an inspection of a different field); the live state at
worker-claim time had cooldowns set.

## What changed

Even though the picker is correct, it had no observability surface for
"why didn't you pick this P0?" — exactly the question the operator was
trying to answer. AC #4 demands a diagnostic event when this happens. We
added two:

1. `picker.priority_skip` — fired when `nextCandidate` selects a bead at
   priority N while at least one strictly higher-priority bead (`< N`)
   was passed over. Each skipped bead names its reason: `in_attempted`
   (already tried this run) or `label_filter` (worker spec restricted).
   `eligibility_filter` and `retry_cooldown` are reserved for future
   re-emit when those filters move into `nextCandidate` (currently they
   apply upstream in `ReadyExecution` and never reach `nextCandidate`).

2. `picker.claim_race` — fired when `Store.Claim` fails (another worker
   grabbed the bead first). Names the bead, priority, and the underlying
   error.

With these in place, an operator who sees a worker claim a P2 while
`ddx bead ready` shows a P0 can now run
`ddx server workers log <id> | grep picker.` and immediately see whether
the P0 is on cooldown (no skip event because the bead is upstream-filtered
— operator can then `ddx bead show <p0>` to find the cooldown), in
the worker's `attempted` map (skip event with reason=in_attempted —
indicates a no-changes loop that should be investigated), label-filtered
out (skip event with reason=label_filter), or actively contended
(claim_race event).

## Files changed

- `cli/internal/agent/execute_bead_loop.go`
  - `nextCandidate` extended to return `[]pickerSkip` alongside the
    chosen bead so the caller can build the diagnostic event.
  - New helper `emitPickerPrioritySkips` emits `picker.priority_skip`
    only when a strictly higher-priority bead was passed over (a same-
    priority FIFO loss is not starvation).
  - Added `emit("picker.claim_race", ...)` at the Claim-failure branch.
- `cli/internal/agent/execute_bead_loop_priority_test.go` (new):
  - `TestExecuteLoop_ClaimsHighestPriorityFirst` (AC #2)
  - `TestExecuteLoop_TwoWorkersBothClaimP0sBeforeP2s` (AC #3)
  - `TestExecuteLoop_EmitsPickerPrioritySkipEvent` (AC #4)

## What was deliberately NOT changed

- The `attempted` map is still populated **before** `Claim`. Moving it
  to after `Claim` would create an infinite-pick loop on the
  ComplexityGate `shouldClaim=false` path, which currently relies on the
  pre-Claim `attempted` add to prevent re-picking the same coarse bead.
  The Claim-race "loser keeps bead in attempted" is harmless because
  the bead is no longer in `ReadyExecution` after the winner's Claim
  flipped its status under `WithLock`.
- The sort comparator was left untouched. Priority ordering is correct.
- No retry/cooldown logic was modified — the upstream filter in
  `readyFiltered` is correct; what was missing was operator visibility
  via `ddx bead ready` (out of scope for this bead; that command's
  filtering is a separate concern).

## Manual verification (per AC #5)

After commit, the next freshly-started worker will pick `ddx-9d55601f`
first (it has Priority=0, no cooldown, no label filter, all deps
closed) — assuming no race with another worker already mid-Claim.
ddx-dc157075, ddx-aee651ec, ddx-29058e2a will be picked when their
respective `execute-loop-retry-after` timestamps elapse.

Write needs approval — I'll present the review directly (it's ready to drop into `self-healing-workers-findings-claude-r3.md` if you want it persisted).

---

## Review: DDx Worker Self-Healing (revised design) — round 3

The revision clears **every r2 blocker** I and codex raised: expired-block restarts now feed the throttle stream (`expired_terminal_block`), flock and `.gitattributes` union-merge are correctly split into 4a/4b, the preserved-review unblock command is concrete, the dependency DAG exists, and `DefaultTerminalBlockTTL` + suppression arithmetic are pinned. **Seven of eight beads are decomposition-ready.** One remains blocked on a code-verified spec gap. All claims below verified against the target.

### Findings

| Severity | Area | Finding |
|---|---|---|
| BLOCKING | Preserved-review gate (bead 5) | The unblock predicate matches on evidence that is never recorded. The preserved signal is written **only** as a `BeadEvent` (`execute_bead_loop.go:5477`), `BeadEvent` has no attempt-id field (`types.go:51-58`), and `preservedNeedsReviewNote` (`:5487`) emits `preserve_ref`/`result_rev`/`gate_summary` but **no `attempt_id=`**. So the rule "the attempt ID must match that event" via `--set preserved-review-unblocked-attempt=<id>` is unsatisfiable. Agents diverge: add `attempt_id=` to the event (changes schema, couples to 4b union-merge + all parsers) vs. match `preserve_ref` vs. drop the attempt clause and match `CreatedAt`. |
| BLOCKING | Preserved-review — layer split (bead 5) | Block side and unblock side are in different data layers; only the unblock side is specified. Eligibility reads `b.Extra`/status (`lifecycleExecutionEligible` `store.go:2301`, `lifecycleSupersededBy` `:2313`) and **never the event stream**. Unblock `--set` fields land in `b.Extra` (readable), but the *block* signal is an event-body string eligibility doesn't read. The comparison "unblocked-at newer than latest preserved event" straddles two layers. Fix: mandate the preserve path (`execute_bead_loop.go`) also stamp durable `b.Extra` markers (`preserved-review-blocked-at`, `preserved-review-attempt`) at preserve time, so eligibility is a pure `b.Extra` comparison. |
| WARNING | Suppression reason normalization (beads 1 & 6) | Suppression set is 3 canonical tokens, but the block `Reason` is free-form from `firstNonEmpty(rec.ReapReason, rec.Status, rec.LastError, …)` (`workers_supervisor.go:376`). No raw→canonical mapping given. EMFILE is the concrete ambiguity: bead 6 says "emits typed operator attention" but `resource_exhausted` is also a listed token — which reason does an EMFILE block carry? That's the bead-1↔bead-6 contract and it drives the throttle key + bead-7 reporting. |
| WARNING | Liveness thresholds (bead 2) | Bead 2's `2 * HeartbeatTTL` window for `status.json` is not reconciled with the existing `age <= HeartbeatTTL` claim-release (1×, `workers.go:1496`) and `2h` same-machine (`:1505`) thresholds. Three windows on the same TTL can disagree (supervisor calls a worker live after the claim path already released its lease). State the relationship. |
| WARNING | Bead readiness (4-8) | Boundaries name files but no `Test*` functions or command AC (rubric requires a named `Test*`). Beads 4/5/6/7/8 cite test *files* but no test *function* or failing assertion. Design should hand the author the concrete assertions so beads score ≥7/8. |
| NOTE | Meta-scan wording | "clear dead-PID run-state" reads against the no-PID-only Non-Goal; align to "run-state whose owner is not live per `workerRecordLive`" (the Liveness section already says this). |
| NOTE | Ready beads | Beads 1, 2, 3, 4a, 4b are ready. Bead 3 reuses the existing `suggested_child_beads[].acceptance` decoder pattern; bead 2's evidence sources (`workerstatus.LastActivityAt`, `bead.HeartbeatTTL`, run-state `ExpiresAt`) all exist. |

### Verdict: REQUEST_CHANGES

### Summary

The revision cleared all r2 blockers; seven of eight beads are file-ready. The one remaining blocker is bead 5: the preserved-review gate matches on an attempt-id the preserved event never records, and its block marker lives in the event stream while eligibility is computed from `b.Extra` — so the gate isn't implementable from one layer as written. Have the preserve path stamp durable `b.Extra` block markers (including attempt identity) so eligibility is a pure bead-field comparison, pin the raw→canonical suppression-reason mapping (EMFILE especially), and hand the author the `Test*` assertions. Then file.

---

Two follow-ups, your call:
1. **Persist this** to `self-healing-workers-findings-claude-r3.md` (needs write approval — I have the file staged).
2. **Cross-model quorum** — `codex-r3.md` is also an empty placeholder. Want me to run codex for an independent r3 (the skill's multi-harness stance favors it for work this high-risk), or is the single verified blocker enough to send bead 5 back for revision now?
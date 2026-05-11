---
ddx:
  id: TD-031
  depends_on:
    - FEAT-004
    - SD-004
    - ADR-004
---
# Technical Design: Bead State Machine and Naming-Role Decisions

## Purpose

This TD is the canonical reference for what bead state means in DDx and where
each observed name in the codebase belongs. It exists because schema, docs,
code, and persisted data have drifted: the bead-record JSON Schema enumerates
six statuses (the bd/br canonical set), FEAT-004 still documents three, and
several legacy/backcompat names appear in code paths (`done`, `needs_human`,
`pending`, `ready`, `review`, `needs_investigation`) without a single decision
record explaining whether each is a status, a derived queue category, an
event kind, a label, or worker state.

The five in-flight hygiene beads each introduce new state-machine vocabulary:

- ddx-b24e9630 — `no_changes_*` outcome verification (NoChangesContract)
- ddx-3c154349 — auto-triage of stuck beads (TriageContract)
- ddx-aede917d — drain pause on quota exhaustion (QuotaPauseContract)
- ddx-c6e3db02 — rate-limit retry behavior (RateLimitRetryContract)
- ddx-da11a34a — main-git/tracker lock contention handling
  (LockContentionContract)

Without one TD that nails down the categories and the transition matrix,
those five beads will each independently invent overlapping vocabulary.

This TD authors the design only. The mechanical reconciliation
(schema-vs-FEAT-004 alignment, code rename of non-status names, CI guards,
hygiene-bead AC substitution, migration survey) is filed as sibling beads.

## Critical Constraint (per ADR-004)

The bead-record envelope must remain compatible with the bd/br interchange
contract. The persisted `status` enum is **fixed** at the bd/br canonical six
values:

```
open, in_progress, closed, blocked, proposed, cancelled
```

DDx-specific execution semantics live in **labels**, **events**, or the
preserved-extras **Extra** map — never in new statuses. Adding a new persisted
status requires upstream bd/br coordination plus an ADR-004 amendment. This
TD does not authorize any such addition.

## Today's Observed State (verified)

- `cli/internal/bead/schema/bead-record.schema.json` enumerates six values.
- `docs/helix/01-frame/features/FEAT-004-beads.md` line 65 still documents
  three values (`open, in_progress, closed`). FEAT-004 lags the schema.
- `ddx bead list --json | jq -r '.[].status' | sort -u` on real projects
  returns only `open`, `in_progress`, `closed` for active rows today —
  `blocked`, `proposed`, and `cancelled` are schema-allowed but unused in
  the current active queue.
- The legacy/backcompat names `done`, `needs_human`, `pending`, `ready`, `review`,
  `needs_investigation` appear in code paths but are **not** persisted bead
  statuses today. Each falls into one of the other categories defined below.

## Sections

The remainder of this TD is structured as the eleven design sections required
by the originating bead, followed by five per-hygiene-bead contract
subsections.

---

## 1. Category Taxonomy

Every state-machine name observed in DDx falls into exactly one of the
following categories. The category determines where the name is allowed to
live (schema, label namespace, event kind, queue derivation, or worker
process state).

| Category | Storage location | Lifecycle | Owner |
|---|---|---|---|
| Persisted bead status | `status` field on the bead record | Mutated by atomic snapshot rewrite | Bead store, locked to bd/br set |
| Derived queue category | Computed on read from status + deps + preserved metadata | Never persisted | Queue derivation code (`ddx bead ready/blocked/status`) |
| Event kind | Append-only entry in `Extra["events"][].kind` | Append-only; explains state, does not control lifecycle | Drain loop, agent service, CLI |
| Terminal phase | A persisted `closed` status plus a closing event/label that names *why* | Mutated once on close | Drain loop / CLI |
| Claim metadata | `assignee`, `claimed-at`, `claimed-pid` fields (preserved extras) | Set on claim, cleared on unclaim, expired by triage | Claim resolution path (TD-004) |
| Label | Entry in the `labels` array | Explains or filters state; never controls lifecycle | Anyone with `ddx bead update` |
| Extra metadata field | Arbitrary key under preserved extras | Explains or filters state; never controls lifecycle | Subsystem owning that key |
| Worker state | In-memory state of the drain process | Lives only for the worker's lifetime | Drain loop process |

A name MUST NOT span categories. If a name today appears as both a label and
a derived queue category (e.g., `ready` is derived; `ready` MUST NOT also
appear as a label), the sibling reconciliation bead removes the duplicate
usage.

## 2. Persisted Status Enumeration

The persisted bead status is exactly:

```
open | in_progress | closed | blocked | proposed | cancelled
```

This is the bd/br-canonical set; it matches `bead-record.schema.json`. This
TD does **not** add, rename, or remove any persisted status.

Plain-English semantics:

- `open` — accepted active work. No claim is active. This includes work that is
  waiting on dependencies; dependency waiting is derived from the dependency DAG
  while the persisted status remains `open`.
- `in_progress` — the bead has an active claim (`assignee`, `claimed-at`,
  `claimed-pid` populated). The drain loop or an operator has taken
  ownership.
- `closed` — terminal satisfied work. A `closed` dependency satisfies
  downstream beads. The reason for closure is encoded in a closing event and/or
  label; the status itself does not carry the reason.
- `blocked` — accepted work paused by a rare external, recheckable blocker.
  Dependency waits are not `status=blocked`; they are derived from unsatisfied
  dependencies while the bead remains `open`.
- `proposed` — operator decision required. Proposed beads are not
  autonomous-work eligible until an operator accepts, rewrites, splits, waives,
  or cancels them.
- `cancelled` — terminal not-doing. Distinct from `closed`: `cancelled` does
  not satisfy dependents unless a later explicit dependency policy says
  otherwise.

## 3. Transition Matrix

| From → To | Allowed? | Driver | Event fired |
|---|---|---|---|
| `proposed` → `open` | yes | operator (`ddx bead update --status open`) | `triaged` |
| `proposed` → `cancelled` | yes | operator | `cancelled` |
| `open` → `proposed` | yes | readiness or operator found missing decision input | `triage-ambiguous` / `review-manual-required` |
| `open` → `in_progress` | yes | drain loop or operator (claim) | `claimed` |
| `open` → `blocked` | yes | operator or auto-triage found an external recheckable blocker | `blocked` |
| `open` → `cancelled` | yes | operator | `cancelled` |
| `in_progress` → `open` | yes | unclaim (operator or stale-claim sweep) | `unclaimed` |
| `in_progress` → `closed` | yes | drain loop (on merge/already-satisfied) or operator | `closed-merged` / `closed-already-satisfied` |
| `in_progress` → `proposed` | yes | non-automatable review/readiness finding or exhausted repair/review budget | `review-block` / `review-manual-required` |
| `in_progress` → `blocked` | yes | drain loop or operator found an external recheckable blocker | `blocked` |
| `blocked` → `open` | yes | operator (block resolved) | `unblocked` |
| `blocked` → `cancelled` | yes | operator | `cancelled` |
| `closed` → * | no | — | — (closed is terminal) |
| `cancelled` → * | no | — | — (cancelled is terminal) |

Closed and cancelled are terminal. `closed` satisfies dependency edges;
`cancelled` does not satisfy dependency edges unless a future dependency policy
explicitly defines an exception. Re-opening a closed bead is not a transition;
it is filing a follow-up bead with `replaces` set.

### 3.1 Derived Queue Buckets

Persisted status is the sole DDx-owned lifecycle field. Queue buckets are
computed read models over status, dependency edges, claim metadata, and
preserved Extra fields:

- `execution-ready` — `status=open`, no active claim, every dependency is
  `closed`, and no execution-suppressing metadata is present.
- `dependency-waiting` — `status=open` with at least one dependency that is not
  `closed`; this is derived waiting, not `status=blocked`.
- `operator-review` — `status=proposed`; these beads require an operator
  decision before autonomous execution.
- `externally-blocked` — `status=blocked`; the blocker must be external and
  recheckable.
- `active` — `status=in_progress`; claim metadata names the current owner.
- `terminal-satisfied` — `status=closed`; this satisfies dependency edges.
- `terminal-not-doing` — `status=cancelled`; this does not satisfy dependency
  edges unless a future dependency policy explicitly says otherwise.

## 4. Event Vocabulary (unified)

Events are append-only entries on `Extra["events"]` (see TD-004 evidence
model). The kinds below form the closed vocabulary used across the five
hygiene beads. New kinds require updating this section.

Lifecycle events:

- `triaged` — `proposed → open`
- `claimed` — claim acquired
- `unclaimed` — claim released
- `blocked` — moved to `blocked` (carries a reason in `body`)
- `unblocked` — moved off `blocked`
- `closed-merged` — drain loop closed after a merge
- `closed-already-satisfied` — drain loop detected the work was already done
- `closed` — operator-driven close (catch-all)
- `cancelled` — moved to `cancelled`

Drain-outcome events (no_changes family — NoChangesContract):

- `no_changes_verified` — the attempt produced no commit, but supplied a
  `verification_command` that passed; the bead is already satisfied.
- `no_changes_unverified` — the attempt produced no commit and supplied a
  `verification_command`, but the command failed or could not run.
- `no_changes_unjustified` — the attempt produced no commit without enough
  structured rationale to prove either satisfaction or a durable blocker.
- `no_changes_needs_investigation` — legacy/backcompat event name for an
  attempt that explicitly asked for operator triage; maps to `status=proposed`
  when operator input is required.
- `no_changes_decomposed` — agent decomposed the bead instead of changing
  files.
- `no_changes_blocked` — agent declared no_changes with a justified external
  blocker.
- `no_changes_recoverable` — transient cause; retrying the same bead after
  time passes can plausibly succeed.

Drain-control events:

- `drain-paused-quota` — QuotaPauseContract: drain paused because the active
  harness reported quota exhaustion
- `drain-resumed-quota`
- `rate-limit-retry` — RateLimitRetryContract: a single attempt was retried
  after a rate-limit response
- `lock-contention` — LockContentionContract: store-lock contention was
  observed and handled (retry or backoff)

Review and triage events:

- `review-block` — reviewer raised a BLOCKING finding
- `review-pass` — reviewer cleared the change
- `review-request-changes` — one or more reviewer slots returned a structured
  `REQUEST_CHANGES` verdict; body carries the review group id and per-AC findings
- `review-fixable-gap` — reviewer found an implementation/test gap that can be
  repaired by another automated cycle
- `review-too-large` — reviewer or readiness found the bead/result too broad for a
  bounded review cycle
- `review-error` — reviewer invocation failed, was malformed, overflowed, or
  returned no parseable verdict
- `review-manual-required` — reviewer failures or non-automatable findings
  exhausted automatic recovery and require operator action
- `triage-decomposed` — readiness or review decomposed a parent into child beads
- `triage-overflow` — decomposition reached the depth cap
- `triage-rewritten` / `intake-rewritten` — readiness applied a validated
  replacement rewrite or metadata update before claim; body carries changed
  fields, rationale, before/after hashes or attachment pointers, and preservation
  evidence
- `triage-ambiguous` — readiness could not safely clarify the bead
- `auto-triage` — TriageContract: triage path mutated labels/status

Candidate-cycle events (FEAT-010 candidate-cycle pipeline):

- `candidate-pinned` — layer 2 pinned a candidate ref for the current cycle's
  `result_rev` before dispatching reviewers; body carries `candidate_ref`,
  `cycle_index`, and `attempt_id`
- `candidate-checks-failed` — post-run verification (tests, lint, gates) failed
  against the candidate; body carries check command and exit status
- `repair-cycle-started` — a repair cycle has been initiated in the still-live
  worktree; body carries `cycle_index`, `repair_context_from_review_group`, and
  `attempt_id`
- `repair-cycle-exhausted` — `repair_max_cycles` was reached for this attempt;
  bead moves to `status=proposed`
- `approved-land-conflict` — the candidate received unanimous `APPROVE` but merge
  to the base branch conflicted; bead is re-queued at its base revision without
  blame on the implementer or reviewer
- `final-result-landed` — the approved candidate has been merged to the base
  branch; immediately precedes the `closed-merged` lifecycle event

Auto-recovery events (ADR-024 P4; SD-025 Layer 3.5):

- `reframe-applied` — reframer rewrote bead description/AC; body carries `from_rev`, `to_rev`, and `reframer_cost`
- `decompose-applied` — decomposer filed child beads; body carries `child_ids` and `reframer_cost`
- `auto-recovery-failed` — both reframe and decompose attempts failed; body carries `reframe_attempt_cost` and `decompose_attempt_cost`
- `per-bead-budget-exhausted` — cumulative per-bead cost cap exceeded; body carries `total_cost`

Push-outcome events:

- `push-failed` — the commit was created but the push to the remote was rejected
  (branch protection, auth-token expiry, pre-push test failure, or similar);
  body carries stderr detail and base-rev. This outcome MUST NOT trigger
  `execute-loop-retry-after` — the cause requires operator action or a code fix,
  not time elapsed (see section 5 invariant).
- `push-conflict` — the push was rejected because the remote advanced since the
  claim was acquired (non-fast-forward); body carries the current remote head.
  A brief recheckable cooldown is appropriate (see section 5).

Each event SHOULD include `kind`, `actor`, `created_at`, and a free-form
`body` (TD-004 schema). Drain events SHOULD additionally include the
`attempt-id` of the execution attempt in `body` or in a structured `extra`
field.

## 5. Outcome → Label / Event / Extra Mapping

When a drain attempt finishes, `execute-bead` returns one of a fixed set of
outcomes plus optional no_changes lifecycle evidence. The mapping below is the
canonical queue-management contract.

Lifecycle reliability invariants:

- Raw `no_changes` is attempt evidence, not a durable bead queue state. The
  drain loop MUST translate it into one of the rows below before mutating the
  bead.
- `execute-loop-retry-after` may be set only when retrying the same bead after
  time passes could plausibly succeed without human/spec/dependency changes.
  Outcomes that MUST NOT set `execute-loop-retry-after` (cause is not
  time-resolvable; requires operator action or a code fix):
  `push_failed`, `declined_needs_decomposition`, `review_spec_gap`,
  `review_missing_acceptance`, and `review_too_large` at the decomposition depth
  cap.
  Outcomes that MAY set `execute-loop-retry-after` (recheckable by waiting):
  `push_conflict` (15 min — remote advanced; re-fetch and re-attempt resolves
  naturally), `no_viable_provider` (15 min — provider may become available),
  `land_conflict` (15 min — base branch may advance to a clean merge point), and
  transient infra/quota/transport (15 min — transient condition may clear).
  Cooldown lifetime SHOULD match the recheckable window; 15 min is the ceiling
  for every current recheckable outcome. A 24 h cooldown is never appropriate
  for any outcome in this table.
- Every bead excluded from ordinary `ddx work` execution MUST have an
  explainable durable reason using existing mechanisms: dependency edge,
  `proposed` status, external `blocked` status, `execution-eligible=false`,
  `superseded-by`, epic/parent queue mode, or an active retry cooldown. Labels,
  events, and Extra fields may explain that reason but do not control
  lifecycle.
- `closed` means implementation, verification, and the default adversarial
  pre-close review gate have all passed, or the work was verified as already
  satisfied. Review failure never reopens a closed bead; it prevents close.
- Automatic implementation retry is bounded and applies only to classifications
  where the implementer had valid task context and further automated work can
  plausibly resolve the finding. Spec gaps, missing acceptance criteria,
  decomposition overflow, and exhausted reviewer failures require operator
  resolution.
- Latest terminal events and close evidence beat stale `execute-loop-*` Extra
  metadata. Reconciliation may clear stale management fields, but MUST preserve
  append-only events and evidence.
- `ClosureGate` (store.go:1245, inside `closeWithEvidence`) applies exclusively
  to evidence-bearing execute-bead closes via `CloseWithEvidence`. The
  dependency-satisfied reconcile-close path (`applyReconcilePlan` at
  reconcile.go:208-254, `CloseSatisfied=true`) is a distinct meta-close that
  intentionally bypasses `ClosureGate`: the bead has no execution session and no
  `closing_commit_sha` of its own. The bypass is safe because every transitive
  dependency is `closed` — each having individually passed `ClosureGate` or been
  a prior meta-close — so the parent's closure inherits its evidence by reference
  through the dependency edges.

| Outcome or lifecycle action | Status transition | Label changes | Events appended | Extra updates |
|---|---|---|---|---|
| `review_pass` after merged candidate | `in_progress → closed` | remove `claimed`, add `last-merged-rev:<sha>` (optional) | `review-pass` + `closed-merged` | `Extra["last-run"]`, `Extra["last-review"]`, and `Extra["closing_commit_sha"]` updated; clear stale `execute-loop-*` management fields |
| `already_satisfied` | `in_progress → closed` | (none) | `closed-already-satisfied` | `Extra["last-run"]`; clear stale `execute-loop-*` management fields |
| `review_fixable_gap` with retry budget remaining | no durable status change if continuing immediately; otherwise `in_progress → open` (claim released) | no lifecycle label required | `review-fixable-gap` + retry decision | `Extra["last-review"]` carries findings ref; next cycle records `repair_context_from_review_group` |
| `review_fixable_gap` with retry budget exhausted | `in_progress → proposed` | optional explanatory review label only | `review-block` | `Extra["last-review"]` carries findings ref and exhausted budget |
| `review_spec_gap` / `review_missing_acceptance` | `in_progress → proposed` | add `triage:spec-gap` or `triage:missing-acceptance` as explanatory labels | `review-block` | `Extra["last-review"]` carries findings ref |
| `review_too_large` | `in_progress → open` after children and dependency edges are filed, or `in_progress → proposed` at depth cap/lossy split | add `decomposed` when children exist | `review-too-large` and optionally `triage-decomposed` / `triage-overflow` | `Extra["children"]` and AC mapping when decomposed |
| `review_error` below retry cap | no durable status change; retry reviewer for same `result_rev` | (none) | `review-error` | `Extra["last-review-error"]` carries class, reviewer slot, and attempt count |
| `review_error` exhausted | `in_progress → proposed` | optional explanatory review label only | `review-manual-required` | `Extra["last-review-error"]` carries class, reviewer slot, and attempt count |
| `execution_failed` | `in_progress → open` (claim released) | (none) | `unclaimed` + a structured failure event | `Extra["last-run"]` |
| verified no_changes already satisfied | `in_progress → closed` | remove no_changes triage labels if present | `no_changes_verified` + `closed-already-satisfied` | `Extra["last-run"]`; clear `execute-loop-retry-after`, `execute-loop-last-status`, and `execute-loop-last-detail` |
| unverified no_changes | `in_progress → open` (claim released) | add `triage:no-changes-unverified` | `no_changes_unverified` | record verification command/result; do not set retry cooldown by default |
| unjustified no_changes / no rationale | `in_progress → open` (claim released) | add `triage:no-changes-unjustified` | `no_changes_unjustified` | record rationale absence/detail; do not set retry cooldown by default |
| legacy no_changes investigation because work is too large | `in_progress → open` after orchestrator child beads and dependency edges are filed, or `in_progress → proposed` at queue-level depth cap/lossy split | add `decomposed` when children exist | legacy/backcompat `no_changes_needs_investigation` + `triage-decomposed` or `triage-overflow` | `Extra["last-rationale"]`, `Extra["children"]`, and AC mapping; do not set retry cooldown |
| legacy no_changes investigation for non-decomposition reason | `in_progress → proposed` when operator action is required before retry; otherwise `in_progress → open` (claim released) | optional explanatory triage labels only | legacy/backcompat `no_changes_needs_investigation` | `Extra["last-rationale"]`; do not set retry cooldown by default |
| parent/epic/decomposed container | `in_progress → open` with dependency edges to children when children were created; otherwise `in_progress → open` with `execution-eligible=false` | add `decomposed` when children exist | `no_changes_decomposed` or `triage-decomposed` | `Extra["children"]` lists child IDs plus AC mapping, or `Extra["execution-eligible"]=false` explains container-only work |
| external blocker | `in_progress → blocked` for hard external recheckable blockers, or `in_progress → open` for visible soft blockers | add `blocked-on-upstream:<id>` as explanatory label when useful | `no_changes_blocked` | `Extra["last-rationale"]` names the external blocker |
| superseded work | no terminal success; leave open only if visible history is needed | add no triage labels | structured superseded event if appended by caller | `Extra["superseded-by"]` names the replacement and makes ordinary execution ineligible |
| transient infra/quota/transport | `in_progress → open` (claim released) | (none) | `no_changes_recoverable`, `drain-paused-quota`, `rate-limit-retry`, or structured transport event | may set `execute-loop-retry-after` only for the retryable time-based condition |
| `push_failed` (branch protection, auth-token expiry, pre-push test failure, or executor data race) | `in_progress → open` (claim released) | optional explanatory label (e.g., `blocked-on-branch-protection`) | `push-failed` with stderr detail and base-rev | `Extra["last-run"]` only; DO NOT set `execute-loop-retry-after` — the cause requires operator action or a code fix, not time elapsed |
| `push_conflict` (remote advanced; fast-forward rejected) | `in_progress → open` (claim released) | (none) | `push-conflict` with current remote head | may set `execute-loop-retry-after` at 15 min (recheckable — remote advanced; re-fetch and re-attempt resolves naturally) |
| stale no_changes tracker metadata | no status change unless latest terminal evidence closes the bead | remove stale no_changes triage labels only when contradicted by terminal evidence | preserve historical events; append reconciliation event if performed | clear stale `execute-loop-*` management fields when latest terminal event or close evidence proves they are obsolete |
| dependency-satisfied reconcile close (`ddx bead reconcile`, `CloseSatisfied=true`; see reconcile.go:208-254) | `open → closed` via `UpdateWithLifecycleStatus`, `ManualClose=true`; `ClosureGate` is **not** invoked on this path (see store.go:1245 and the invariant above) | remove stale `execute-loop-*` labels if present; add `reconciled-nochanges-state` label | `lifecycle_reconciled` | `Extra["execute-loop-*"]` management fields cleared per `ReconcilePlan.ClearFields`; `externalizeEvents` called at reconcile.go:252 after close |
| `reframe_applied` — bead description and/or acceptance criteria were rewritten by the reframer agent; bead re-enters the execution-ready lane | `in_progress → open` (claim released; bead is immediately re-claimable) | add `reframed` label | `reframe-applied` with `from_rev`, `to_rev`, and `reframer_cost` in body | `Extra["consecutive_ladder_exhaustions"]` reset to 0; `Extra["last-recovery"]` updated with reframer run id and cost |
| `decompose_applied` — the decomposer agent filed 2–5 child beads; parent is left open but not execution-eligible until children are closed | `in_progress → open` with dependency edges to children filed; parent `execution-eligible=false` | add `decomposed` label | `decompose-applied` with `child_ids` list and `reframer_cost` in body | `Extra["children"]` lists child IDs and AC mapping; `Extra["execution-eligible"]=false`; `Extra["last-recovery"]` updated |
| `auto_recovery_failed` — both reframe and decompose attempts failed or the operator label `recovery:manual` was set; bead requires operator action | `in_progress → proposed` | add `triage:auto-recovery-failed` label | `auto-recovery-failed` with `reframe_attempt_cost` and `decompose_attempt_cost` (0 if not attempted) in body; `review-manual-required` is NOT fired here — this is a recovery, not a review | `Extra["last-recovery"]` records both attempt costs and failure reasons; DO NOT set `execute-loop-retry-after` |
| `per_bead_budget_exhausted` — cumulative per-bead cost for this bead exceeded the configured cap (`escalation.per_bead_budget_usd`) | `in_progress → open` (claim released; bead is re-claimable — budget exhaustion is recheckable, not terminal) | (none) | `per-bead-budget-exhausted` with `total_cost` in body | `Extra["last-run"]` only; DO NOT set `execute-loop-retry-after` — budget exhaustion requires operator action or config change, not time elapsed; see ADR-024 Per-Bead Budget |

Legacy/backcompat `needs_human` and `triage:needs-investigation` labels are not
lifecycle controls. New routing uses `status=proposed` for operator decisions;
those labels may remain only as migration metadata until cleanup removes them.

## 6. Claim Semantics

Claim is metadata, not status. The persisted-status convention is that
`in_progress` implies a claim is held; `open` implies no claim. The actual
claim fields are:

- `Extra["assignee"]` (string) — the entity holding the claim.
- `Extra["claimed-at"]` (RFC3339 timestamp) — when the claim was acquired.
- `Extra["claimed-pid"]` (int) — process id of the claim holder, advisory.

Acquired:

- During `open → in_progress` transition under the store lock.
- The drain loop and operator CLI (`ddx bead update --claim`) are the two
  legitimate drivers.

Released:

- Explicit `--unclaim` (or implicit unclaim on `in_progress → open`).
- On terminal close (`closed` / `cancelled`) the claim metadata is left in
  place as historical record but is no longer authoritative.

Worker shutdown and interruption:

- A worker that receives graceful shutdown while no bead attempt has reached a
  terminal mutation MUST release any active claim it owns before exiting. The
  bead returns `in_progress → open`, claim metadata is cleared, and an
  `unclaimed` event records `reason=worker_shutdown`.
- If the worker has already applied a terminal mutation (`closed`,
  `cancelled`, or `blocked`) for the current attempt, shutdown MUST NOT undo
  that terminal state. The worker may emit best-effort worker-disconnect
  telemetry, but bead state is already authoritative.
- If the child agent process or attempt worktree is interrupted by context
  cancellation, SIGTERM, SIGINT, or a server/operator cancel before a terminal
  mutation, the attempt is classified as mechanically disrupted. The worker
  preserves any available evidence, appends a structured attempt/interruption
  event, releases the claim, and leaves the bead re-claimable unless a separate
  explicit blocker or retryable cooldown was recorded.
- An ungraceful worker death may strand an `in_progress` bead temporarily. The
  stale-claim sweep is the recovery path: after the configured stale threshold,
  it releases the claim, appends the recovery event, and makes the bead
  queue-eligible again. This recovery MUST NOT delete the original attempt
  events or evidence bundle.
- Cooldown is not a shutdown-cleanup mechanism. A stopped or interrupted worker
  may set `execute-loop-retry-after` only when the recorded outcome is a
  retryable time-based condition; ordinary shutdown, SIGTERM/SIGINT, or
  operator cancel releases the claim without parking the bead behind time.

Stale claim handling (TriageContract owns the policy):

- A claim is *stale* if `claimed-at` is older than the configured stale
  threshold AND the bead is still `in_progress`.
- Stale claims are released by the auto-triage path (`auto-triage` event),
  status moves `in_progress → open`, and the bead becomes queue-eligible
  again.
- Auto-triage MUST NOT delete prior claim metadata; it appends an event
  recording the release.

### Auto-Recovery Role Catalogue

Two agent roles support the cross-cycle recovery path (ADR-024 P4; SD-025 Layer 3.5). These roles are dispatched by the drain proxy, not by an operator or agent tool directly.

| Role | `MinPower` floor | Claim held by | Output contract | Dispatch condition |
|---|---|---|---|---|
| `reframer` | Strong-tier per ADR-024 P3 — `MinPower` set above the current cycle's implementer actual power | Drain proxy (not the reframer agent); the reframer runs read-only against the bead record, then returns structured edits | Structured edits to bead description and/or acceptance criteria. Edits must preserve all explicit commitments (AC, non-scope, named files/tests, deps, governing artifact refs). A no-op result (no change) is valid and triggers the decomposer path. | `consecutive_ladder_exhaustions >= 2` and bead does not carry `recovery:manual` label |
| `decomposer` | Strong-tier per ADR-024 P3 — same floor as reframer | Drain proxy; the decomposer runs read-only against the bead record, then returns a list of child bead specs | List of 2–5 child bead specs for `Store.Create`. Each spec must include title, description, numbered AC, labels (inheriting parent's labels and `spec-id`), and a parent/dep edge to the parent bead. A no-op result triggers `auto_recovery_failed`. | Reframe attempt returned no change, or reframer invocation failed |

Both roles are dispatched with the same `role`, `bead_id`, `attempt_id`, `session_id`, and `review_group_id` correlation fields used by the reviewer role (ADR-024 Default Adversarial Pre-Close Review Gate). Operator-supplied passthrough constraints (`--harness`, `--provider`, `--model`) are forwarded unchanged. If those constraints prevent satisfying the strong-tier `MinPower` floor, the dispatch returns `readiness_error` and the outcome maps to `auto_recovery_failed`.

### `consecutive_ladder_exhaustions` Extra Field

`Extra["consecutive_ladder_exhaustions"]` is an integer counter maintained by the drain loop on each bead record.

- **Incremented** at the end of each drain cycle in which the bead's within-cycle escalation ladder was fully exhausted (all power levels tried, none produced a close or forward progress).
- **Reset to 0** when the bead is successfully closed, when a reframe or decompose pass fires (the bead's prompt has changed; start fresh), or when an operator explicitly clears the counter via `ddx bead update`.
- **Threshold:** when `consecutive_ladder_exhaustions >= 2` (default; configurable in `.ddx/config.yaml` as `escalation.auto_recovery_threshold`), the drain loop triggers the auto-recovery sequence described in ADR-024 Escalation Sequencing and SD-025 Layer 3.5.
- **Category:** Extra metadata field (section 1). This field explains and drives the auto-recovery trigger; it does not control persisted lifecycle status. The auto-recovery decision itself fires the `reframe_applied` or `decompose_applied` outcome, which changes status per section 5.

## 7. Naming-Role Decision Matrix

Every name observed in code, schema, docs, or persisted data is assigned
a single category here. Names not in the persisted-status set MUST NOT
appear as `status` values.

| Name | Category | Rationale |
|---|---|---|
| `open` | Persisted status | bd/br canonical; queue-eligible default. |
| `in_progress` | Persisted status | bd/br canonical; implies an active claim. |
| `closed` | Persisted status | bd/br canonical; terminal-success path. |
| `blocked` | Persisted status | bd/br canonical; accepted work paused by an external recheckable blocker. |
| `proposed` | Persisted status | bd/br canonical; operator decision required before autonomous work. |
| `cancelled` | Persisted status | bd/br canonical; terminal not-doing path that does not satisfy dependents. |
| `done` | Removed alias | Historical alias of `closed` in code paths. Not a persisted status. The reconciliation sibling bead replaces remaining code references with `closed`. |
| `pending` / `waiting` | Derived queue category | Used by queue derivation to mean "open AND has unmet deps"; this is derived dependency waiting, not `status=blocked`. Never persisted. |
| `ready` | Derived queue category | "open AND no unmet deps AND not claimed". Computed from status+deps+claim; never persisted. |
| `review` | Terminal phase / event | A *phase of work*, not a status. Implemented as the `review-block` / `review-pass` event pair plus review evidence. Never persisted as a status. |
| `needs_human` | Legacy/backcompat label | Migration-only signal formerly used for operator intervention. New lifecycle routing uses `status=proposed`; this label never controls lifecycle. |
| `needs_investigation` | Legacy/backcompat label | Migration-only signal formerly used for unclear cause. New lifecycle routing uses `status=proposed` when operator action is required. |
| `blocked-on-upstream:<id>` | Label | Parameterized label naming an external upstream blocker. Distinct from derived dependency waiting and from the `blocked` status; it explains state but does not control lifecycle. |
| `decomposed` | Label | Set by drain on `no_changes_decomposed`; pairs with `Extra["children"]`. |
| `triage` | Label | Set by drain on `no_changes_no_evidence` or by auto-triage. |
| `idle` / `draining` / `paused-quota` / `paused-rate-limit` / `exiting` | Worker state | Lives in the drain loop process, not on the bead. See section 10. |

## 8. Per-Hygiene-Bead Contracts

Each hygiene bead's AC must cite this section. Anything not in the
contract here is out of scope for that bead.

### 8.1 NoChangesContract (ddx-b24e9630)

NoChangesContract outcomes use the canonical mapping in section 5. This section
exists only to bind the hygiene bead to that mapping; it does not define a
second disposition table.

Claim behavior: the claim is released for every no_changes action that leaves
the bead `open`. It is not released separately when the same mutation moves the
bead directly to terminal `closed` or hard `blocked`.

Loop interaction: the try package parses and verifies no_changes rationale,
then returns the lifecycle action. The drain loop applies section 5 exactly and
appends one of the no_changes event kinds from section 4; it does not invent
new event kinds or use cooldown as a generic parking lot.

### 8.2 Bead Readiness Assessment And Triage Contract (ddx-3c154349)

Bead readiness assessment is the canonical pre-claim decision for
actionability and scope. It owns the readiness queue decision and runs before a
worker owns the bead, so most readiness actions start from `open`.
Lint/rubric scoring is the diagnostic pass inside readiness, and post-attempt
triage is a separate after-evidence queue action. The implementation entrypoint
may still be named `MODE: intake` for compatibility, but that is legacy
wording only; the product concept is bead readiness assessment.

Status transitions used:

- `in_progress → open` when releasing stale claims.
- `open → open` when readiness applies a validated replacement rewrite or
  metadata-only safe improvement before implementation. The bead remains
  execution-ready unless a later readiness decision parks it.
- `open → blocked` when triage decides a bead has an external recheckable
  blocker.
- `open → open` when readiness decomposes a parent and adds dependency edges to
  children; the parent is dependency-waiting but remains `open`.
- `open → proposed` when readiness reaches decomposition depth overflow or finds
  ambiguity that cannot be safely rewritten.

For successful replacement rewrites, the bead body may be materially shorter or
longer than the original when prompt fitness requires it. Preservation is proven
by the `triage-rewritten` / `intake-rewritten` evidence record and durable
anchors, not by keeping old text inside the prompt body. For rejected rewrites,
readiness moves the bead to `status=proposed` instead of releasing it back into
the execution-ready lane.

Labels added: `triage`, `blocked-on-upstream:<id>`, `decomposed`,
`triage:spec-gap`, `triage:missing-acceptance` (as applicable).
Legacy/backcompat labels such as `needs_human` and `needs-human-decomposition`
may be read during migration but must not be added as lifecycle controls.

Labels removed: `triage` after a triaged bead is reclaimed cleanly (the
reverse mutation of `triage` is "next successful claim removes the
label").

Events fired: `auto-triage`, `triage-ambiguous`, `triage-decomposed`,
`triage-overflow`, `unclaimed`, optionally `blocked`.

Claim behavior: the auto-triage sweep is the only path that releases a
claim it does not own. It MUST log the prior `assignee` and `claimed-at`
in the appended event body.

Loop interaction: readiness and triage run out-of-band of execute-bead. They
MUST NOT race with an active drain attempt holding the store lock — they
acquire the same store lock and releases-then-reclaims is not their job.

### 8.3 QuotaPauseContract (ddx-aede917d)

Status transitions used: none. Quota is a worker-state concern, not a
bead-state concern.

Labels: none added on the affected bead. The current attempt is treated
as `execution_failed` (claim released, status returns to `open`).

Events fired: `drain-paused-quota` is appended to a worker-scoped event
log (drain-process record), not to the bead's event stream. A
`unclaimed` event is appended to the bead.

Claim behavior: the claim on the in-flight bead is released cleanly so
another worker (or a later resumed worker) can pick it up.

Loop interaction: the worker transitions to worker-state
`paused-quota` (section 10). It stops claiming new beads until the
configured backoff elapses or an explicit resume signal arrives.

### 8.4 RateLimitRetryContract (ddx-c6e3db02)

Status transitions used: none. Rate-limit retry is internal to a single
attempt; the bead remains `in_progress` throughout.

Labels: none.

Events fired: `rate-limit-retry` per retry, on the bead's event stream.
Each event carries the retry count and the wait duration in the body.

Claim behavior: claim is held continuously across retries.

Loop interaction: rate-limit handling is bounded by the retry budget; on
budget exhaustion the outcome becomes `execution_failed` and the
standard mapping in section 5 applies. The worker briefly enters
`paused-rate-limit` for the wait window, then returns to `draining`.

### 8.5 LockContentionContract (ddx-da11a34a)

Status transitions used: none. Lock contention is a main-git/tracker
coordination concern, not a bead-state concern.

Labels: none.

Events fired: `lock-contention` (append-only, on the affected bead, only
after the contention has been resolved and the mutation succeeded — so a
future audit can see that contention was observed). Pure read-side
contention does not emit an event.

Claim behavior: unaffected. The claim is acquired or not; partial states
are not persisted.

Loop interaction: the worker retries the locked operation with
exponential backoff up to a bounded budget. On budget exhaustion the
calling outcome maps to `execution_failed` (section 5). The worker does
not enter a dedicated worker-state for lock contention; it remains
`draining` and treats the failure as ordinary.

Filesystem-shape contract: the main-git/tracker lock path
`.ddx/.git-tracker.lock` is a process-shared lock **directory**, not a
regular lockfile. Lock acquisition MUST classify the existing path
immediately after `mkdir` reports that it already exists and MUST NOT
sleep/back off until the path is confirmed to be a real lock directory.

- Missing after race: retry acquisition immediately.
- Directory: apply the existing PID/`acquired_at` stale-lock policy. A
  directory owned by a live process remains ordinary lock contention.
- Stale regular file: if and only if `lstat` reports an exact regular
  file and its mtime is older than the stale-lock threshold, remove it
  with single-path removal and retry acquisition immediately.
- Fresh regular file: fail fast with a malformed-lock diagnostic. Do not
  wait for the lock-contention retry budget and do not report
  `owner pid: unknown`.
- Symlink, socket, device, or other special file: fail fast with a
  malformed-lock diagnostic and do not remove it.

Malformed lock paths are operator/remediation diagnostics, not lock
contention. They do not emit `lock-contention`, do not introduce a new
status or label, and do not change claim semantics. If a malformed path
is surfaced through `ddx work` pre-claim guarding, the operator-facing
message MUST name the malformed path and expected directory shape rather
than repeatedly skipping every candidate behind a retry timeout.

## 9. Future-Change Process

This is the rule of record for any subsequent change to bead state.

> Any bead that introduces or changes a label, an event kind, an outcome →
> state mapping, claim handling, or worker-state semantics MUST cite the
> TD-031 section that authorizes the change. If no section authorizes it,
> TD-031 is amended in the same PR (or a parent bead) before the dependent
> work lands.
>
> Adding a new persisted bead status is **not** authorized by TD-031.
> It requires upstream bd/br coordination AND an amendment to ADR-004.

In practice:

- A new event kind: amend section 4 in the same PR.
- A new outcome → state mapping: amend section 5.
- A new label: amend section 7.
- A new worker state: amend section 10.
- A new persisted status: do not start the work; file an ADR-004
  amendment first.

CI guard: a sibling bead adds a check that any change to
`bead-record.schema.json` or to the persisted-status enum touches
ADR-004 and TD-031 in the same commit. That guard is out of scope for
this bead.

## 10. Worker-State Enumeration

Worker state is the in-process state of the drain loop. It is **distinct
from bead state** and is not persisted in the bead store. It exists only
for the worker's lifetime.

| Worker state | Meaning | Entry | Exit |
|---|---|---|---|
| `idle` | Worker is up but not actively claiming. | Worker startup; transient between attempts. | Operator or scheduler moves it to `draining`. |
| `draining` | Worker is actively claiming and executing beads. | Default operating state. | Quota or rate-limit pause; explicit stop. |
| `paused-quota` | Worker has observed quota exhaustion on its harness. | `drain-paused-quota` event. | `drain-resumed-quota` event after backoff or operator resume. |
| `paused-rate-limit` | Worker is sleeping out a rate-limit retry window inside an attempt. | Rate-limit response from harness. | Wait window elapses; worker resumes the same attempt. |
| `exiting` | Worker is shutting down cleanly; will not claim more beads. | Operator stop or terminal failure. | Process exit. |

Worker state is observable via the worker's stdout/log; it is not stored
on any bead. Hygiene beads that affect worker state (QuotaPauseContract,
RateLimitRetryContract) MUST NOT add new fields to the bead schema to
represent these transient states.

## 11. One-Way Lifecycle Migration And Startup Gate

The transition from label-owned lifecycle lanes to the status-owned lifecycle
is one-way. Legacy/backcompat labels and pseudo-statuses such as
`needs_human`, `triage:needs-investigation`, and `needs_investigation` are
migration input only. Normal runtime MUST NOT maintain compatibility lanes,
runtime aliases, or fallback queue behavior for those names after the lifecycle
gate is enabled.

DDx startup MUST refuse normal operation when the active project bead queue
contains unmigrated lifecycle state. The preflight scans the active queue before
ordinary commands load queue views or mutate beads. It fails closed when it
finds any of:

- open or in-progress beads carrying legacy lifecycle labels
  (`needs_human`, `triage:needs-investigation`, or equivalent old operator-lane
  labels);
- non-canonical pseudo-status values outside the six persisted statuses;
- legacy Extra fields that still control routing instead of merely preserving
  historical evidence.

Allowed bypass surfaces are intentionally narrow:

- `ddx help`, `ddx --help`, `ddx version`, and equivalent metadata-only help
  commands;
- `ddx doctor` and other read-only diagnostics that do not derive worker
  eligibility or mutate lifecycle;
- `ddx bead migrate --lifecycle --dry-run`;
- `ddx bead migrate --lifecycle --apply`.

All other bead and worker surfaces MUST refuse to continue until migration is
complete, including `ddx bead ready`, `ddx bead blocked`, `ddx bead status`,
`ddx work`, server-managed workers, GraphQL worker starts, REST/MCP worker
starts, and queue-readiness APIs. The refusal is a startup/configuration error,
not a retryable worker outcome and not an agent routing failure.

The startup error output MUST include:

1. counts of legacy lifecycle labels by name;
2. counts of non-canonical pseudo-status values by name;
3. the first few affected bead IDs for operator orientation;
4. the exact commands:

   ```bash
   ddx bead migrate --lifecycle --dry-run
   ddx bead migrate --lifecycle --apply
   ```

5. the rollback instruction: use git rollback (for example, restore the prior
   `.ddx/beads.jsonl` commit) if the one-way migration result is wrong.

Because the bead store is git-tracked, rollback is git rollback, not dual
semantics. The migrator owns translating old labels and pseudo-statuses into
`status=proposed`, `status=open`, `status=blocked`, `status=closed`, or
`status=cancelled` according to this TD. Runtime code consumes only the
post-migration state-owned contract.

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
several additional names appear in code paths (`done`, `needs_human`,
`pending`, `ready`, `review`, `needs_investigation`) without a single decision
record explaining whether each is a status, a derived queue category, an
event kind, a label, or worker state.

The five in-flight hygiene beads each introduce new state-machine vocabulary:

- ddx-b24e9630 — `no_changes_*` outcome verification (NoChangesContract)
- ddx-3c154349 — auto-triage of stuck beads (TriageContract)
- ddx-aede917d — drain pause on quota exhaustion (QuotaPauseContract)
- ddx-c6e3db02 — rate-limit retry behavior (RateLimitRetryContract)
- ddx-da11a34a — store-lock contention handling (LockContentionContract)

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
- The names `done`, `needs_human`, `pending`, `ready`, `review`,
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
| Derived queue category | Computed on read from status + labels + deps | Never persisted | Queue derivation code (`ddx bead ready/blocked/status`) |
| Event kind | Append-only entry in `Extra["events"][].kind` | Append-only | Drain loop, agent service, CLI |
| Terminal phase | A persisted `closed` status plus a closing event/label that names *why* | Mutated once on close | Drain loop / CLI |
| Claim metadata | `assignee`, `claimed-at`, `claimed-pid` fields (preserved extras) | Set on claim, cleared on unclaim, expired by triage | Claim resolution path (TD-004) |
| Label | Entry in the `labels` array | Add/remove during normal mutation | Anyone with `ddx bead update` |
| Extra metadata field | Arbitrary key under preserved extras | Free-form per consumer | Subsystem owning that key |
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

- `open` — the bead exists and is eligible for queue derivation. No claim is
  active.
- `in_progress` — the bead has an active claim (`assignee`, `claimed-at`,
  `claimed-pid` populated). The drain loop or an operator has taken
  ownership.
- `closed` — the bead is terminal. The reason for closure is encoded in a
  closing event and/or label; the status itself does not carry the reason.
- `blocked` — the bead cannot progress because of a hard precondition (an
  upstream dependency, an external blocker). The reason is encoded in a
  label (`blocked-on-upstream:<id>`) or an event.
- `proposed` — the bead is captured but has not been accepted into the
  active queue. Used for triage backlogs; not consumed by drain.
- `cancelled` — the bead was abandoned without completion. Distinct from
  `closed`: `cancelled` means "we are not doing this work"; `closed` means
  "the work is done (or determined unnecessary)".

## 3. Transition Matrix

| From → To | Allowed? | Driver | Event fired |
|---|---|---|---|
| `proposed` → `open` | yes | operator (`ddx bead update --status open`) | `triaged` |
| `proposed` → `cancelled` | yes | operator | `cancelled` |
| `open` → `in_progress` | yes | drain loop or operator (claim) | `claimed` |
| `open` → `blocked` | yes | operator or auto-triage | `blocked` |
| `open` → `cancelled` | yes | operator | `cancelled` |
| `in_progress` → `open` | yes | unclaim (operator or stale-claim sweep) | `unclaimed` |
| `in_progress` → `closed` | yes | drain loop (on merge/already-satisfied) or operator | `closed-merged` / `closed-already-satisfied` |
| `in_progress` → `blocked` | yes | drain loop on non-automatable review/readiness finding, or operator | `review-block` / `blocked` |
| `blocked` → `open` | yes | operator (block resolved) | `unblocked` |
| `blocked` → `cancelled` | yes | operator | `cancelled` |
| `closed` → * | no | — | — (closed is terminal) |
| `cancelled` → * | no | — | — (cancelled is terminal) |
| any → `proposed` | no | — | proposed is an entry-only status |

Closed and cancelled are terminal. Re-opening a closed bead is not a
transition; it is filing a follow-up bead with `replaces` set.

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
- `no_changes_needs_investigation` — the attempt explicitly asked for operator
  triage with `status: needs_investigation`.
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
- `triage-ambiguous` — readiness could not safely clarify the bead
- `auto-triage` — TriageContract: triage path mutated labels/status

Each event SHOULD include `kind`, `actor`, `created_at`, and a free-form
`body` (TD-004 schema). Drain events SHOULD additionally include the
`run-id` of the execution attempt in `body` or in a structured `extra`
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
- Every bead excluded from ordinary `ddx work` execution MUST have an
  explainable durable reason using existing mechanisms: dependency edge,
  `blocked` status, `needs_human`/triage labels, `execution-eligible=false`,
  `superseded-by`, epic/parent queue mode, or an active retry cooldown.
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

| Outcome or lifecycle action | Status transition | Label changes | Events appended | Extra updates |
|---|---|---|---|---|
| `review_pass` after merged candidate | `in_progress → closed` | remove `claimed`, add `last-merged-rev:<sha>` (optional) | `review-pass` + `closed-merged` | `Extra["last-run"]`, `Extra["last-review"]`, and `Extra["closing_commit_sha"]` updated; clear stale `execute-loop-*` management fields |
| `already_satisfied` | `in_progress → closed` | (none) | `closed-already-satisfied` | `Extra["last-run"]`; clear stale `execute-loop-*` management fields |
| `review_fixable_gap` with retry budget remaining | no durable status change if continuing immediately; otherwise `in_progress → open` (claim released) | add no human label unless retry budget exhausted | `review-fixable-gap` + retry decision | `Extra["last-review"]` carries findings ref; next cycle records `repair_context_from_review_group` |
| `review_fixable_gap` with retry budget exhausted | `in_progress → blocked` | add `needs_human` | `review-block` | `Extra["last-review"]` carries findings ref and exhausted budget |
| `review_spec_gap` / `review_missing_acceptance` | `in_progress → blocked` | add `needs_human`, `triage:spec-gap` or `triage:missing-acceptance` | `review-block` | `Extra["last-review"]` carries findings ref |
| `review_too_large` | `in_progress → blocked` after children are filed, or `in_progress → blocked` with `needs-human-decomposition` at depth cap | add `decomposed` when children exist; otherwise add `needs_human` | `review-too-large` and optionally `triage-decomposed` / `triage-overflow` | `Extra["children"]` and AC mapping when decomposed |
| `review_error` below retry cap | no durable status change; retry reviewer for same `result_rev` | (none) | `review-error` | `Extra["last-review-error"]` carries class, reviewer slot, and attempt count |
| `review_error` exhausted | `in_progress → blocked` | add `needs_human` | `review-manual-required` | `Extra["last-review-error"]` carries class, reviewer slot, and attempt count |
| `execution_failed` | `in_progress → open` (claim released) | (none) | `unclaimed` + a structured failure event | `Extra["last-run"]` |
| verified no_changes already satisfied | `in_progress → closed` | remove no_changes triage labels if present | `no_changes_verified` + `closed-already-satisfied` | `Extra["last-run"]`; clear `execute-loop-retry-after`, `execute-loop-last-status`, and `execute-loop-last-detail` |
| unverified no_changes | `in_progress → open` (claim released) | add `triage:no-changes-unverified` | `no_changes_unverified` | record verification command/result; do not set retry cooldown by default |
| unjustified no_changes / no rationale | `in_progress → open` (claim released) | add `triage:no-changes-unjustified` | `no_changes_unjustified` | record rationale absence/detail; do not set retry cooldown by default |
| no_changes needs investigation because work is too large | `in_progress → blocked` after orchestrator child beads are filed, or `in_progress → blocked` with `needs-human-decomposition` at queue-level depth cap | add `decomposed` when children exist; add `needs_human` only for lossy/ambiguous/depth-overflow cases | `no_changes_needs_investigation` + `triage-decomposed` or `triage-overflow` | `Extra["last-rationale"]`, `Extra["children"]`, and AC mapping; do not set retry cooldown |
| no_changes needs investigation for non-decomposition reason | `in_progress → open` (claim released) | add `triage:needs-investigation`; add `needs_human` when operator action is required before retry | `no_changes_needs_investigation` | `Extra["last-rationale"]`; do not set retry cooldown by default |
| parent/epic/decomposed container | `in_progress → blocked` when children were created; otherwise `in_progress → open` with `execution-eligible=false` | add `decomposed` when children exist | `no_changes_decomposed` or `triage-decomposed` | `Extra["children"]` lists child IDs plus AC mapping, or `Extra["execution-eligible"]=false` explains container-only work |
| external blocker | `in_progress → blocked` for hard blockers, or `in_progress → open` with a `blocked-on-upstream:<id>` label for visible soft blockers | add `needs_human` or `blocked-on-upstream:<id>` | `no_changes_blocked` | `Extra["last-rationale"]` names the external blocker |
| superseded work | no terminal success; leave open only if visible history is needed | add no triage labels | structured superseded event if appended by caller | `Extra["superseded-by"]` names the replacement and makes ordinary execution ineligible |
| transient infra/quota/transport | `in_progress → open` (claim released) | (none) | `no_changes_recoverable`, `drain-paused-quota`, `rate-limit-retry`, or structured transport event | may set `execute-loop-retry-after` only for the retryable time-based condition |
| stale no_changes tracker metadata | no status change unless latest terminal evidence closes the bead | remove stale no_changes triage labels only when contradicted by terminal evidence | preserve historical events; append reconciliation event if performed | clear stale `execute-loop-*` management fields when latest terminal event or close evidence proves they are obsolete |

`needs_human` is a **label**, not a status. It is the standard signal that
a human operator should look at the bead before drain re-attempts it.

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

## 7. Naming-Role Decision Matrix

Every name observed in code, schema, docs, or persisted data is assigned
a single category here. Names not in the persisted-status set MUST NOT
appear as `status` values.

| Name | Category | Rationale |
|---|---|---|
| `open` | Persisted status | bd/br canonical; queue-eligible default. |
| `in_progress` | Persisted status | bd/br canonical; implies an active claim. |
| `closed` | Persisted status | bd/br canonical; terminal-success path. |
| `blocked` | Persisted status | bd/br canonical; hard precondition unmet. |
| `proposed` | Persisted status | bd/br canonical; pre-triage backlog. |
| `cancelled` | Persisted status | bd/br canonical; terminal-abandoned path. |
| `done` | Removed alias | Historical alias of `closed` in code paths. Not a persisted status. The reconciliation sibling bead replaces remaining code references with `closed`. |
| `pending` | Derived queue category | Used by queue derivation to mean "open AND has unmet deps" (a synonym for unsatisfied dependencies). Never persisted. The reconciliation sibling bead either renames it to `waiting` or folds it into `blocked` derivation. |
| `ready` | Derived queue category | "open AND no unmet deps AND not claimed". Computed from status+deps+claim; never persisted. |
| `review` | Terminal phase / event | A *phase of work*, not a status. Implemented as the `review-block` / `review-pass` event pair plus the `needs_human` label. Never persisted as a status. |
| `needs_human` | Label | Signals "an operator must intervene". Stays a label per bd/br compatibility — never a status. |
| `needs_investigation` | Label | Signals "the cause is unclear; triage required". Stays a **label** per bd/br compatibility. The reconciliation sibling bead removes any code path that treats it as a status. |
| `blocked-on-upstream:<id>` | Label | Parameterized label naming the upstream blocker. Distinct from the `blocked` status: a bead can carry the label while remaining `open` for visibility, or while `blocked` for enforcement. |
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

### 8.2 Bead Readiness And Triage Contract (ddx-3c154349)

Bead readiness assessment is the normal path for actionability and scope
checks. It owns the readiness queue decision and runs before a worker owns the
bead, so most readiness actions start from `open`. The implementation
entrypoint may still be named `MODE: intake` for compatibility, but that is
legacy wording only; the product concept is bead readiness assessment. Within
readiness, lint/rubric scoring is a diagnostic pass, not a separate queue
action. Post-attempt triage is a separate after-evidence queue action that
reuses the same event taxonomy without changing the readiness decision.

Status transitions used:

- `in_progress → open` when releasing stale claims.
- `open → blocked` when triage decides a bead has an unmet hard
  precondition.
- `open → blocked` when readiness decomposes a parent, reaches decomposition depth
  overflow, or finds ambiguity that cannot be safely rewritten.

Labels added: `triage`, `needs_human`, `blocked-on-upstream:<id>`,
`decomposed`, `needs-human-decomposition`, `triage:spec-gap`,
`triage:missing-acceptance` (as applicable).

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

Status transitions used: none. Lock contention is a store-access
concern, not a bead-state concern.

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

## 11. Migration Plan (placeholder)

The actual migration survey — which existing beads carry orphaned status
values, which code paths persist non-canonical statuses, and what the
mechanical rename looks like — runs in the sibling reconciliation bead.
This TD does not catalogue current data.

Sibling beads handling the migration:

- Schema/docs/code reconciliation: align FEAT-004 line 65 with the
  schema; rename code references to non-status names; add CI guards.
- Hygiene-bead AC substitution: replace each hygiene bead's
  contract section with a reference to TD-031 §8.x.
- Migration survey: scan persisted bead stores in known DDx projects for
  non-canonical status values and produce a remediation list.

When those siblings land, this section is replaced with a concrete
migration record citing the sibling bead IDs and the survey result.

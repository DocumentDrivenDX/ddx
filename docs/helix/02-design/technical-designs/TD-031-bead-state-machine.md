---
ddx:
  id: TD-031
  depends_on:
    - TD-027
    - ADR-004
    - ADR-024
  related:
    - SD-025
    - FEAT-010
---
# Technical Design: Drain-Loop Operational Contract over Beads

## Purpose

This TD specifies the **operational contract** by which the DDx drain loop / executor consumes the bead lifecycle defined in [TD-027](TD-027-bead-collection-abstraction.md). It is the policy layer between bead storage primitives and the agent execution machinery.

**Scope:**

1. Persisted status enumeration (§2) — the canonical bead lifecycle values this operational layer may consume.
2. The outcome → state mapping table (§3) — what the drain loop does with each `execute-bead` outcome.
3. Worker-state enumeration (§4) — the in-process state of the drain loop, distinct from bead state.
4. Auto-recovery role catalogue (§5) — when and how the drain proxy dispatches reframer/decomposer roles.
5. `consecutive_ladder_exhaustions` policy (§6) — when the counter is incremented, reset, and threshold-triggered.
6. Per-hygiene-bead contracts (§7) — the operational bindings for in-flight beads (NoChanges, Triage, QuotaPause, RateLimit, LockContention).
7. Future-change process for operational contract changes (§8).

**Out of scope:**

- The bead state machine itself (statuses, transitions, queue buckets) — TD-027 §1–§4.
- The bead data model — TD-027 §11.
- Claim semantics — TD-027 §12.
- Event vocabulary — TD-027 §13 (the controlled list of event kinds).
- Storage interface, Operation pattern, module boundary — TD-027 §5–§10, §21.

**Reading order**: read TD-027 first to understand the bead substrate. This TD then specifies what the drain loop does to/with that substrate.

## Critical Constraint

The persisted bead status enum is fixed at the six bd/br canonical values (TD-027 §1). This TD does not authorize new statuses. DDx-specific execution semantics live in **labels**, **events**, or the preserved `extra` map per the categorization in TD-027 §3.

## 1. Background

This TD exists because schema, docs, code, and persisted data had drifted: the bead-record JSON Schema enumerated six statuses, FEAT-004 documented three, and several legacy/backcompat names (`done`, `needs_human`, `pending`, `ready`, `review`, `needs_investigation`) appeared in code paths without a single decision record explaining where each belonged. TD-027 now owns the canonical taxonomy. This TD owns the layer above: what the drain loop does with the bead lifecycle in operational terms.

Several hygiene beads introduce drain-loop vocabulary that this TD nails down:

- ddx-b24e9630 — `no_changes_*` outcome verification (NoChangesContract)
- ddx-3c154349 — auto-triage of stuck beads (TriageContract)
- ddx-aede917d — drain pause on quota exhaustion (QuotaPauseContract)
- ddx-c6e3db02 — rate-limit retry behavior (RateLimitRetryContract)
- ddx-da11a34a — main-git/tracker lock contention handling (LockContentionContract)

Without one normative TD for the operational contract, these beads would independently invent overlapping vocabulary.

## 2. Persisted Status Enumeration

The persisted bead status enum is inherited from TD-027 and ADR-004. This TD may map execution outcomes onto those values, but it MUST NOT authorize additional lifecycle statuses.

open | in_progress | closed | blocked | proposed | cancelled

New DDx execution semantics must use labels, events, or `extra` metadata unless ADR-004 and TD-027 are amended first.

## 3. Outcome → Label / Event / Extra Mapping

When a drain attempt finishes, `execute-bead` returns one of a fixed set of outcomes plus optional no_changes lifecycle evidence. The mapping below is the canonical queue-management contract. For details on how closures work, see §3.1 (reconcile-close vs evidence-close).

### 3.1 Bead Closure Paths: Reconcile-Close vs Evidence-Close

DDx supports two distinct paths to transition a bead to `closed` status, each with different prerequisites and safety guarantees:

**Reconcile-Close** (`UpdateWithLifecycleStatus` with `ManualClose=true`, invoked by `bead.Reconcile` when `CloseSatisfied=true`):
- **When used**: When all transitive dependencies are satisfied (closed), the parent bead is automatically closed to reflect the completion of its dependency tree.
- **Execution evidence**: The bead has no execution session and no `closing_commit_sha` of its own. Instead, closure is justified by reference — each transitive dependency has individually either passed `ClosureGate` (evidence-close) or been a prior reconcile-close, so the parent's closure inherits evidence by reference through the dependency edges.
- **ClosureGate bypass**: Reconcile-close intentionally bypasses `ClosureGate` because the evidence is carried in the dependency graph, not in the bead's own fields. This bypass is safe by design: a bead cannot transition to `closed` via reconcile unless every transitive dependency is `closed`.
- **Code locations**: `reconcile.go:244-250` (`UpdateWithLifecycleStatus`), `reconcile.go:239-243` (intent comment), `reconcile_test.go:TestReconcileCloseSkipsClosureGate`.

**Evidence-Close** (`CloseWithEvidence` or `Store.Close`):
- **When used**: When a bead is closed after execution, review, or manual administration. This is the primary close path for work that ran and completed.
- **Execution evidence required**: The bead must carry one of:
  - A `closing_commit_sha` (exact SHA of the commit that closed the work)
  - A `session_id` (agent session that ran the work)
  - An execute-bead success event in the events history
  - AND a terminal verdict event (review APPROVE with non-empty rationale, explicit review-skipped marker, or manual-close marker)
- **ClosureGate enforcement**: `CloseWithEvidence` enforces `ClosureGate` (per `ddx-e30e60a9`) to reject closes without sufficient evidence, preventing silent false-closures. `Store.Close` bypasses the gate by design as a manual-administration escape hatch.
- **Code locations**: `store.go:1568-1610` (`CloseWithEvidence`), `store.go:1428-1560` (`Store.Close`), `store.go:1502-1558` (`ClosureGate` definition and documentation).

Both paths append an event (`lifecycle_reconciled` for reconcile-close, implicit in the session evidence for evidence-close) and trigger event externalization to sidecars when needed.

### 3.1.1 Lifecycle Reliability Invariants

- Raw `no_changes` is attempt evidence, not a durable bead queue state. The drain loop MUST translate it into one of the rows below before mutating the bead.
- `work-retry-after` may be set only when retrying the same bead after time passes could plausibly succeed without human/spec/dependency changes.
  - **MUST NOT set retry-after** (cause is not time-resolvable by waiting): `push_failed`, `declined_needs_decomposition`, `review_spec_gap`, `review_missing_acceptance`, `review_too_large` at decomposition depth cap.
  - **MAY set retry-after** (recheckable by waiting): `push_conflict` (15 min — remote advanced), `land_conflict` (15 min), transient quota/transport (15 min). `no_viable_provider` MUST NOT set per-bead retry-after when alternate routing paths exist or when the worker can transition to `paused-infra`; a per-bead cooldown is only appropriate when no alternate route exists AND the condition is purely time-dependent with no other worker that could claim the bead.
  - Cooldown lifetime SHOULD match the recheckable window; 15 min is the ceiling for every current recheckable outcome. A 24 h cooldown is never appropriate.
- Continuous forward progress is the default. Before a bead can move to
  `status=proposed`, the drain loop MUST exhaust every applicable automatic
  path in priority order: same-result review retry, same-bead implementation
  retry, reframe, decomposition, sibling/replacement split, or already-satisfied
  verification. `status=proposed` is only valid when those paths failed, would
  be lossy, or require a product/spec choice an agent cannot infer.
- `status=blocked` is reserved for hard external recheckable blockers. Too-large
  work, decomposition depth overflow, exhausted implementation retries,
  exhausted reviewer retries, and no-changes uncertainty MUST NOT map to
  `blocked` unless the evidence names an external blocker whose later resolution
  can be rechecked mechanically.
- Decomposition depth overflow is not, by itself, an operator-required
  condition. At child-depth cap, the decomposer MUST try a sibling or replacement
  split under the nearest safe parent/root. Only a failed or lossy
  sibling/replacement split may map to `status=proposed`.
- Every bead excluded from ordinary `ddx work` execution MUST have an explainable durable reason using existing mechanisms: dependency edge, `proposed` status, external `blocked` status, `execution-eligible=false`, `superseded-by`, epic/parent queue mode, or an active retry cooldown. Labels, events, and `extra` fields may explain that reason but do not control lifecycle.
- `extra` is not a general rule namespace. Only fields explicitly specified by
  TD-027 or this TD may affect queue eligibility, retry/recovery thresholds, or
  worker selection. Optional telemetry in `extra` may improve auditability, but
  missing optional telemetry MUST NOT by itself block, propose, cooldown, or
  skip otherwise executable work.
- `closed` means implementation, verification, and the default adversarial pre-close review gate have all passed, or the work was verified as already satisfied. Review failure never reopens a closed bead; it prevents close.
- Automatic implementation retry is bounded and applies only to classifications where the implementer had valid task context and further automated work can plausibly resolve the finding. Spec gaps, missing acceptance criteria, decomposition overflow, and exhausted reviewer failures require operator resolution only after the automatic reframe/decompose/replacement sequence cannot produce executable work.
- Latest terminal events and close evidence beat stale `work-*` `extra` metadata. Reconciliation may clear stale management fields, but MUST preserve append-only events and evidence.
- Closure paths are covered in detail in §3.1: reconcile-close (automatic, dependency-driven) vs evidence-close (manual or execution-driven). See §3.1 for the safety and evidence requirements of each path.

### 3.2 Outcome → Label / Event / Extra Mapping

| Outcome or lifecycle action | Status transition | Label changes | Events appended | Extra updates |
|---|---|---|---|---|
| `review_pass` after merged candidate | `in_progress → closed` | remove `claimed`, add `last-merged-rev:<sha>` (optional) | `review-pass` + `closed-merged` | `extra.last-run`, `extra.last-review`, `extra.closing_commit_sha` updated; clear stale `work-*` |
| `already_satisfied` | `in_progress → closed` | (none) | `closed-already-satisfied` | `extra.last-run`; clear stale `work-*` |
| `review_fixable_gap` with retry budget remaining | no durable change if continuing; otherwise `in_progress → open` | no lifecycle label required | `review-fixable-gap` + retry decision | `extra.last-review` carries findings ref |
| `review_fixable_gap` with retry budget exhausted | `in_progress → open` to enter auto-recovery; `in_progress → proposed` only after `auto_recovery_failed` | optional explanatory review label only | `review-fixable-gap` + ladder-exhausted/recovery decision | `extra.last-review` carries findings ref and exhausted budget; may increment `extra.consecutive_ladder_exhaustions` |
| `review_spec_gap` / `review_missing_acceptance` | `in_progress → open` when a safe reframe can make the bead executable; `in_progress → proposed` only when the missing spec/AC requires operator judgment | add `triage:spec-gap` or `triage:missing-acceptance` | `review-block` + optional `reframe-applied` | `extra.last-review` carries findings ref; `extra.last-recovery` records any safe rewrite attempt |
| `review_too_large` | `in_progress → open` with child dep edges, or sibling/replacement dep edges at child-depth cap; `in_progress → proposed` only if decomposition is lossy or no executable split can be generated | add `decomposed` when children/replacements exist | `review-too-large` + optionally `triage-decomposed` / `triage-overflow` | `extra.children` or `extra.superseded-by` + AC mapping when decomposed |
| `review_error` below retry cap | no durable change; retry reviewer for same `result_rev` | (none) | `review-error` | `extra.last-review-error` carries class, slot, attempt count |
| `review_error` exhausted | `in_progress → open` unless the error proves operator action is required; `in_progress → proposed` only after the automatic review/recovery path is exhausted | optional explanatory review label only | `review-manual-required` only for operator-required classes; otherwise `review-error` + recovery decision | `extra.last-review-error` carries class, slot, attempt count |
| `execution_failed` | `in_progress → open` | (none) | `unclaimed` + structured failure event | `extra.last-run` |
| verified no_changes already satisfied | `in_progress → closed` | remove no_changes triage labels | `no_changes_verified` + `closed-already-satisfied` | `extra.last-run`; clear `work-*` |
| unverified no_changes | `in_progress → open` | add `triage:no-changes-unverified` | `no_changes_unverified` | record verification command/result; do not set retry cooldown by default |
| unjustified no_changes | `in_progress → open` | add `triage:no-changes-unjustified` | `no_changes_unjustified` | record rationale absence/detail |
| legacy no_changes investigation (work too large) | `in_progress → open` with child dep edges, or sibling/replacement dep edges at child-depth cap; `in_progress → proposed` only if decomposition is lossy or no executable split can be generated | add `decomposed` when children/replacements exist | legacy `no_changes_needs_investigation` + `triage-decomposed`/`triage-overflow` | `extra.last-rationale`, `extra.children` or `extra.superseded-by`, AC mapping |
| legacy no_changes investigation (non-decomposition reason) | `in_progress → open` for retriable, verifiable, or recoverable uncertainty; `in_progress → proposed` only when evidence proves operator judgment is required | optional explanatory triage labels only | legacy `no_changes_needs_investigation` + recovery decision | `extra.last-rationale`; no retry cooldown |
| parent/epic/decomposed container | `in_progress → open` with dep edges to children, or `in_progress → open` with `execution-eligible=false` | add `decomposed` when children exist | `no_changes_decomposed` or `triage-decomposed` | `extra.children` lists child IDs + AC mapping, or `extra.execution-eligible=false` |
| external blocker | `in_progress → blocked` (hard) or `in_progress → open` (soft) | add `blocked-on-upstream:<id>` as explanatory label when useful | `no_changes_blocked` | `extra.last-rationale` names the external blocker |
| superseded work | no terminal success; leave open if visible history needed | (none) | structured superseded event if appended | `extra.superseded-by` names the replacement |
| transient infra/quota/transport | `in_progress → open` | (none) | `no_changes_recoverable`, `drain-paused-quota`, `rate-limit-retry`, or structured transport event | may set `work-retry-after` for retryable time-based condition only |
| `push_failed` (branch protection, auth-token, pre-push test, executor race) | `in_progress → open` | optional explanatory label (e.g. `blocked-on-branch-protection`) | `push-failed` with stderr detail and base-rev | `extra.last-run` only; **DO NOT** set `work-retry-after` |
| `push_conflict` (remote advanced; FF rejected) | `in_progress → open` | (none) | `push-conflict` with current remote head | may set `work-retry-after` at 15 min |
| stale no_changes tracker metadata | no status change unless latest terminal evidence closes the bead | remove stale no_changes triage labels only when contradicted by terminal evidence | preserve historical events; append reconciliation event if performed | clear stale `work-*` when latest terminal event proves them obsolete |
| dependency-satisfied reconcile close (`ddx bead reconcile`, `CloseSatisfied=true`) | `open → closed` via `UpdateWithLifecycleStatus`, `ManualClose=true`; `ClosureGate` **not** invoked | remove stale `work-*` labels; add `reconciled-nochanges-state` label | `lifecycle_reconciled` | `extra.work-*` cleared per `ReconcilePlan.ClearFields`; `externalizeEvents` called after close |
| `reframe_applied` — reframer rewrote description/AC; bead re-enters execution-ready lane | `in_progress → open` | add `reframed` label | `reframe-applied` with `from_rev`, `to_rev`, `reframer_cost` | `extra.consecutive_ladder_exhaustions` reset to 0; `extra.last-recovery` updated |
| `decompose_applied` — decomposer filed 2-5 executable child, sibling, or replacement beads; oversized bead remains open but not directly executable | `in_progress → open` with child dep edges, sibling/replacement dep edges, or supersession metadata; oversized bead `execution-eligible=false` when it should not be claimed directly | add `decomposed` label | `decompose-applied` with generated bead IDs + `reframer_cost` | `extra.children` or `extra.superseded-by` + AC mapping; `extra.execution-eligible=false`; `extra.last-recovery` updated |
| `auto_recovery_failed` — reframe, child decomposition, and sibling/replacement decomposition failed, or a valid operator-authored `recovery:manual` label set | `in_progress → proposed` | add `triage:auto-recovery-failed` label | `auto-recovery-failed` with `reframe_attempt_cost` + `decompose_attempt_cost` | `extra.last-recovery` records costs and failure reasons; **DO NOT** set `work-retry-after` |
| `per_bead_budget_exhausted` — cumulative cost exceeded `escalation.per_bead_budget_usd` | `in_progress → open` (re-claimable — budget exhaustion is recheckable) | (none) | `per-bead-budget-exhausted` with `total_cost` | `extra.last-run` only; **DO NOT** set `work-retry-after` — requires operator action or config change |

Legacy/backcompat `needs_human` and `triage:needs-investigation` labels are not lifecycle controls. New routing uses `status=proposed` for operator decisions; those labels may remain only as migration metadata until cleanup removes them.

### 3.3 Decomposition-First Transition Sequence

When any pre-claim readiness result, post-attempt triage result, review result,
or no-changes rationale classifies the bead as too large, needing breakdown, or
structurally non-executable, DDx applies this sequence before any
operator-attention transition:

1. **Safe in-place reframe**: if the bead can be made executable by preserving
   commitments while tightening description or acceptance criteria, apply
   `reframe_applied`; transition `in_progress → open` or `open → open`; reset
   `extra.consecutive_ladder_exhaustions`.
2. **Child decomposition**: if the bead can be split under itself, create 2-5
   executable child beads, map every parent AC to child ACs or explicit
   `non_scope`, add dependency edges, set the parent
   `extra.execution-eligible=false`, add `decomposed`, append
   `decompose-applied`, and leave the parent `status=open`.
3. **Sibling/replacement decomposition**: if child depth is exhausted but the
   work is still decomposable, create executable sibling or replacement beads
   under the nearest safe parent/root, record `extra.superseded-by` on the
   oversized bead when a replacement owns the remaining work, add dependency
   edges so the queue advances through the replacement work, and leave the
   oversized bead `status=open` but not execution-eligible.
4. **Final escape**: move to `status=proposed` only when reframe,
   child decomposition, and sibling/replacement decomposition all fail, would
   drop explicit scope, or require operator judgment. The event body MUST record
   which automatic actions were attempted and why each could not safely move
   work forward.

This sequence is synchronous from the state-machine perspective: there is no
durable "needs decomposition" status. If implementation needs to release a claim
between steps, it releases to `status=open` with evidence naming the pending
automatic recovery action, not to `blocked`, cooldown, or `proposed`.

## 4. Worker-State Enumeration

Worker state is the in-process state of the drain loop. It is **distinct from bead state** and is not persisted in the bead store. It exists only for the worker's lifetime.

| Worker state | Meaning | Entry | Exit |
|---|---|---|---|
| `idle` | Worker is up but not actively claiming. | Worker startup; transient between attempts. | Operator or scheduler moves to `draining`. |
| `draining` | Worker is actively claiming and executing beads. | Default operating state. | Quota or rate-limit pause; explicit stop. |
| `paused-quota` | Worker observed quota exhaustion on its harness. | `drain-paused-quota` event. | `drain-resumed-quota` after backoff or operator resume. |
| `paused-rate-limit` | Worker is sleeping out a rate-limit retry window inside an attempt. | Rate-limit response from harness. | Wait window elapses; worker resumes the same attempt. |
| `paused-infra` | Worker observed an infrastructure-class failure (`no_viable_provider`, all beads in infra-fault cooldowns). Beads are left immediately reclaimable — no per-bead `work-retry-after` is written. Worker sleeps `PausedInfraInterval` (2 min) then re-evaluates the full queue. | `loop.paused-infra` event with `resume_at` timestamp. See reliability-principles.md P6. | `resume_at` elapses or `WakeCh` fires; worker returns to `draining`. |
| `exiting` | Worker is shutting down cleanly; will not claim more beads. | Operator stop or terminal failure. | Process exit. |

Worker state is observable via the worker's stdout/log; it is not stored on any bead. Hygiene beads affecting worker state (QuotaPauseContract, RateLimitRetryContract) MUST NOT add new fields to the bead schema to represent these transient states.

## 5. Auto-Recovery Role Catalogue

Two agent roles support the cross-cycle recovery path (per ADR-024 P4; SD-025 Layer 3.5). These roles are dispatched by the drain proxy, not by an operator or agent tool directly.

| Role | `MinPower` floor | Claim held by | Output contract | Dispatch condition |
|---|---|---|---|---|
| `reframer` | Strong-power per ADR-024 P3 — `MinPower` set above the current cycle's implementer actual power | Drain proxy (not the reframer agent); reframer runs read-only against the bead record, then returns structured edits | Structured edits to bead description and/or acceptance criteria. Edits must preserve all explicit commitments (AC, non-scope, named files/tests, deps, governing artifact refs). A no-op result (no change) is valid and triggers the decomposer path. | Counter/event-derived ladder exhaustion, or an explicit structural classification that can be safely reframed, and no valid operator-authored `recovery:manual` label |
| `decomposer` | Strong-power per ADR-024 P3 — same floor as reframer | Drain proxy; decomposer runs read-only against the bead record, then returns a list of child, sibling, or replacement bead specs | List of 2-5 executable bead specs for `Backend.Create`. Each spec must include title, description, numbered AC, labels (inheriting parent's labels and `spec-id`), and the required parent/dep or supersession edge. A child-depth cap switches the requested output to sibling/replacement specs under the nearest safe parent/root; a no-op result triggers `auto_recovery_failed`. | Reframe attempt returned no change, reframer invocation failed, or a too-large/depth-cap classification requires structural split |

Both roles are dispatched with the same `role`, `bead_id`, `attempt_id`, `session_id`, and `review_group_id` correlation fields used by the reviewer role (ADR-024 Default Adversarial Pre-Close Review Gate). Operator-supplied passthrough constraints (`--harness`, `--provider`, `--model`) are forwarded unchanged. If those constraints prevent satisfying the strong-power `MinPower` floor, the dispatch returns `readiness_error` and the outcome maps to `auto_recovery_failed`.

## 6. `consecutive_ladder_exhaustions` Policy

The field `extra.consecutive_ladder_exhaustions` is a compact coordination counter maintained by the drain loop on each bead record. The field's data definition lives in TD-027 §11 (Bead Data Model); this section specifies the operational policy for incrementing, resetting, and threshold-triggering. It is not the only source of truth for recovery eligibility: DDx may derive equivalent facts from append-only attempt/review events, and missing, stale, or malformed counter data must fail open to ordinary execution or event-derived recovery rather than blocking, proposing, or cooling down the bead.

- **Incremented** at the end of each drain cycle in which the bead's within-cycle escalation ladder was fully exhausted (all power levels tried, none produced a close or forward progress).
- **Reset to 0** when the bead is successfully closed, when a reframe or decompose pass fires (the bead's prompt has changed; start fresh), or when an operator explicitly clears the counter via `ddx bead update`.
- **Threshold**: when `consecutive_ladder_exhaustions >= 2` (default; configurable in `ddxroot.Path()/config.yaml` as `escalation.auto_recovery_threshold`), the drain loop triggers the auto-recovery sequence described in ADR-024 Escalation Sequencing and SD-025 Layer 3.5.
- **Direct structural trigger**: explicit too-large, needs-decomposition, child-depth-cap, or no-changes-decompose classifications may enter §3.3 immediately without waiting for the counter threshold. The counter prevents infinite same-bead retry loops; it must not delay obvious decomposition work.
- The auto-recovery decision itself fires the `reframe_applied` or `decompose_applied` outcome, which changes status per §3.

## 7. Per-Hygiene-Bead Contracts

Each hygiene bead's AC must cite the relevant subsection. Anything not in the contract here is out of scope for that bead.

### 7.1 NoChangesContract (ddx-b24e9630)

NoChangesContract outcomes use the canonical mapping in §3. This subsection binds the hygiene bead to that mapping; it does not define a second disposition table.

**Claim behavior**: the claim is released for every no_changes action that leaves the bead `open`. It is not released separately when the same mutation moves the bead directly to terminal `closed` or hard `blocked`.

**Loop interaction**: the try package parses and verifies no_changes rationale, then returns the lifecycle action. The drain loop applies §3 exactly and appends one of the no_changes event kinds from TD-027 §13; it does not invent new event kinds or use cooldown as a generic parking lot.

### 7.2 Bead Readiness Assessment And Triage Contract (ddx-3c154349)

Bead readiness assessment is the canonical pre-claim decision for actionability and scope. It owns the readiness queue decision and runs before a worker owns the bead, so most readiness actions start from `open`. Lint/rubric scoring is the diagnostic pass inside readiness; post-attempt triage is a separate after-evidence queue action. The implementation entrypoint may still be named `MODE: intake` for compatibility, but that is legacy wording only; the product concept is bead readiness assessment.

**Status transitions used**:

- `in_progress → open` when releasing stale claims.
- `open → open` when readiness applies a validated replacement rewrite or metadata-only safe improvement before implementation. The bead remains execution-ready unless a later readiness decision parks it.
- `open → blocked` when triage decides a bead has an external recheckable blocker.
- `open → open` when readiness decomposes a parent and adds dependency edges to children; the parent is dependency-waiting but remains `open`.
- `open → proposed` only when readiness finds ambiguity or decomposition loss
  that cannot be safely rewritten, decomposed into children, or decomposed into
  sibling/replacement work. Decomposition depth overflow alone stays on the
  automatic sibling/replacement split path and does not authorize operator
  attention.

The `proposed → open` transition, recorded by `triaged`, is the operator-acceptance signal for readiness idempotency. After that durable override, readiness may re-evaluate the bead, but it must not re-park the same bead for the same rule or finding unless prompt-relevant fields changed or the operator explicitly requests re-triage.

For successful replacement rewrites, the bead body may be materially shorter or longer than the original when prompt fitness requires it. Preservation is proven by the `triage-rewritten` / `intake-rewritten` evidence record and durable anchors, not by keeping old text inside the prompt body. For rejected rewrites, readiness enters the §3.3 decomposition-first sequence when structural follow-up work can preserve explicit scope; it moves the bead to `status=proposed` only when safe rewrite/decomposition/replacement cannot resolve the ambiguity without operator judgment.

**Labels added**: `triage`, `blocked-on-upstream:<id>`, `decomposed`, `triage:spec-gap`, `triage:missing-acceptance` (as applicable). Legacy/backcompat labels (`needs_human`, `needs-human-decomposition`) may be read during migration but must not be added as lifecycle controls.

**Labels removed**: `triage` after a triaged bead is reclaimed cleanly (the reverse mutation of `triage` is "next successful claim removes the label").

**Events fired**: `auto-triage`, `triage-ambiguous`, `triage-decomposed`, `triage-overflow`, `unclaimed`, optionally `blocked`.

**Claim behavior**: the auto-triage sweep is the only path that releases a claim it does not own. It MUST log the prior `assignee` and `claimed-at` in the appended event body.

**Loop interaction**: readiness and triage run out-of-band of execute-bead. They MUST NOT race with an active drain attempt holding the store lock — they acquire the same store lock and releases-then-reclaims is not their job.

### 7.3 QuotaPauseContract (ddx-aede917d)

**Status transitions used**: none. Quota is a worker-state concern, not a bead-state concern.

**Labels**: none added on the affected bead. The current attempt is treated as `execution_failed` (claim released, status returns to `open`).

**Events fired**: `drain-paused-quota` is appended to a worker-scoped event log (drain-process record), not to the bead's event stream. An `unclaimed` event is appended to the bead.

**Claim behavior**: the claim on the in-flight bead is released cleanly so another worker (or a later resumed worker) can pick it up.

**Loop interaction**: the worker transitions to worker-state `paused-quota` (§4). It stops claiming new beads until the configured backoff elapses or an explicit resume signal arrives.

### 7.4 RateLimitRetryContract (ddx-c6e3db02)

**Status transitions used**: none. Rate-limit retry is internal to a single attempt; the bead remains `in_progress` throughout.

**Labels**: none.

**Events fired**: `rate-limit-retry` per retry, on the bead's event stream. Each event carries the retry count and the wait duration in the body.

**Claim behavior**: claim is held continuously across retries.

**Loop interaction**: rate-limit handling is bounded by the retry budget; on budget exhaustion the outcome becomes `execution_failed` and the standard mapping in §3 applies. The worker briefly enters `paused-rate-limit` for the wait window, then returns to `draining`.

### 7.5 LockContentionContract (ddx-da11a34a)

**Status transitions used**: none. Lock contention is a main-git/tracker coordination concern, not a bead-state concern.

**Labels**: none.

**Events fired**: `lock-contention` (append-only, on the affected bead, only after the contention has been resolved and the mutation succeeded). Pure read-side contention does not emit an event.

**Claim behavior**: unaffected. The claim is acquired or not; partial states are not persisted.

**Loop interaction**: the worker retries the locked operation with exponential backoff up to a bounded budget. On budget exhaustion the calling outcome maps to `execution_failed` (§3). The worker does not enter a dedicated worker-state for lock contention; it remains `draining` and treats the failure as ordinary.

**Filesystem-shape contract**: per `ddx-d30bc1a0`, the main-git/tracker lock path is `ddxroot.Path()/.git-tracker.lock`, where `ddxroot.Path()` is the resolved per-project DDx root. It is a process-shared lock **directory**, not a regular lockfile. Lock acquisition MUST classify the existing path immediately after `mkdir` reports that it already exists and MUST NOT sleep/back off until the path is confirmed to be a real lock directory.

- Missing after race: retry acquisition immediately.
- Directory: apply the existing PID/`acquired_at` stale-lock policy. A directory owned by a live process remains ordinary lock contention.
- Stale regular file: if and only if `lstat` reports an exact regular file and its mtime is older than the stale-lock threshold, remove it with single-path removal and retry acquisition immediately.
- Fresh regular file: fail fast with a malformed-lock diagnostic. Do not wait for the lock-contention retry budget and do not report `owner pid: unknown`.
- Symlink, socket, device, or other special file: fail fast with a malformed-lock diagnostic and do not remove it.

Malformed lock paths are operator/remediation diagnostics, not lock contention. They do not emit `lock-contention`, do not introduce a new status or label, and do not change claim semantics.

## 8. Future-Change Process

> Any bead that introduces or changes a label, an event kind, an outcome → state mapping, claim handling, or worker-state semantics MUST cite the relevant TD section that authorizes the change. If no section authorizes it, the TD is amended in the same PR (or a parent bead) before the dependent work lands.

In practice:

- A new event kind: amend TD-027 §13 in the same PR (kind vocabulary lives in TD-027 because consumers depend on the list).
- A new outcome → state mapping: amend this TD §3.
- A new label: amend TD-027 §4.
- A new worker state: amend this TD §4.
- A new auto-recovery role: amend this TD §5.
- A new persisted status: do not start the work; file an ADR-004 amendment first (per TD-027's critical constraint).

The CI guard described in TD-027 §22 also applies here: changes to `bead-record.schema.json` or to the persisted-status enum must touch ADR-004 + TD-027 in the same commit.

## 9. Relationship to TD-027

TD-027 owns the bead substrate (storage system + lifecycle + data model). This TD owns the operational contract by which the drain loop / executor uses that substrate. The clean conceptual split:

| Concern | Doc |
|---|---|
| Status enum (the six values) | TD-027 §1 |
| Transition matrix | TD-027 §2 |
| Category taxonomy (status / label / event / etc.) | TD-027 §3 |
| Naming-role decisions (what's a label vs. a status) | TD-027 §4 |
| Storage interface (Backend + sub-interfaces) | TD-027 §6 |
| Operation pattern | TD-027 §7 |
| Bead data model (fields, invariants, wire format) | TD-027 §11 |
| Claim semantics (acquire/release, worker shutdown) | TD-027 §12 |
| Event vocabulary (the controlled list) | TD-027 §13 |
| Status enum (the six values) | **TD-031 §2** |
| Outcome → state/event/label mapping | **TD-031 §3** |
| Worker-state enumeration | **TD-031 §4** |
| Auto-recovery role dispatch | **TD-031 §5** |
| `consecutive_ladder_exhaustions` policy | **TD-031 §6** |
| Hygiene-bead operational contracts | **TD-031 §7** |
| Collection registry, archival, attachments | TD-027 §15–§17 |
| Module boundary (internal/) | TD-027 §21 |

If you change something in TD-031 that depends on a contract in TD-027 (e.g. adding an outcome that fires a new event kind), update both docs in the same PR.

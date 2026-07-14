---
ddx:
  id: IP-2026-07-13-phase1-lower-altitude
  type: implementation-plan
  status: reviewed
  depends_on:
    - IP-2026-07-13-phase0-stop-self-harm
---
# Phase 1 Plan: Lower the Altitude

**Date:** 2026-07-13
**Status:** Revised r4 — execution-readiness and Fizeau runtime-boundary repairs applied 2026-07-13
**Source:** AR-2026-07-13-vision-vs-reality.md §7 Phase 1 and §7 "The architectural decision"
**Mode:** Mixed — spec/contract tasks are operator-driven; mechanical extractions and test-backed code tasks may be drained once Phase 0's exit criteria hold.

## Goal

Move DDx from reconstructing Fizeau runtime state by text-scraping to consuming
the public Fizeau immediate-error/final-event contract, plus richer typed fields
only after their release is pinned. Fizeau is the full agent runtime and
harness-of-harnesses for Claude Code, Codex, Gemini, and other providers; DDx
is the work tracker and git-aware work-execution orchestrator. DDx keeps bead
state, worktree lifecycle, verification, landing, and durable evidence while
Fizeau owns provider selection, session execution, in-session retries, any
future contract-defined continuation, and provider process trees. Measured
target: `ddx try` is non-inferior to a bare Fizeau `Execute` baseline (MET-003)
on landed-rate within the predeclared 10-percentage-point margin, with paired
landed-bead wall-clock within 1.25× of arm B. The paired decision rules and
sample floors are exit criterion 1; weekly point estimates alone cannot close
the phase.

## Scope

In scope: the Fizeau/DDx runtime contract, typed attempt policy, deletion of DDx
route preflight, cooldown posture, the close gate, worktree lifecycle ownership
(spec + implementation), claim liveness, lock-stack consolidation, the loop-file
decomposition, and the closed-means-class-dead process rule.

Non-goals: server/worker coordination (Phase 3 — including `TestIntegration_ManualWorkerContinuesOfflineAndReconciles`, which review moved out of this phase: it needs coordination machinery only Phase 3 builds); doc-truth work beyond the two spec artifacts this phase forces (ADR-024 amendment, new worktree spec); any new feature surface; any DDx integration with, invocation of, continuation of, or process supervision for an underlying provider CLI outside Fizeau.

### Entry criteria

Phase 0 exit note appended. Exception (pre-authorized there): WB-2a may start during Phase 0 if routing refusals dominate the loss taxonomy.

### Exit criteria (revised)

1. **Paired MET-003 non-inferiority, with predeclared error control:** each pair is the same bead run from the same pinned base revision with the same task facts, initial abstract `MinPower` floor, one immutable comparison-wide operator-passthrough envelope identical across arms, no DDx-originated route pins, and an equalized landed predicate. Fizeau routes each arm independently; actual harness/provider/model facts are confined to linked per-invocation audit detail and never used in comparison display, grouping, filtering, grading, admission, exclusion, or steering. For landed-rate, encode each pair's arm-A-minus-arm-B result as `-1`, `0`, or `1`; after at least 100 complete pairs, the lower bound of an anytime-valid 95% confidence sequence for the mean paired difference must be greater than `-0.10` (the 10-point non-inferiority margin, consistent with Phase 3). MET-003 must name the confidence-sequence method and alpha before the first outcome is examined; weekly evidence may accumulate into that sequence, but the old two-consecutive-20-bead rule and unadjusted repeated CIs are not gates. Wall-clock is measured from dispatch start through the equalized landed-predicate verdict, including DDx close-gate and repair overhead. On at least 40 pairs where both arms land, the predeclared one-sided anytime-valid 95% paired confidence sequence for the median per-bead elapsed-time ratio `arm A / arm B` must have an upper bound less than `1.25`; timeouts use MET-003's predeclared cap rather than being dropped. If either sequence is inconclusive or its sample floor is unmet, the phase stays open and collection continues under the predeclared sequential rule.
2. DDx-owned stages account for < 10% of attempt losses — measurable because WB-1 records Fizeau's typed runtime cause/stage separately from the DDx attempt stage. Fizeau-owned stages include admission/routing/provider/transport/runtime; DDx-owned stages include claim/workspace/verify/review-policy/land/tracker. MET-003 gains a loss-by-owner-and-stage table rather than attributing Fizeau routing or provider failures to DDx (review: this attribution did not previously exist anywhere).
3. Zero recurrence of any `niflheim-*` defect class for 3 consecutive weeks (per-class tripwires from the Phase 0 class map green; no new incident labels).
4. **Pattern-table lint green** (replaces the original grep criterion, which review showed both contradicted WB-3 and missed `cli/cmd`): a checked-in lint (evidencelint pattern) forbids substring/regex failure-classification tables outside the single `failclass` package, covering `cli/internal/...` **and** `cli/cmd/...`, with an explicit annotated whitelist for non-classification uses of substring matching.
5. Median DDx-arm wall-clock < 15 minutes over the same MET-003 cohort after at
   least 100 complete pairs. Elapsed time runs from dispatch through the
   equalized repository-evidence verdict; timeouts are retained at MET-003's
   predeclared cap rather than dropped.

## Assumptions

- Fizeau (`github.com/easel/fizeau` v0.14.50) is the current full runtime/harness-of-harnesses and is operator-controlled. The 29 non-test DDx files importing Fizeau are the integration surface to consolidate, not a provider-specific fallback surface. Claude Code, Codex, Gemini, and future providers remain Fizeau implementation details.
- Missing typed cause/stage, harness-neutral continuation, session-targeted
  lifecycle operations, or explicit provider-process-tree disposition are
  upstream Fizeau contract gaps. Current context cancellation remains the only
  DDx cancellation mechanism. WB-1 enhances that contract, pins the first
  compatible Fizeau release, and blocks dependent DDx work until its
  conformance suite passes. DDx never bypasses Fizeau to meet a schedule.
- Current in-band repair starts a fresh Fizeau `Execute` seeded with the task,
  current diff, and gate failure at the attempt's unchanged `MinPower`. A
  harness-neutral continuation path may replace that only after DDx pins a
  compatible public Fizeau API. If capability-sensitive evidence warrants
  higher power, DDx ends the current attempt and allocates a distinct attempt
  ID/workspace; it may never invoke an underlying harness directly.
- Phase 0 tripwires stay green throughout; any regression pauses drained execution of this phase.
- "DDx owns execution; workflows own ordering" remains the layer boundary.

### Runtime boundary and completion/retry semantics

- **Fizeau owns:** provider discovery and selection, provider-specific prompts/protocols, the public `Execute` stream, streaming, usage/cost capture, in-session and provider retry, and the lifetime and termination of every underlying provider process tree. Current DDx cancellation is only cancellation of the context supplied to `Execute`; any future continuation or session-targeted lifecycle operation must come from a pinned public Fizeau contract.
- **DDx owns:** bead claim and state, attempt identity, worktree and candidate lifecycle, task/context passed to Fizeau, deterministic verification and review policy, landing, tracker closure, and the policy that decides whether a completed Fizeau operation plus DDx evidence warrants a new DDx attempt.
- **Forbidden boundary crossings:** DDx does not invoke Claude Code, Codex, Gemini, or any other provider CLI; parse provider stdout/stderr for control flow; manufacture provider-specific continuation commands; or discover/kill provider process trees. It calls only the pinned Fizeau API and consumes its typed contract.

| Lifecycle term | Contract |
|---|---|
| Fizeau operation completion | Under current v0.14.50, one `Execute` ends either with an immediate public error or one final event carrying public `ServiceFinalData`; cancellation is requested through its context. CONTRACT-003 may later add machine-readable cause/stage, harness-neutral continuation, and provider-tree disposition, but DDx cannot consume them before pinning that release. Fizeau success means the runtime operation finished; it does not mean DDx verification passed, the attempt landed, or the bead closed. |
| DDx attempt completion | One claimed attempt reaches a DDx terminal state such as landed, failed, parked, or cancelled. An attempt may contain an initial Fizeau operation plus fresh-`Execute` repair operations at unchanged abstract power in the same candidate worktree; only a future pinned contract may substitute Fizeau-owned continuation. |
| Bead completion | DDx has verified and landed the candidate, satisfied the closure gate, and durably closed the tracker record. Neither a successful Fizeau session nor a successful DDx attempt is alone sufficient unless this state is reached. |
| Fizeau in-session/provider retry | Fizeau retries or changes provider execution before emitting the CONTRACT-003 terminal result. This remains inside one Fizeau operation and one DDx attempt; DDx neither classifies the provider text nor creates a new attempt. |
| Fizeau continuation/repair | Current repair uses a fresh Fizeau `Execute` with task + diff + failure evidence at unchanged power. After a compatible release is pinned, DDx may instead request continuation only through that opaque, harness-neutral public mechanism. This is never a direct provider invocation. |
| DDx new-attempt retry | Only after an attempt reaches a terminal DDx decision does DDx allocate a new attempt ID/workspace and call Fizeau `Execute` again, optionally with higher `MinPower` for capability-sensitive evidence. The current decision consumes typed immediate errors, public final fields, and DDx-owned evidence; future cause/stage may be used only after its release is pinned. It never consumes provider text or concrete route identity. |

## Work Breakdown

Ordering: WB-2a immediately (no dependencies, largest single failure class); WB-1 next (riskiest dependency, 2-week timebox); WB-4's spec and cleanup-responsibility inventory are written in parallel week 1; WB-3 and WB-2b after WB-1; WB-8 after WB-4's owner exists (locks move into it); WB-6 after WB-1/2 land; WB-5 and WB-7 independent. WB-4 may migrate responsibilities incrementally, but deletion of `ExecutionCleanupManager` waits for every replacement owner and dependency in the WB-4 contract.

### WB-2a: Delete DDx route preflight — no dependencies, start immediately

- Objective: delete DDx's route prediction and refusal layer: 7,823 "no viable
  routing candidate" refusals = 96% of all disruptions. Route viability is
  exclusively a Fizeau decision. (Review split this out of the old WB-2: it
  needs nothing typed, and it may be needed to pass Phase 0's own success gate.)
- Files or systems: **corrected anchors** (review: the original `internal/server/workers.go:803` contains no routing code): `cli/internal/agent/execute_bead_loop.go:7595,7826` (preflight refusal path), `cli/cmd/agent_execute_loop_escalation.go:374`, `cli/internal/agent/readiness_classification.go:257`; legacy profile input `cli/internal/config/types.go:501-507`, `config.go:135`, and `routing_ladder_test.go`.
- Steps: delete the DDx route-preflight call, catalog/viability query, prediction,
  annotation, and `no viable routing candidate` classifier. Remove the final
  `agent.routing.profile_priority` compatibility field/warning/tests rather than
  translating profile names into the current public Fizeau `Policy`; v0.14.50
  has no per-request `Profile`. Keep only
  request-shape validation for DDx-owned fields. Replace the profile selector
  with TD-037's direct DDx work-policy mapping
  (`easy -> MinPower 0`, `medium -> 7`, `hard -> 9`) on Fizeau's public
  abstract 1–10 scale; never query a catalog or route-viability surface. Then
  call Fizeau. Consume either
  the typed immediate error or public final event returned by the pinned Fizeau
  contract; do not reconstruct route viability. Before the validation drain,
  write a cohort manifest containing the 50 new attempt IDs plus UTC start/end
  bounds; a checked-in audit query accepts that manifest and ignores events
  outside both the ID set and time window.
- Acceptance: the DDx execution path has no route-preflight, catalog, or
  profile-to-policy translation dependency; initial difficulty mapping changes
  only numeric `MinPower` and has focused `easy`/`medium`/`hard` tests; the three legacy profile/model
  routing config keys are absent from accepted config; `disruption_detected` events with the historical DDx-generated
  cause == 0 for the newly generated 50-attempt cohort; deletion is not
  flag-gated. The audit-query regression fixture contains a historical matching
  event outside the cohort and proves it is excluded.
- Validation: run the cohort-scoped audit query over `.ddx/attachments/*/events.jsonl` with the recorded attempt manifest and UTC bounds, archive the manifest and zero-count result with the drain evidence, and report before/after counts for that cohort only. A repository-wide historical tally is diagnostic and cannot fail this acceptance criterion.
- Non-scope: cooldown changes (WB-2b); bead-readiness intake lint (ADR-023 WARN-ONLY stays).
- Dependencies: none.

### WB-1: Typed Fizeau session results; one DDx attempt-policy owner

- Objective: every DDx attempt decision consumes a typed immediate error or
  public final event from Fizeau CONTRACT-003 plus explicit DDx-owned stage
  evidence; provider-text scraping and provider-process supervision in DDx are
  deleted, not bypassed.
- Files or systems: upstream Fizeau CONTRACT-003 amendment: the versioned
  `Execute` event stream plus harness-neutral continuation and cancellation
  semantics, with immediate/final outcomes carrying outcome enum,
  machine-readable cause, Fizeau stage, optional queue-level `RetryAfter`,
  usage/cost, session reference, continuation capability, and
  provider-process-tree disposition. Fizeau must supervise the full provider
  process group/tree and make terminal/cancellation semantics deterministic —
  incident ddx-01b89378 left 28 underlying Codex processes alive because that
  runtime guarantee was absent. DDx deletion/rewire list (review-extended — the
  original list left six scraping sites alive): `cli/internal/agent/executor.go:101-140,275-295` (22 kill-regexes), `provider_failure.go:85-192`, `execute_bead_status.go:124-215` (+ the `containsAny` helper at `:255`), `cli/cmd/agent_execute_loop_escalation.go:44-59`, `cli/internal/escalation/infrastructure.go:22-78`, `readiness_classification.go:204-257`, `server_outage.go:155-163`, `execute_bead_validation.go:119,130`, `triage_dispatch.go:10`, `execute_bead_loop.go:7824`. `execute_bead_review_classification.go:42-48` (FEAT-022 review-error classes) is **kept but rewired** in WB-3 to consume Fizeau's typed cause. The single DDx owner is an attempt-policy adapter in `cli/internal/agent/failclass` (or folded into `internal/agent/executeloop/state_machine.go`); it does not reimplement Fizeau provider classification.
- Steps: (1) amend the upstream contract and add Fizeau conformance tests across its wrapped providers; (2) release Fizeau with typed results, continuation capability reporting, cancellation, and process-tree guarantees; (3) pin that compatible release in DDx and add boundary conformance fixtures; (4) translate immediate typed errors and the public final event once into DDx attempt policy, keeping Fizeau in-session/provider retry distinct from DDx new-attempt retry; (5) route DDx retry/escalate/park/cooldown decisions through that adapter; (6) delete/rewire the listed text sites; DDx cancels the context supplied to current `Execute`, and uses only whatever harness-neutral lifecycle operation a later pinned public contract actually defines — DDx never invents a `Continue`/`Cancel` method or signals an underlying provider tree itself; (7) delete the PATH shim only after the pinned Fizeau process-tree contract passes; (8) ship the exit-criterion-4 pattern-table lint in the same change so deleted patterns cannot regrow.
- Acceptance: pattern-table lint green over `cli/internal` + `cli/cmd`; `TestFizeauImmediateErrorAndFinalEventRoundTrip`; `TestFizeauContinuationCapabilityRoundTrip`; `TestDDXAttemptPolicyConsumesTypedFizeauResult` (one synthetic service outcome → exactly one of current-attempt repair/new-attempt retry/escalate/park); `TestFizeauExecuteContextCancellationTerminatesProviderTree` and kill-DDx-mid-flight integration prove no surviving provider tree; typed Fizeau cause/stage and DDx owner/stage remain separate in MET-003.
- Validation: upstream Fizeau contract suite green; `cd cli && go test ./internal/agent/... ./internal/escalation/... ./cmd/...`; supervised 20-attempt drain with a typed immediate/final outcome for every Fizeau operation and no provider-specific DDx subprocess.
- Non-scope: Fizeau's internal routing, provider selection, provider retry, and provider-specific protocols; review content; any direct provider integration in DDx.
- Dependencies: upstream Fizeau epic `fizeau-6f9ffa71`; contract bead `fizeau-8440f9ec`; typed terminal bead `fizeau-9d2c83e8`; provider-process lifetime bead `fizeau-c795e7d9`; and continuation bead `fizeau-022cbb9a`. DDx pins the first Fizeau release satisfying the applicable CONTRACT-003 children and the boundary conformance suite. WB-1-dependent work and Phase 1 exit remain blocked until it exists; independent WBs may continue. There is no timeboxed DDx bypass, direct-provider fallback, or Fizeau demotion/replacement path.

### WB-2b: Typed queue pause and reclaimability

- Objective: release the bead and pause queue claiming only when current public
  `*fizeau.NoViableProviderForNow` (or a future pinned public equivalent)
  supplies an explicit queue-level `RetryAfter`; never turn provider or route
  identity into a per-bead cooldown.
- Files or systems: cooldown derivation `cli/internal/bead/store.go:3263-3272`
  (`originMainHead` returns `""` on any error → origin-bound cooldowns silently
  never clear offline) and the worker's queue-pause state.
- Steps: remove cause-keyed provider/quota bead cooldowns. On the public
  `RetryAfter`, release the active bead claim, persist the pause deadline and
  evidence reference at worker/queue scope, and resume from wall clock without
  network or origin reachability. Without that typed hint, return the bead to
  the ready lane or surface operator action according to the available public outcome; DDx
  does not derive delay from provider text or route identity.
- Acceptance: `TestTypedRetryAfterReleasesClaimBeforeQueuePause`,
  `TestQueuePauseExpiresOffline`, and
  `TestProviderIdentityCannotCreatePerBeadCooldown`.
- Validation: `cd cli && go test -run 'RetryAfter|QueuePause|Cooldown' ./internal/bead/... ./internal/agent/...`.
- Dependencies: WB-1.

### WB-3: Risk-proportional close gate with in-band repair (ADR-024 amendment)

- Objective: default close path ≤ 1 LLM invocation of overhead; review failures repair in-band.
- Files or systems: ADR-024 amendment; `candidate_cycle.go` (737 lines), `execute_bead_review.go` (1,492 lines), `execute_bead_review_group.go`; `execute_bead_review_classification.go:42-48` rewired to typed causes (from WB-1).
- Steps: (1) amend ADR-024: default tier = deterministic checks + one reviewer requested at a stronger abstract `MinPower` than the implementer, with every implementer and reviewer session executed through Fizeau; elevated tier (two stronger slots and repair cycles) only for `kind:safety`, `area:storage`, `kind:migration`, or an explicit operator review-tier override — never because a route pin exists; (2) after Fizeau exhausts its in-session/provider transport policy and returns a public typed review-transport cause, DDx may retry one review operation at the identical abstract power and with immutable passthrough, then downgrade to deterministic-checks-only with an operator-visible event — transport failure never justifies higher power and never fails the implementation attempt; (3) under current v0.14.50, gate failure uses a fresh Fizeau `Execute` at unchanged abstract power, seeded with the original task, current diff, and failure output; (4) only after a compatible public continuation contract is pinned may that fresh operation be replaced with harness-neutral continuation. DDx never constructs or invokes a provider-specific resume/spawn command and never sets harness/provider/model/profile fields to obtain a stronger reviewer.
- Acceptance: `TestDefaultCloseGateSingleReviewer`,
  `TestReviewTransportRetryKeepsPowerAndPins`,
  `TestReviewTransportErrorDowngradesNotFails`, and
  `TestGateFailureUsesFreshFizeauExecuteAtSamePower`; after a compatible
  continuation release is pinned, additionally enable
  `TestGateFailureUsesContractDefinedFizeauContinuation`. No provider binary
  appears in DDx repair fixtures; elevated review tier fires only on labeled
  risk classes or explicit operator override (table test).
- Validation: current Fizeau consumer tests green; after pinning continuation,
  its upstream conformance test is also green; `cd cli && go test -run
  'CloseGate|ReviewTransport|GateFailure|Fizeau' ./internal/agent/...`;
  review-error ratio < 5% over a week; MET-003 wall-clock trend.
- Non-scope: review prompt content; removing the escalated tier. Note for Phase 2: FEAT-022/TD-033 text amendments tied to this ADR change are sequenced behind it (Phase 2 ledger rows 3/4 wait for this WB).
- Dependencies: WB-1.

### WB-4: Worktree lifecycle — write the missing spec, migrate every cleanup duty, then delete

- Objective: close the AR's largest spec hole and replace overlapping best-effort worktree reapers with one transactional owner, without accidentally deleting the unrelated retention, process-reaping, heartbeat, or Phase 0 audit duties currently bundled into `ExecutionCleanupManager`.
- Files or systems: new spec `docs/helix/01-frame/features/FEAT-030-worktree-lifecycle.md`. Implementation: **extend the existing `AttemptWorkspace` type (`cli/internal/agent/attempt_backend.go:62`) into the single worktree-lifecycle owner, or supersede it under a new name with a migration step — the choice is made in the spec, not by the implementing bead** (review: the original "new AttemptWorkspace type" collides with the existing exported type). Creation sites routed through it: attempt backends, `lifecycle_dispatch.go:81` (per-readiness-check worktree, teardown errors discarded at `:40-41`), epic decomposition. Deletions after the migration gate below: prune-retry string matching `attempt_backend.go:110-157`; only the superseded worktree/liveness heuristics in the 1,570-line `ExecutionCleanupManager`; liveness/max-age guessing in `cli/cmd/execute_loop_shared.go:782-855` (full path per review — this deletion spans the cmd/ layer). The manager type/file is deleted only after all non-worktree responsibilities have moved to the named replacements below. Also: `cli/internal/git/exec.go:35-66` gains `LC_ALL=C, LANG=C`; `execute_bead_land.go:387-526` checkout emulation (2026-07-06 clobber incident) replaced by landing onto a non-checked-out integration ref; blob forensics (`:1532-1541 → :584-609`) move outside the lock.
- Steps: (1) spec first (operator-reviewed): enumerated states (created → claimed → landed | preserved → reaped), journal record written before `git worktree add`, crash-recovery semantics including `locked` (unlock-if-owner-dead → prune), ownership metadata schema, invariant "every registration reachable from exactly one journal record", the AttemptWorkspace extend-vs-supersede decision, and the responsibility inventory below; (2) implement the journal-backed worktree owner; (3) extract or rewire every other cleanup responsibility to its named owner, retaining the current behavior until its replacement test passes; (4) run old and new owners in dry-run parity over a fixture corpus and a 50-attempt drain, treating any deletion-set disagreement as blocking; (5) switch each call site independently; (6) locale pinning; (7) integration-ref landing; (8) forensics out of the lock; (9) delete `ExecutionCleanupManager` and the other two superseded reapers only when the migration checklist has no unresolved row; `doctor --unjam` and `ddx cleanup` then compose the journal and the narrow replacement owners rather than one heuristic manager.

#### Mandatory cleanup migration and retention contract

| Current responsibility | Named replacement owner | Must exist before the old path is removed | Regression/parity proof |
|---|---|---|---|
| Worktree registration, stale registered/unregistered worktrees, scratch worktrees, worktree pruning, and stale run-state cleanup | Journal-backed `AttemptWorkspace` worktree-lifecycle owner from FEAT-030 | WB-4 spec and implementation | `TestChaos_SIGKILLDuringWorktreeAddReclaimsOnNextStart`, `TestUnlockIfOwnerDeadPrunesLockedInitializing`, and old/new dry-run deletion-set parity |
| Phase 0 WB-3 `cleanup --audit-unowned`, watched-root scratch auditing/reaping, and other non-worktree runtime scratch | Narrow `RuntimeResourceJanitor` extracted under `cli/internal/agent/runtimecleanup`; it consumes the same ownership metadata but does not infer worktree liveness | Phase 0 WB-3 and the extraction bead | `TestAttemptLeavesNoUnownedTempDirs`, a named `TestRuntimeResourceJanitorParity`, and a clean 50-attempt cohort audit |
| Published claim-heartbeat scope directories and abandoned heartbeat temp files | WB-5's synced claim-liveness store, including explicit expiry/reaping | WB-5 design note and implementation | `TestCrossMachineClaimNotStolenWhileFresh`, `TestClaimLivenessStoreReapsExpiredPublishedHeartbeats`, and `TestClaimLivenessStoreReapsAbandonedTempFiles` |
| Underlying provider process-group/tree lifetime, cancellation, and crash recovery | Fizeau runtime's public execution/process-tree contract; DDx retains only its correlation ID and opaque `SessionLogPath`, plus a typed disposition only after a compatible release exposes one | WB-1's pinned compatible Fizeau release | Existing `ExecutionCleanupManager_*Process*` scenarios move to upstream Fizeau conformance plus `TestFizeauCancelTerminatesProviderTree` and kill-DDx/no-surviving-provider-tree integration; DDx process scraping/reaping is then deleted |
| Complete execution evidence and `.ddx/runs` scanning/retention, preserving active or incomplete evidence and `executions.retain_days` | Extracted `ExecutionRetention` owner, evidence policy | Extraction bead; no Phase 3 run-substrate redesign required | Existing active/incomplete evidence and retain-days tests pass unchanged against `ExecutionRetention` |
| DDx attempt-log and opaque Fizeau artifact-reference retention | Extracted `ExecutionRetention` owner; Fizeau retains native logs and owns session lifecycle | Extraction bead | Retain-days tests cover DDx evidence and opaque `SessionLogPath` references without active-session discovery or native-log parsing |
| Worker-directory retention, including live-PID preservation | Extracted `ExecutionRetention` owner, worker-directory policy; Phase 3 may later rehome it without changing this gate | Extraction bead | Existing live-worker and retain-days worker-dir tests pass unchanged against `ExecutionRetention` |
| Landed candidate-ref pruning after durable result metadata exists | WB-6 land/report stage's `CandidateRefRetention` owner | WB-6 goldens and land/report extraction | Existing durable-result/candidate-ref tests plus a named no-prune-before-durable-result test |

The table is a deletion gate, not documentation after the fact: each row must name its landed replacement commit and test in the WB-4 exit evidence. A responsibility may remain temporarily in `ExecutionCleanupManager`, but no implementation bead may remove it, its config, its summary/event fields, or its CLI observability before that row's owner and proof are green. `ExecutionCleanupManager` itself is deleted only after every row has migrated and the composed `ddx cleanup` output retains equivalent audit and dry-run coverage.

- Acceptance (review-corrected): all migration-table rows name a landed replacement and passing regression test; `TestExecutionCleanupReplacementParity` proves the composed owners preserve the old manager's protected/deleted fixture sets before the manager is removed; `TestChaos_SIGKILLDuringWorktreeAddReclaimsOnNextStart`; `TestChaos_PreDispatchMutationWindowDoesNotHoldLockAcrossHarnessWait` (the TP-021 test that belongs to this layer — the offline-reconcile TP-021 test moved to Phase 3); `TestUnlockIfOwnerDeadPrunesLockedInitializing` (replaces the unusable `grep "worktree unlock"` criterion — git args are separate strings, the grep could only match comments); landing test asserts no write to a checked-out branch with a dirty operator tree.
- Validation: `cd cli && go test -race -run 'Chaos|Worktree|Land|Unlock' ./internal/agent/... ./internal/git/... ./cmd/...` (cmd/ added per review); 50-attempt drain leaves `git worktree list` clean.
- Non-scope: epic execution contract redesign (Phase 2 re-specs the container/auto-decompose divergence).
- Dependencies: spec review; WB-6 goldens for the land stage land first or together. Deleting `ExecutionCleanupManager` additionally depends on Phase 0 WB-3, WB-1's pinned Fizeau process-tree contract, WB-5's claim-liveness owner, WB-6's candidate-ref owner, every `ExecutionRetention` policy extraction, and green replacement-parity evidence for all rows above.

### WB-5: Claim liveness into synced state; queue hot-path cost

- Objective: kill the structural double-execution risk and the per-iteration latency tax.
- Files or systems (review-corrected description): the defect is that **lease facts live in an unsynced per-machine sidecar** (`cli/internal/bead/claim_liveness.go:51-59`), so cross-machine freshness is invisible — current staleness logic is already machine-aware (`:289-303`: PID check only when `rec.Machine` matches; cross-machine falls back to age) and `ClaimLivenessRoot` already honors the configured runtime root; do not restate the old root cause in beads. Hot path: `:202-212` (1 ms×3 sleeps on the normal unclaimed path); `store.go` `Status()` (4 full reparse-classify passes + git subprocess per pass; measured 3.11 s at 348 beads).
- Steps: (1) **design note first, with review-mandated contents**: heartbeat write cadence vs the autocommit machinery (`HeartbeatTTL` is 90 s at `store.go:948`, but a median attempt is 10 min — heartbeats must force their own writes or the TTL must be re-derived; forced tracked-row writes either dirty the tree between autocommits or regenerate the commit noise Phase 0 WB-7(c) removed); cross-machine freshness is bounded by git sync cadence, so the note must state the **maximum accepted double-execution window** (age-only staleness across machines) and re-derive TTL from sync cadence; the "synced sidecar collection" alternative has the same sync-latency property and is a layout choice, not an independent mitigation; (2) implement the chosen design; delete the /tmp sidecar scheme and retry sleeps; (3) single-pass queue classification with a shared snapshot; cache `origin/main` head per process with TTL.
- Acceptance: the design note states the double-execution window and the drain honors it; `TestCrossMachineClaimNotStolenWhileFresh` (with the note's window as the assertion bound — review: an in-process pass alone proves nothing about real sync latency, so the note's window is the contract); `bead ready` < 150 ms and `bead status` < 400 ms at live store size, asserted by benchmarks.
- Validation: `cd cli && go test -run 'Claim|Queue' ./internal/bead/...`; `time ddx bead status` before/after recorded in the exit note.
- Non-scope: FEAT-004's <100ms@10k NFR (re-baselined in Phase 2); storage-format changes.
- Dependencies: design note review.

### WB-6: Decompose `execute_bead_loop.go`

- Objective: break the 8,199-line churn epicenter into owned stages.
- Files or systems: `cli/internal/agent/execute_bead_loop.go` → intake / attempt / verify / land / report around the existing composition root `internal/agent/executeloop/state_machine.go`.
- Steps: (1) golden tests freezing status transitions (`TestExecuteBeadStatusGolden` over recorded fixtures); (2) mechanical extraction, one stage per change, no behavior edits; (3) stages consume/produce WB-1's typed values.
- Acceptance: no file in the loop path > 2,500 lines; goldens unchanged; extraction commits are pure moves.
- Validation: `cd cli && go test ./internal/agent/...` green at each step.
- Non-scope: fixing latent bugs found while moving (file beads).
- Dependencies: after WB-1 and WB-2a land.

### WB-7: Closed means the class is dead (mechanism specified per review)

- Objective: reliability beads close only with a live regression tripwire.
- Files or systems: `ClosureGate` (`cli/internal/bead/store.go:1994`) + bead lint; `docs/helix/06-iterate/bead-authoring-template.md`.
- Steps: (1) for beads labeled `kind:reliability`/`kind:bug` in Go projects, closure evidence names an exact `Test*` symbol and package. The closure gate creates or reuses an isolated detached verification worktree at the landed result revision—never checking out or mutating the operator's project root—then proves the test is in the default test set and executes `go test -json -run '^<name>$' <package>`. Closure requires an explicit `pass` event for that test and rejects missing, skipped, build-tag-only, or failing tests with a typed reason. Non-Go projects require an operator-reviewed equivalent tripwire or the explicit `tripwire:waived-non-go` label with rationale; (2) the template gains the criterion; (3) the four defect-family parents from Phase 0's class map close only after their tripwires pass at their landed revisions.
- Acceptance: `TestClosureGateRequiresTripwireForReliabilityBeads`; `TestClosureGateRunsTripwireAtLandedRevisionWithoutMutatingProjectRoot`; table cases prove nonexistent, skipped, build-tag-only, and failing tests are rejected while an exact passing default-suite test is accepted (builds on Phase 0 WB-7's propagation).
- Validation: `cd cli && go test -run 'ClosureGate' ./internal/bead/...`.
- Non-scope: retroactive audit of closed beads.
- Dependencies: Phase 0 WB-7.

### WB-8: Lock-stack consolidation (new — review found this work dangling between plans)

- Objective: one lock package; the three separately-patched implementations (bead mkdir lock, agent tracker lock, gitlock index recovery) collapse onto it.
- Files or systems: `cli/internal/bead/lock.go`, `cli/internal/agent/tracker_lock.go:245-330`, `cli/internal/gitlock/`.
- Steps: extract the Phase 0 WB-5 tombstone-break into a shared package with one acquire/break/release API and lockmetrics instrumentation built in; migrate the three call sites; delete the duplicated stale-break logic.
- Acceptance: one implementation of stale-break in the tree (`grep` for the tombstone rename pattern finds one definition); `TestStaleLockBreakSingleWinner` passes against the shared package from all three call sites.
- Validation: `cd cli && go test -race ./internal/bead/... ./internal/agent/... ./internal/gitlock/...`.
- Non-scope: changing lock scopes or hold budgets (that shape is WB-4/WB-5 territory).
- Dependencies: WB-4's owner exists (worktree/journal locking moves into it); Phase 0 WB-5 (the in-place fix being consolidated).

## Validation

- Per-WB named tests, all in the default suite; WB-4 chaos tests and WB-5 benchmarks are the phase spine.
- Weekly MET-003 (now with the loss-by-stage table) tracks convergence and accumulates the predeclared paired confidence sequence; it cannot close the phase before the sample floors or on unadjusted weekly point estimates.
- Supervised drains (20–50 attempts) after each WB lands; **audits archived durably** — the events go to the attachments stream and the summaries to `docs/helix/06-iterate/metrics/` (review: `.ddx/executions/` is per-machine gitignored exhaust and cannot back an exit note).

## Risks And Rollback

- **Fizeau contract slips** (WB-1): typed-boundary and continuation-dependent WBs stay blocked while independent Phase 1 work proceeds. The remedy is an upstream Fizeau contract/release followed by an explicit DDx version pin and conformance run; the 29 importing files measure the consolidation surface. Schedule pressure does not authorize DDx to invoke or supervise an underlying provider directly.
- **Direct Fizeau dispatch exposes unavailable routes** (WB-2a): Fizeau owns
  availability and fallback. DDx records the typed immediate/terminal outcome;
  WB-2b later applies queue-level cooldown only where the typed contract calls
  for it. The interim cost is monitored in MET-003 — still far below 7,823
  DDx-generated refusals.
- **Single-reviewer default misses defects** (WB-3): escalated tier remains for labeled classes; async full suite backstops; ADR-024 amendment records the trade.
- **WB-4 is the largest change to the most dangerous code** — spec first, goldens first for the land stage, every deletion ships with its replacement test, independent commits.
- **WB-5 heartbeat cadence** — the design note's stated double-execution window is the contract; if tracked-row heartbeats prove noisy, the sidecar-collection layout is the fallback *with the same window math* (not a different risk profile).
- **Extraction churn** (WB-6) — freeze other `internal/agent` merges during each step.

## Open Questions

1. Which Fizeau release first satisfies the typed cause/stage, continuation-capability, cancellation, and provider-process-tree contract? Pin that exact compatible version after the upstream conformance suite passes.
2. What harness-neutral continuation operation, if any, does that Fizeau release
   expose? Where its public result reports continuation unsupported, use a fresh
   Fizeau `Execute` with task + diff + gate evidence inside the same DDx repair
   cycle; no invented service method or direct provider fallback is permitted.
3. WB-4 spec id: fresh FEAT-030 (proposal) vs extending contradiction-laden FEAT-012 (Phase 2 narrows FEAT-012 either way).
4. WB-5 layout: in-row lease facts (default) vs synced sidecar collection — decided by the design note's autocommit-interaction analysis.

## Handoff

Spec artifacts (FEAT-030, ADR-024 amendment, WB-5 design note) are operator-reviewed before dependent code lands. WB-1 first lands the upstream Fizeau contract and pins the compatible release; no dependent DDx bead dispatches before boundary conformance is green. Code WBs are cut into P7-rubric beads referencing this plan's corrected anchors; drained execution only while Phase 0 tripwires stay green. Exit note with the five criteria + MET-003 links appended here before Phase 3 activates (Phase 2 runs in parallel; its ledger rows 3/4 wait for this phase's WB-3, per the cross-plan sequencing note).

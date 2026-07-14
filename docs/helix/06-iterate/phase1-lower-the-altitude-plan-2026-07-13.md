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
**Status:** Revised r3 — adversarial review and execution-readiness repairs applied 2026-07-13
**Source:** AR-2026-07-13-vision-vs-reality.md §7 Phase 1 and §7 "The architectural decision"
**Mode:** Mixed — spec/contract tasks are operator-driven; mechanical extractions and test-backed code tasks may be drained once Phase 0's exit criteria hold.

## Goal

Move DDx from supervising opaque harness processes by text-scraping to consuming typed results and delegating in-flight supervision to the harness. DDx keeps what the substrate lacks — queue, worktree lifecycle, landing, durable evidence — and sheds what the substrate does better. Measured target: `ddx try` is non-inferior to the direct-sub-agent baseline (MET-003) on landed-rate within the predeclared 10-percentage-point margin, with paired landed-bead wall-clock within 1.25× of arm B. The paired decision rules and sample floors are exit criterion 1; weekly point estimates alone cannot close the phase.

## Scope

In scope: the fizeau/harness result contract, failure classification, preflight/cooldown posture, the close gate, worktree lifecycle ownership (spec + implementation), claim liveness, lock-stack consolidation, the loop-file decomposition, and the closed-means-class-dead process rule.

Non-goals: server/worker coordination (Phase 3 — including `TestIntegration_ManualWorkerContinuesOfflineAndReconciles`, which review moved out of this phase: it needs coordination machinery only Phase 3 builds); doc-truth work beyond the two spec artifacts this phase forces (ADR-024 amendment, new worktree spec); any new feature surface.

### Entry criteria

Phase 0 exit note appended. Exception (pre-authorized there): WB-2a may start during Phase 0 if routing refusals dominate the loss taxonomy.

### Exit criteria (revised)

1. **Paired MET-003 non-inferiority, with predeclared error control:** each pair is the same bead run from the same pinned base revision with the same model tier and equalized landed predicate. For landed-rate, encode each pair's arm-A-minus-arm-B result as `-1`, `0`, or `1`; after at least 100 complete pairs, the lower bound of an anytime-valid 95% confidence sequence for the mean paired difference must be greater than `-0.10` (the 10-point non-inferiority margin, consistent with Phase 3). MET-003 must name the confidence-sequence method and alpha before the first outcome is examined; weekly evidence may accumulate into that sequence, but the old two-consecutive-20-bead rule and unadjusted repeated CIs are not gates. Wall-clock is measured from dispatch start through the equalized landed-predicate verdict, including DDx close-gate and repair overhead. On at least 40 pairs where both arms land, the predeclared one-sided anytime-valid 95% paired confidence sequence for the median per-bead elapsed-time ratio `arm A / arm B` must have an upper bound less than `1.25`; timeouts use MET-003's predeclared cap rather than being dropped. If either sequence is inconclusive or its sample floor is unmet, the phase stays open and collection continues under the predeclared sequential rule.
2. DDx-owned stages account for < 10% of attempt losses — measurable because WB-1's typed causes carry a `stage` field (routing / scratch / land / review-transport / substrate) and the MET-003 weekly report gains a loss-by-stage table (review: this attribution did not previously exist anywhere).
3. Zero recurrence of any `niflheim-*` defect class for 3 consecutive weeks (per-class tripwires from the Phase 0 class map green; no new incident labels).
4. **Pattern-table lint green** (replaces the original grep criterion, which review showed both contradicted WB-3 and missed `cli/cmd`): a checked-in lint (evidencelint pattern) forbids substring/regex failure-classification tables outside the single `failclass` package, covering `cli/internal/...` **and** `cli/cmd/...`, with an explicit annotated whitelist for non-classification uses of substring matching.
5. Median landed-bead wall-clock < 15 minutes.

## Assumptions

- fizeau (`github.com/easel/fizeau` v0.14.50) is operator-controlled. **Review-corrected sizing:** 29 non-test Go files import fizeau (server resolvers, providers, profile_select, review, retry, dispatch, cmd/try...), and there is no Go Agent SDK (anthropic-sdk-go in go.mod is an indirect API client, not a harness runner). The WB-1 fallback is therefore scoped: v1 replaces spawn/stream/result for the **claude CLI (stream-json) only**; codex/gemini stay routed via fizeau until their typed streams are built. "Fizeau demoted to routing library" keeps its routing/model-discovery symbols; the classifier is identical either way.
- Harness session continuation (`claude --resume`) exists for in-band repair; for other harnesses the fallback (fresh spawn seeded with diff + failure output) is the mechanism.
- Phase 0 tripwires stay green throughout; any regression pauses drained execution of this phase.
- "DDx owns execution; workflows own ordering" remains the layer boundary.

## Work Breakdown

Ordering: WB-2a immediately (no dependencies, largest single failure class); WB-1 next (riskiest dependency, 2-week timebox); WB-4's spec and cleanup-responsibility inventory are written in parallel week 1; WB-3 and WB-2b after WB-1; WB-8 after WB-4's owner exists (locks move into it); WB-6 after WB-1/2 land; WB-5 and WB-7 independent. WB-4 may migrate responsibilities incrementally, but deletion of `ExecutionCleanupManager` waits for every replacement owner and dependency in the WB-4 contract.

### WB-2a: Advisory preflight — no dependencies, start immediately

- Objective: kill predict-and-refuse: 7,823 "no viable routing candidate" refusals = 96% of all disruptions. (Review split this out of the old WB-2: it needs nothing typed, it is reliability principle P1, and it may be needed to pass Phase 0's own success gate.)
- Files or systems: **corrected anchors** (review: the original `internal/server/workers.go:803` contains no routing code): `cli/internal/agent/execute_bead_loop.go:7595,7826` (preflight refusal path), `cli/cmd/agent_execute_loop_escalation.go:374`, `cli/internal/agent/readiness_classification.go:257`.
- Steps: preflight may annotate, never refuse; dispatch proceeds and the terminal result says what happened; the `no viable routing candidate` disruption class is downgraded to an annotation event. Before the validation drain, write a cohort manifest containing the 50 new attempt IDs plus UTC start/end bounds; a checked-in audit query accepts that manifest and ignores events outside both the ID set and time window.
- Acceptance: `disruption_detected` events with that cause == 0 for the newly generated 50-attempt cohort; refusal path deleted, not flag-gated. The audit-query regression fixture contains a historical matching event outside the cohort and proves it is excluded.
- Validation: run the cohort-scoped audit query over `.ddx/attachments/*/events.jsonl` with the recorded attempt manifest and UTC bounds, archive the manifest and zero-count result with the drain evidence, and report before/after counts for that cohort only. A repository-wide historical tally is diagnostic and cannot fail this acceptance criterion.
- Non-scope: cooldown changes (WB-2b); bead-readiness intake lint (ADR-023 WARN-ONLY stays).
- Dependencies: none.

### WB-1: Typed terminal results; one failure-classification owner

- Objective: every reliability decision consumes a typed result with a `stage` attribution; free-text scraping is deleted, not bypassed.
- Files or systems: fizeau upstream (CONTRACT-003 amendment: terminal event = outcome enum + machine-readable cause + stage + usage/cost + session ref; spawn contract sets pdeathsig or supervised group — incident ddx-01b89378, 28 orphaned codex processes). Deletion/rewire list (review-extended — the original list left six scraping sites alive): `cli/internal/agent/executor.go:101-140,275-295` (22 kill-regexes), `provider_failure.go:85-192`, `execute_bead_status.go:124-215` (+ the `containsAny` helper at `:255`), `cli/cmd/agent_execute_loop_escalation.go:44-59`, `cli/internal/escalation/infrastructure.go:22-78`, `readiness_classification.go:204-257`, `server_outage.go:155-163`, `execute_bead_validation.go:119,130`, `triage_dispatch.go:10`, `execute_bead_loop.go:7824`. `execute_bead_review_classification.go:42-48` (FEAT-022 review-error classes) is **kept but rewired** in WB-3 to consume typed causes. New single owner: `cli/internal/agent/failclass` (or folded into `internal/agent/executeloop/state_machine.go`).
- Steps: (1) contract as a versioned type + conformance tests both sides; (2) bump fizeau; translate typed causes in the one classifier; (3) all retry/escalate/park/cooldown decisions route through its verdict; (4) delete/rewire the listed sites; runs are cancelled only on operator signal, timeout, or typed fatal event — never log text; (5) keep the PATH shim only if fizeau cannot own process-group lifetime; (6) ship the exit-criterion-4 pattern-table lint in the same change so deleted patterns cannot regrow.
- Acceptance: pattern-table lint green over `cli/internal` + `cli/cmd`; `TestProviderTerminalEventRoundTrip`; `TestFailclassVerdictsDriveSingleDecision` (one synthetic failure → exactly one of retry/escalate/park); orphan test (kill `ddx run` mid-flight → no surviving provider tree); typed causes carry `stage` (feeds exit criterion 2 and the MET-003 loss-by-stage table).
- Validation: `cd cli && go test ./internal/agent/... ./internal/escalation/... ./cmd/...`; supervised 20-attempt drain with typed causes on every terminal event.
- Non-scope: routing policy (fizeau keeps routing); review content.
- Dependencies: fizeau release. **Decision point at 2 weeks:** switch to the scoped fallback (claude stream-json integration; fizeau demoted to routing library for codex/gemini) rather than waiting. The fallback surface is the 29-import list above — sized as real work, not "only the producer changes."

### WB-2b: Typed-evidence cooldowns

- Objective: cooldowns fire only on typed evidence and always expire.
- Files or systems: cooldown derivation `cli/internal/bead/store.go:3263-3272` (`originMainHead` returns `""` on any error → origin-bound cooldowns silently never clear offline).
- Steps: key cooldowns on typed causes (quota-exhausted, provider-fatal); wall-clock expiry independent of network reachability.
- Acceptance: `TestCooldownExpiresOffline`.
- Validation: `cd cli && go test -run 'Cooldown' ./internal/bead/...`.
- Dependencies: WB-1.

### WB-3: Risk-proportional close gate with in-band repair (ADR-024 amendment)

- Objective: default close path ≤ 1 LLM invocation of overhead; review failures repair in-band.
- Files or systems: ADR-024 amendment; `candidate_cycle.go` (737 lines), `execute_bead_review.go` (1,492 lines), `execute_bead_review_group.go`; `execute_bead_review_classification.go:42-48` rewired to typed causes (from WB-1).
- Steps: (1) amend ADR-024: default tier = deterministic checks + one reviewer at implementer power; escalated tier (two slots, MinPower, repair cycles) only for `kind:safety`, `area:storage`, `kind:migration`, or operator pin; (2) review transport errors (41% today; 308/345 zero-byte) retry once then downgrade to deterministic-checks-only with an operator-visible event — never fail the attempt; (3) gate failure resumes the implementing session (`claude --resume`; other harnesses: fresh spawn seeded with diff + failure output).
- Acceptance: `TestDefaultCloseGateSingleReviewer`, `TestReviewTransportErrorDowngradesNotFails`, `TestGateFailureResumesSession` (or documented fallback); escalated tier fires only on labeled classes (table test).
- Validation: `cd cli && go test -run 'CloseGate|ReviewTransport|GateFailure' ./internal/agent/...`; review-error ratio < 5% over a week; MET-003 wall-clock trend.
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
| Attempt process-group discovery and reaping | WB-1's typed provider/process supervisor plus a narrow `AttemptProcessReaper` for crash recovery | WB-1 typed terminal/process-lifetime contract | Existing `ExecutionCleanupManager_*Process*` coverage migrated to the new owner, including kill-parent/no-surviving-provider-tree |
| Complete execution evidence and `.ddx/runs` scanning/retention, preserving active or incomplete evidence and `executions.retain_days` | Extracted `ExecutionRetention` owner, evidence policy | Extraction bead; no Phase 3 run-substrate redesign required | Existing active/incomplete evidence and retain-days tests pass unchanged against `ExecutionRetention` |
| Agent-log retention, including active-session preservation | Extracted `ExecutionRetention` owner, agent-log policy | Extraction bead | Existing active-session and retain-days agent-log tests pass unchanged against `ExecutionRetention` |
| Worker-directory retention, including live-PID preservation | Extracted `ExecutionRetention` owner, worker-directory policy; Phase 3 may later rehome it without changing this gate | Extraction bead | Existing live-worker and retain-days worker-dir tests pass unchanged against `ExecutionRetention` |
| Landed candidate-ref pruning after durable result metadata exists | WB-6 land/report stage's `CandidateRefRetention` owner | WB-6 goldens and land/report extraction | Existing durable-result/candidate-ref tests plus a named no-prune-before-durable-result test |

The table is a deletion gate, not documentation after the fact: each row must name its landed replacement commit and test in the WB-4 exit evidence. A responsibility may remain temporarily in `ExecutionCleanupManager`, but no implementation bead may remove it, its config, its summary/event fields, or its CLI observability before that row's owner and proof are green. `ExecutionCleanupManager` itself is deleted only after every row has migrated and the composed `ddx cleanup` output retains equivalent audit and dry-run coverage.

- Acceptance (review-corrected): all migration-table rows name a landed replacement and passing regression test; `TestExecutionCleanupReplacementParity` proves the composed owners preserve the old manager's protected/deleted fixture sets before the manager is removed; `TestChaos_SIGKILLDuringWorktreeAddReclaimsOnNextStart`; `TestChaos_PreDispatchMutationWindowDoesNotHoldLockAcrossHarnessWait` (the TP-021 test that belongs to this layer — the offline-reconcile TP-021 test moved to Phase 3); `TestUnlockIfOwnerDeadPrunesLockedInitializing` (replaces the unusable `grep "worktree unlock"` criterion — git args are separate strings, the grep could only match comments); landing test asserts no write to a checked-out branch with a dirty operator tree.
- Validation: `cd cli && go test -race -run 'Chaos|Worktree|Land|Unlock' ./internal/agent/... ./internal/git/... ./cmd/...` (cmd/ added per review); 50-attempt drain leaves `git worktree list` clean.
- Non-scope: epic execution contract redesign (Phase 2 re-specs the container/auto-decompose divergence).
- Dependencies: spec review; WB-6 goldens for the land stage land first or together. Deleting `ExecutionCleanupManager` additionally depends on Phase 0 WB-3, WB-1's process owner, WB-5's claim-liveness owner, WB-6's candidate-ref owner, every `ExecutionRetention` policy extraction, and green replacement-parity evidence for all rows above.

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

- **fizeau contract slips** (WB-1): 2-week timebox, then the scoped fallback (claude-only stream integration; fizeau keeps routing for codex/gemini). Honestly sized: 29 importing files, three harnesses ≠ one integration. The classifier is producer-independent, so that work is never wasted.
- **Advisory preflight dispatches into dead providers** (WB-2a): bounded by WB-2b typed cooldowns once they land; in the window between 2a and 2b, cost of failed dispatches is monitored in MET-003 — still ≪ 7,823 refusals.
- **Single-reviewer default misses defects** (WB-3): escalated tier remains for labeled classes; async full suite backstops; ADR-024 amendment records the trade.
- **WB-4 is the largest change to the most dangerous code** — spec first, goldens first for the land stage, every deletion ships with its replacement test, independent commits.
- **WB-5 heartbeat cadence** — the design note's stated double-execution window is the contract; if tracked-row heartbeats prove noisy, the sidecar-collection layout is the fallback *with the same window math* (not a different risk profile).
- **Extraction churn** (WB-6) — freeze other `internal/agent` merges during each step.

## Open Questions

1. WB-1 timebox: 2 weeks confirmed? (Same operator owns fizeau; the timebox forces the decision.)
2. WB-3 in-band repair for non-Claude harnesses: fresh-spawn-with-context as the only mechanism — acceptable? (Proposal: yes.)
3. WB-4 spec id: fresh FEAT-030 (proposal) vs extending contradiction-laden FEAT-012 (Phase 2 narrows FEAT-012 either way).
4. WB-5 layout: in-row lease facts (default) vs synced sidecar collection — decided by the design note's autocommit-interaction analysis.

## Handoff

Spec artifacts (FEAT-030, ADR-024 amendment, WB-5 design note) are operator-reviewed before dependent code lands. Code WBs are cut into P7-rubric beads referencing this plan's corrected anchors; drained execution only while Phase 0 tripwires stay green. Exit note with the five criteria + MET-003 links appended here before Phase 3 activates (Phase 2 runs in parallel; its ledger rows 3/4 wait for this phase's WB-3, per the cross-plan sequencing note).

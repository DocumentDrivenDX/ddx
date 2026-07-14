---
ddx:
  id: IP-2026-07-13-phase0-stop-self-harm
  type: implementation-plan
  status: reviewed
  depends_on:
    - AR-2026-07-13-vision-vs-reality
    - FEAT-006
    - TD-027
    - TD-031
---
# Phase 0 Plan: Stop the Self-Harm

**Date:** 2026-07-13
**Status:** Revised r4 — corrected Fizeau runtime boundary applied 2026-07-13; prior review findings remain incorporated
**Source:** AR-2026-07-13-vision-vs-reality.md §7 Phase 0
**Mode:** Operator-driven. The queue must not be trusted to repair itself until this phase completes; every task here is executed or directly supervised by the operator, not drained autonomously.
**Duration honesty:** the hands-on work is days, but exit criteria 3–5 need 2–3 calendar weeks of observed drain under a single supervised worker.

## Goal

Remove the mechanisms by which DDx actively damages its own host, repository, and work product, and stand up the measurement that arbitrates the rest of the recovery. Restore the ~48% attempt-success plateau (May/June level) as a floor, not a target — Phase 1 raises the ceiling.

## Scope

In scope: host reclamation, scratch/lease placement, the two lock-corruption sources, gate truthfulness (including test hermeticity against host config), the three correct-work-destroyers, the weekly A/B baseline, and the intake freeze that protects this work.

Non-goals: designing or implementing the typed boundary, review-gate redesign, or worktree ownership refactor (Phase 1 owns them; Phase 0 only gates affected work on the corrected boundary); lock-implementation *consolidation* (Phase 1 WB-8 owns it; this phase only fixes the TOCTOU in place); spec edits beyond what a code fix forces (Phase 2); server work (Phase 3). No new features of any kind.

Runtime boundary: Fizeau is the complete agent runtime and harness-of-harnesses. It exclusively invokes, parses, routes, and supervises Claude/Codex/Gemini sessions, and owns any continuation a future public contract exposes. Phase 0 cleans and verifies only DDx-owned queue, git/worktree, lock, claim, and evidence resources; DDx consumes Fizeau's current public immediate-error/final-event result and decides bead success or another attempt from repository evidence.

### Entry criteria — host-wide quiescence (revised)

The scratch roots being reclaimed are **shared across projects and across two OS namespaces**: `~/.ddx/config.yaml:39` sets `temp_worktree_root: /Users/erik/Projects/.ddx-exec-wt`, which contains entries for at least fjord, heimq, pqueue, snorri, and this repo, plus the **live** `ddx-claim-heartbeats` subtree; and macOS-side ddx processes are invisible to a Linux-side `pgrep`. Therefore, before any deletion:

1. Enumerate DDx workers for **every** project whose `ExecutionWorktreeRoot`
   resolves to the shared root — on both sides of the `/Users` mount (verified
   live today: a snorri worker and a ddx worker, both `--server-managed`). Do not
   assume Fizeau exposes active-session enumeration or cancel-by-session.
2. Stop the DDx orchestration workers: set each project's
   `.ddx/workers/desired.json` count to 0 **and** stop the ddx-server supervisor
   that respawns them (a fresh worker for this repo appeared today at 12:51 while
   this plan was being reviewed — the supervisor must be disabled first or it
   will race the reclamation). Graceful worker stop cancels each live context
   supplied to Fizeau `Execute`; abrupt worker death relies on the pinned Fizeau
   caller-death/process-tree guarantee. DDx does not query, signal, or reap a
   provider process.
3. Verify `pgrep -af 'ddx (work|try|run)'` is empty on Linux and with the
   equivalent macOS-side check. Require the pinned Fizeau caller-death and
   process-tree conformance tests to be green before deletion. A provider-process
   `pgrep` is diagnostic contract-violation evidence only and never a DDx cleanup
   authority.

### Exit criteria (revised — each names its measurement)

1. **Leak-free drain:** zero new DDx-created entries lacking DDx ownership
   metadata under a cohort-private `TMPDIR` and the configured
   `temp_worktree_root` after a supervised 20-attempt drain. The cohort manifest
   records the starting inventory, attempt IDs, and UTC bounds; unrelated host
   files are excluded. Measured by the WB-3 tripwire plus the scratch audit
   command WB-3 delivers (`ddx cleanup --audit-unowned --cohort <manifest>`, a
   dry-run listing that must print none). Raw `/tmp` inode percentage and
   pre-existing non-DDx entries are diagnostics, not acceptance criteria.
2. **Worktree-clean drain:** `git worktree list --porcelain | grep -c '^worktree'` == 1 before and after the same drain; zero new `.ddx-exec-wt` entries older than their attempt's lifetime.
3. **No unrelated-gate refusals:** zero gate refusals whose failing package lies outside the bead's diff, over 7 consecutive drain days — measured mechanically from the structured `gate_failed` events WB-6 adds (each names the failing package; compared against the bead diff), not by grepping rationale prose. Until those events exist, the interim protocol is a daily operator review of every `no_changes_rationale.txt` against a written checklist.
4. **Plateau restored:** attempt success (landed work) ≥ 45% over the most recent 100 attempts, computed from `.ddx/metrics/attempts.jsonl` **excluding** A/B arm-B rows (which live in a separate file per WB-8).
5. **Baseline running:** first two weekly A/B reports published (WB-8).

## Assumptions

- The org spend limit permits the A/B baseline runs (~40 attempts/week).
- The corrected CONTRACT-003 Fizeau session-result contract is published and FEAT-006 identifies DDx as its consumer. WB-7(a) and WB-8 do not start until boundary conformance proves DDx neither invokes provider CLIs nor parses provider output; Fizeau routing and session lifecycle remain out of Phase 0 scope.
- The bead tracker core is healthy (chaos-tested per AR §3.4) — Phase 0 changes nothing about storage format.
- Attempt volume is throttled to what the operator can supervise (single worker, `--once` batches).

## Approval And Tracker-Mutation Boundary

This document is a plan, not authorization to mutate the queue. The operator must approve and check in this revised plan before WB-1 changes any bead. After approval, every tracker mutation in this phase must go through the supported `ddx bead` CLI (`create`, `update`, `dep add`/`dep remove`, or `close` as applicable); `.ddx/beads.jsonl` must never be edited directly. After each coherent mutation batch, verify it with `ddx bead show`, `ddx bead list`, `ddx bead ready`, and/or `ddx bead status`, then stage and commit `.ddx/beads.jsonl` promptly. Use a tracker-only commit when the mutation stands alone, or fold it into the directly related implementation/docs commit when that commit is already being prepared.

## Work Breakdown

### WB-1: Intake freeze and WIP ratchet (recounted)

- Objective: stop new feature work; consolidate the reliability backlog into per-class work.
- Files or systems: bead tracker (status, relationships, labels, and explanatory metadata via `ddx bead` only); the shared bead-creation policy in `cli/internal/bead/` and its CLI/server/agent callers; harness guidance (`AGENTS.md`, `CLAUDE.md`, and generated/copied DDx skill guidance) as explanatory documentation, not as the enforcement boundary.
- Steps: (1) **only after the plan approval gate above**, park open `kind:feature` beads in `area:server`, `area:ui`, `area:federation`, `area:website` (live count: ~13 of 21 open feature beads) in the operator lane with `ddx bead update <id> --status proposed --set phase0-freeze=true`; `status=proposed`, not `blocked` or the explanatory metadata, is what makes them ineligible for autonomous workers. Feature beads in other areas (agent/bead/axon/migration) are individually triaged — park them the same way unless they *are* reliability work mislabeled. Thaw requires an explicit operator transition back to `open` after the ratchet condition clears. (2) Enumerate **all 15** open `incident:niflheim-*` classes (not just the four largest) and write an explicit consolidation mapping: multi-bead classes (cleanup-io ×13, provider-test-cpu ×16) each get one parent created with `ddx bead create`; wire relationships with `ddx bead dep add`/`ddx bead update --parent`; mark symptom duplicates with `ddx bead update <id> --status cancelled --set superseded-by=<parent-id>` rather than closing unfinished duplicates as completed; singleton classes keep their single bead and are tagged to one of the four defect families (scratch/process-reap/review/liveness) for Phase 1's per-class tripwires. (3) Implement the ratchet at the shared store-level creation boundary: while the count of open `kind:reliability` beads is greater than 40, reject creation of a bead labeled `kind:feature` with a structured error naming the count, threshold, and thaw condition. The guard must cover `ddx bead create`, server REST/GraphQL mutations, and agent-generated beads; add parity tests so no harness or UI can bypass it. Mirror the rule in cross-harness guidance for discoverability, but do not rely on prompt text for enforcement. (4) Verify the approved mutation batch, then commit the resulting tracker change under the policy above.
- Acceptance: the consolidation mapping is checked in as `docs/helix/06-iterate/niflheim-class-map-2026-07.md`; every bead in the pre-mutation inventory of open `incident:*` work appears in it exactly once, including duplicates later marked `cancelled`; every frozen feature bead is `status=proposed` and absent from `ddx bead ready --execution`; open-bead count target is derived from the mapping arithmetic (review measured ≈ 65 after park+consolidation — the acceptance is "matches the mapping," not a round number); `TestFeatureIntakeRatchetRejectsCreateAboveThreshold`, `TestFeatureIntakeRatchetAllowsCreateAtOrBelowThreshold`, and creation-surface parity tests prove the guard is shared by CLI, server, and agent paths.
- Validation: archive before/after output from `ddx bead list --status open --json | ddx jq -r '.[].labels[]' | sort | uniq -c`; verify each changed bead with `ddx bead show <id>`; verify frozen beads do not appear in `ddx bead ready --execution`; run the focused ratchet and mutation-parity tests plus `lefthook run pre-commit`; confirm the tracker mutation is committed and `.ddx/beads.jsonl` is not left dirty.
- Non-scope: cancelling feature beads; a general bead-lint or readiness redesign beyond the narrow shared creation guard.
- Dependencies: none.

### WB-2: Host reclamation (one-time, destructive — operator confirms each step)

- Objective: clean baseline on every root the tripwires will watch. Inventory at review time (2026-07-13 late): `/tmp` inodes already back to 4% with zero `ddx-home-*` dirs (earlier 406-dir/100% state has partially self-cleared or been cleaned); what remains: ~27k stale heartbeat scope dirs under `/tmp`, the **live** `ddx-claim-heartbeats` under the shared exec-wt root, 2 locked worktree registrations, stale `.ddx-exec-wt` entries and 6 `.ddx-shared-cache-*` dirs. **Re-inventory at execution time before every deletion** — this state moves.
- Files or systems: `/tmp/ddx-claim-heartbeats/` (stale, Linux-side), `/Users/erik/Projects/.ddx-exec-wt/` including its `ddx-claim-heartbeats` (LIVE — `ClaimLivenessRoot` prefers the configured runtime root, `cli/internal/bead/claim_liveness.go:51-59`) and `.ddx-shared-cache-*` (gocache, `attempt_backend.go:483-492`), and the two `locked initializing` registrations. Fizeau/provider-owned temp roots are inventory-only here and are cleaned through Fizeau or by the operator, never by DDx.
- Steps: (1) host-wide quiescence per entry criteria — **hard prerequisite**; (2) snapshot inventory listings to `docs/helix/06-iterate/phase0-reclaim-inventory.md` (durable, not `.ddx/executions/` which is per-machine exhaust); (3) delete stale `/tmp` heartbeat dirs and exec-wt scratch; the live `ddx-claim-heartbeats` subtree is deleted **only** after step 1 confirms every owning project quiescent — deleting a live heartbeat makes a fresh claim look stale → double execution; (4) worktree registrations: `git worktree unlock <path>` then `git worktree prune` (the registered paths don't exist on disk, so `git worktree remove` would error — review-verified; unlock→prune is the working sequence); (5) remove `.ddx-exec-wt` entries older than 7 days whose owning project is quiescent, and all `.ddx-shared-cache-*`.
- Acceptance: `git worktree list --porcelain | grep -c '^worktree'` == 1; inventory doc committed; zero heartbeat dirs whose owning project was non-quiescent were touched (inventory shows the exclusion set).
- Validation: re-run the listing commands; diff against inventory.
- Non-scope: `.ddx/executions/` history (retained as evidence); any code change.
- Dependencies: entry criteria.

### WB-3: Scratch/lease placement — rescoped to what actually remains

Review finding: much of the originally planned "relocation" already exists (`config.ExecutionTempRoot` avoids `os.TempDir` with a UserCacheDir fallback; `ExecutionScratchRoot`/`MkdirExecutionScratch` exist; `ClaimLivenessRoot` already honors the configured runtime root; tmp-sidecar reaping exists at `claim_liveness.go:147-183`; the `ddx-home-*` producers were **test fixtures**, already decomposed into beads ddx-7326dd54 / ddx-e03399c8). The remaining real work:

- Objective: no DDx-created resource under any unowned root; cleanup only visits roots carrying ownership metadata.
- Files or systems: `cli/internal/agent/execution_cleanup.go:845,853` (the two `os.TempDir()` appends — note the code comment says they exist to sweep legacy debris, so WB-2 must complete on every affected host first); `cli/internal/agent/runner_test.go:29-34,56-61` (the fixture producers — execute existing child beads ddx-7326dd54/ddx-e03399c8); `cli/internal/bead/claim_liveness.go` (published heartbeat files and per-project scope dirs have no reaper — the tmp-sidecar reaper does not cover them); repo-wide audit for remaining direct `os.TempDir()` / `os.MkdirTemp("")` call sites in production code.
- Steps: (1) after WB-2, delete the two legacy appends; (2) land the two existing fixture beads; (3) add scope-dir/published-heartbeat reaping keyed on ownership metadata; (4) call-site audit with a short table (site → owner → resolved root) appended to this plan, rejecting any DDx cleanup claim over a Fizeau-owned session resource; (5) tripwire test `TestAttemptLeavesNoUnownedTempDirs` — point `TMPDIR` at a private per-test directory and drive the DDx attempt boundary with a fake typed Fizeau session result, asserting only DDx-owned resources; (6) deliver the audit command exit criterion 1 uses: `ddx cleanup --audit-unowned` listing DDx-owned entries under watched DDx roots that lack ownership metadata.
- Acceptance: tripwire green in the default suite; `ddx cleanup --audit-unowned` empty after a 20-attempt drain; the two `os.TempDir()` appends gone.
- Validation: `cd cli && go test -run 'TestAttemptLeavesNoUnownedTempDirs' ./internal/agent/...`; audit command output archived with the exit note.
- Non-scope: single-owner `AttemptWorkspace` refactor (Phase 1 WB-4); new config knobs — all roots derive from the existing `ExecutionWorktreeRoot`/`ExecutionTempRoot` machinery (review resolved former open question 1).
- Dependencies: WB-2.

### WB-4: Lock-cap watchdog must never delete a live lock

- Objective: remove corruption source A.
- Files or systems: `cli/internal/lockmetrics/lockcap.go:106-158` (`enforceCapViolation` does `os.RemoveAll` on `.git/index.lock` / the tracker lock while the holder's critical section continues; the cap timer wraps the entire retry loop at `gitlock.go:207`, and the durable-audit section runs 4–6 git subprocesses under the same 30 s cap, making the race routine).
- Steps: change cap enforcement to observe-and-alert only (structured violation event, no unlink); keep telemetry; repurpose the force-release knob as alert threshold.
- Acceptance: no code path in `lockmetrics` calls `os.Remove*` on a path it did not create; `TestLockCapViolationDoesNotReleaseLiveLock` (cap fires while a real holder sleeps in the critical section; lock survives; holder completes).
- Validation: `cd cli && go test ./internal/lockmetrics/... ./internal/gitlock/...`; `lefthook run pre-commit`.
- Non-scope: consolidating the three lock implementations — **owned by Phase 1 WB-8** (review found the original deferral pointed at work no plan owned); changing cap values.
- Dependencies: none.

### WB-5: Fix the TOCTOU stale-lock break in place (all three sites)

- Objective: remove corruption source B without waiting for consolidation.
- Files or systems: `cli/internal/bead/lock.go:89-110`; `cli/internal/agent/tracker_lock.go:245-330` (stale-break call at `:248`, `RemoveAll` at `:282/:317/:327` — re-anchored per review; `:124` is the `WithMainGitLock` wrapper); `cli/internal/gitlock/` recovery cascade.
- Steps: replace delete-then-acquire with rename-to-tombstone-then-acquire at each site: breaker atomically renames the stale lock dir to a unique tombstone (rename fails for the loser → loser re-enters wait), then removes only its own tombstone.
- Acceptance: cross-process test `TestStaleLockBreakSingleWinner` (two `os/exec` children race one stale lock; exactly one acquires; the winner's fresh lock is never deleted) green under `-race -count=20`.
- Validation: `cd cli && go test -race -run 'StaleLock' ./internal/bead/... ./internal/agent/... ./internal/gitlock/...`.
- Non-scope: unification into one package (Phase 1 WB-8); threshold changes.
- Dependencies: none.

### WB-6: Gate truthfulness — hermetic tests, attributed refusals, scoped gates (rewritten per review)

Review corrections adopted: (a) `internal/server/perf` is **already** behind `//go:build perf` with its own non-blocking CI lane (`ci.yml:283-300`) — the original step and its acceptance were already true and are dropped; (b) `TestWorkerManagerStopStaleDiskEntry` is **not** deterministically red — it fails only on hosts whose global `~/.ddx/config.yaml` sets `temp_worktree_root` (passes with `HOME` neutralized), so the defect class is *host-config leakage into tests*; (c) no full-suite `go test ./...` gate was found inside the loop's pre-close path — the plausible refusal locus is the **bead AC template itself**, which auto-inserts `cd cli && go test ./...` (`recovery_decompose.go:613`) for the agent to run.

- Objective: refusals attributable, tests hermetic, gates scoped to the work.
- Steps: (1) **attribution first**: instrument the loop to emit a structured `gate_failed` event naming the DDx-run command and failing package(s) whenever DDx's repository verification fails; typed Fizeau session failures are recorded from the session result without parsing rationale or provider output. Run 3–5 supervised drain days and confirm where broad-suite refusals originate (AC-template text, DDx-constructed verification, or a typed Fizeau result) before cutting further beads. (2) **Hermeticity**: tests must not read the host's global `~/.ddx/config.yaml` — add a test-scoped config isolation (fixture HOME or explicit config injection), fix the diverged helper at `workers_prune_test.go:308` by deriving the heartbeat path from the production `ClaimLivenessRoot` function, and sweep for other host-config-sensitive tests (`cd cli && env HOME=/nonexistent go test ./...` as the detector). (3) **AC template**: change the auto-inserted final gate from `cd cli && go test ./...` to the bead's touched packages plus the curated core-invariant suite (list checked into `docs/helix/06-iterate/core-invariant-suite.md`) — this requires the small diff→packages mapper the review noted is real work: map the attempt's changed files to Go packages (`go list` over dirs), sized as its own bead. (4) Full suite runs async post-land; failures auto-file one `kind:regression` bead per failing package with the landing commit in `Extra`.
- Acceptance: `cd cli && env HOME=/nonexistent go test ./...` and `cd cli && go test ./...` both green on main; `gate_failed` events present in a supervised drain with package attribution; AC template emits scoped gate text (fixture test on `recovery_decompose.go` output); async lane files a regression bead when seeded with a deliberate breakage.
- Validation: exit criterion 3's mechanical computation runs on the new events; `cd cli && go test -run 'StaleDiskEntry' ./internal/server/...` green with and without host config present.
- Non-scope: lefthook pre-commit content; in-band gate repair (Phase 1 WB-3).
- Dependencies: WB-1 (freeze prevents new red from feature work).

### WB-7: Stop destroying correct work

- Objective: three surgical fixes where the orchestrator or tracker converts success into failure.
- Files or systems: (a) the legacy retry-rationale parser — the orchestrator synthesizes an iteration commit and runs full hooks after interpreting provider prose as `status: open` (bug ddx-232aa2c0; 51 attempts destroyed); replace it with the corrected FEAT-006/CONTRACT-003 consumer boundary; (b) `cli/internal/bead/store.go:2063-2068` — `closeWithEvidence` **swallows the existing** `ErrClosureGateRejected` (defined at `store.go:1976` — review: do not re-invent the type) and returns nil; `execute_bead_loop.go:4247-4257` counts it as success; (c) `cli/internal/agent/durable_audit.go:32-60` — per-attempt staging + commit of tracked audit files under the shared tracker lock.
- Steps: (a) delete free-text retry-rationale interpretation. Consume the typed Fizeau session result, inspect the resulting repository diff and verification evidence, and either land/close or record a new DDx attempt; never synthesize a commit merely because provider prose resembles a retry request, and never resume or invoke the provider from DDx. (b) Propagate the existing closure-gate error; the loop records a distinct failure outcome and surfaces the gate reason on the bead. (c) Batch audit rows and commit once per drain iteration outside the lock window, flush on shutdown.
- Acceptance: `TestTypedSessionResultDoesNotSynthesizeCommit`, `TestRepositoryEvidenceDeterminesAttemptOutcome`, `TestClosureGateRejectionPropagatesToLoop`, `TestDurableAuditCommitOutsideTrackerLock`.
- Validation: `cd cli && go test -run 'TypedSessionResult|RepositoryEvidence|ClosureGateRejection|DurableAudit' ./internal/agent/... ./internal/bead/...`; tracker-noise ratio in `git log --oneline -100` < 10%.
- Non-scope: the completion-contract redesign (Phase 1).
- Dependencies: corrected CONTRACT-003 Fizeau session-result contract and FEAT-006 consumer conformance for step (a); none for steps (b)–(c).

### WB-8: Weekly A/B baseline — the arbiter metric (methodology rewritten per review)

Review found three structural holes (no base-rev pinning, asymmetric success predicates, arm-B rows polluting `attempts.jsonl` — which has no `labels` field; `ddx try` has no tagging or `--no-merge` flag, despite CLAUDE.md advertising one — that stale doc line goes to Phase 2's phantom-command sweep). The corrected methodology also keeps both arms behind Fizeau's public runtime boundary:

- Objective: same beads, DDx orchestration over Fizeau versus a Fizeau-only session baseline, measured fairly; closing the orchestration delta becomes the top-line metric.
- Files or systems: new `scripts/ab-baseline.sh`; new metric doc `docs/helix/06-iterate/metrics/MET-003-ab-baseline.md`; separate results file `.ddx/metrics/ab-baseline.jsonl` (never `attempts.jsonl` — keeps exit criterion 4's window clean).
- Steps: (1) sample 20 ready beads stratified by area and size; **pin one base rev, the same request facts/abstract `MinPower` floor, and one immutable comparison-wide operator-passthrough envelope for both arms**. The envelope is identical on every arm and excluded from grouping, filtering, grading, and comparison policy. DDx may raise `MinPower` only for a distinct new attempt after capability-sensitive evidence, or by review intent for a reviewer; route, quota, transport, authentication, setup, operator-action, and generic outcomes never trigger a power raise. Explicit operator pins pass through unchanged, and DDx never chooses or directs a harness, provider, or model. (2) **Arm B runs first** from the pinned rev in a script-created scratch worktree by submitting a bare Fizeau Execute request with those facts and `MinPower`; the script never invokes Claude/Codex/Gemini. Fizeau owns harness/provider/model choice, routing, streaming, any contract-defined continuation, and process lifetime and returns a public operation outcome. (3) Arm A runs after: `ddx try <id> --from <pinned-rev>` submits the equivalent request through the corrected FEAT-006 boundary and consumes the public immediate-error or final-event result; its rows are identified post-hoc by attempt ID plus the script's recorded time window. (4) **Equalized landed predicate for both arms**: the benchmark's shared repository-evidence evaluator checks that the diff applies cleanly to the pinned rev, the bead's named tests and core-invariant suite pass, and `lefthook run pre-commit` passes on the result tree. Arm A uses that evidence for DDx's bead verdict; the Fizeau-only arm records the comparable benchmark verdict without mutating the bead. Session-reported success alone does not count as landed. (5) Record landed-rate, wall-clock, cost, opaque `SessionLogPath`, request `MinPower`, and an `audit_ref` in `ab-baseline.jsonl`. Actual harness/provider/model metadata may exist only behind that linked per-invocation audit detail; it is excluded from the comparison table, grouping, filtering, grading, and policy. Publish the route-neutral weekly table in MET-003.
- Acceptance: MET-003 specifies base-rev and equal request-fact/power pinning, arm order, the equalized repository-evidence predicate, row segregation, and stratification before the first run; a boundary test proves neither arm invokes or parses a provider CLI or selects a harness/provider/model; two consecutive weekly reports with ≥ 18/20 beads attempted per arm.
- Validation: `ddx jq . .ddx/metrics/ab-baseline.jsonl | wc -l` ≥ 80 after week two; spot-audit two beads per week end-to-end.
- Non-scope: optimizing either arm; single-week conclusions.
- Dependencies: WB-2 (clean host); corrected CONTRACT-003 Fizeau contract and FEAT-006 consumer conformance.

## Validation

- Every WB names tests or measurable commands; all new tests enter the default suite.
- Phase gate: the five revised exit criteria, evidenced in a dated note appended here. Durable evidence (inventories, audits, weekly tables) lives under `docs/helix/06-iterate/`, not `.ddx/executions/` (per-machine exhaust, per CLAUDE.md).
- `lefthook run pre-commit` green on every commit; full suite green via the WB-6 async lane.

## Risks And Rollback

- **WB-2 deletes something live** — the revised host-wide quiescence procedure, the live-heartbeat exclusion rule, and execution-time re-inventory are the mitigations; `/tmp` deletions are reboot-ephemeral by definition; worktree unlock→prune is non-destructive to data (registrations point at nonexistent paths).
- **WB-6 scoped gates admit a real cross-package regression** — accepted; the async full suite + auto-filed regression bead bounds exposure to minutes. The attribution step (WB-6.1) prevents fixing the wrong locus — review showed the original plan had already mislocated two of four steps.
- **WB-7(c) batching loses audit rows on crash** — flush on shutdown; bounded telemetry loss only, never tracker state.
- **A/B fairness** — both arms use equal request facts, the same abstract `MinPower` floor, and one immutable comparison-wide operator-passthrough envelope through the public Fizeau operation contract, while the equalized repository-evidence predicate and pinned base rev remove arm-order and predicate asymmetry. Fizeau alone chooses the concrete runtime; the envelope is identical across arms and excluded from comparison policy. Residual DDx-only review overhead is recorded in MET-003; no arm may bypass Fizeau by invoking a provider CLI.
- **Freeze fatigue** — objective thaw condition (ratchet), not an open-ended freeze.

## Open Questions

1. WB-6 core-invariant suite contents: proposal is `internal/bead` conformance + lifecycle tests, the WB-3/4/5 tripwires, and the FEAT-022 evidence-cap lint — operator to confirm before the AC-template change lands.
2. WB-8 request comparability: use identical request facts, the same abstract `MinPower` floor, and one immutable operator-passthrough envelope for both arms. Actual harness/provider/model metadata is confined to linked per-invocation audit detail and excluded from comparison display, grouping, filtering, grading, and policy. DDx never creates a concrete routing choice.

## Handoff

Each WB is sized and anchored to be cut into beads per the P7 rubric, executed under operator supervision. Phase 1 (`phase1-lower-the-altitude-plan-2026-07-13.md`) activates when the exit-criteria note is appended here; Phase 2 may start any time after WB-1.

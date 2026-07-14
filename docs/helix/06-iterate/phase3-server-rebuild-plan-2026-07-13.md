---
ddx:
  id: IP-2026-07-13-phase3-server-rebuild
  type: implementation-plan
  status: reviewed
  depends_on:
    - AR-2026-07-13-vision-vs-reality
    - IP-2026-07-13-phase0-stop-self-harm
    - IP-2026-07-13-phase1-lower-altitude
    - IP-2026-07-13-phase2-doc-truth
    - FEAT-006
    - FEAT-002
    - FEAT-010
    - FEAT-020
    - ADR-022
---
# Phase 3 Plan: Server — Demote, Substrate, Rebuild

**Date:** 2026-07-13
**Status:** Revised r4 — corrected Fizeau runtime boundary applied 2026-07-13
**Source:** AR-2026-07-13-vision-vs-reality.md §7 Phase 3; §3.5 (server gap analysis)
**Mode:** WB-1 (demotion) may begin as soon as Phase 0 completes; WB-2/WB-3 activate only after Phase 1's exit criteria hold. Demoted mode is a fully supported resting state — if Phase 1 slips, the server simply stays an observer.

## Goal

Turn ddx-server from a component that *appears* to manage DDx work execution (worker truth in four weakly-consistent channels, silent parking) into one that observably does: DDx-worker state derives from one durable substrate, restarts lose no orchestration state, and the UI's picture matches the DDx worker process table. Fizeau remains the sole owner of Claude/Codex/Gemini sessions. Sequence: demote → build the substrate → rebuild DDx-worker management on it.

## Scope

In scope: demotion of managed DDx-worker **spawning** (repair stays on — see WB-1), the `.ddx/runs` orchestration-record substrate with atomic-write discipline, ADR-022 rev 6 completion (narrowed to its verified gaps), single-channel DDx-worker truth with an explicit partition for reported (manual/remote) DDx workers, and real-server e2e for the dispatch chain including its CI plumbing. A managed worker may submit an execution request to Fizeau and consume its public operation result under the pinned contract; neither the worker nor ddx-server owns the provider process or parses provider output.

Non-goals: federation write path / ADR-029 lease, FEAT-029 managed nodes, multi-node dashboard, evaluation UX (all Deferred per AR §7); UI feature work beyond truthful state display; MCP surface expansion. Doc ownership: the FEAT-002 "Known limitations" note lands inside Phase 2's FEAT-002 re-stamp commit (review: avoids a two-plan edit collision).

### Entry criteria

- WB-1: Phase 0 exit note appended.
- WB-2/WB-3: Phase 1 exit note appended (typed Fizeau results, worktree owner, claim-liveness fixes are this phase's substrate), the corrected CONTRACT-003 revision is recorded, and FEAT-006 consumer conformance proves DDx has no direct provider invocation, parsing, routing, resume, or supervision path.

### Exit criteria (revised — measurable rules, not point estimates)

1. **Managed-drain parity, as a predeclared sequential decision rule:** pair
   managed and supervised-CLI attempts on bead, base revision, task facts,
   initial `MinPower`, and one immutable comparison-wide operator-passthrough
   envelope identical across arms and excluded from comparison policy; timeouts
   count as failures and are never dropped. Before
   the first outcome, MET-003 fixes an empirical-Bernstein confidence sequence
   for the bounded paired difference (`-1`, `0`, `1`) at alpha `0.05`. After at
   least 100 complete pairs, the lower bound must exceed `-0.10`. Weekly looks
   accumulate into that one sequence rather than recomputing ordinary 95% CIs.
   Tightening to a 5-point bound requires at least 400 pairs and is optional.
2. **Restart-loss test green:** kill the server mid-drain with 2 managed DDx
   workers; on restart, worker state, claims, DDx correlation plus opaque
   Fizeau artifact/result fields,
   and run records reconcile — zero orphaned DDx worker processes and zero
   wedged `in_progress` beads. An execution without a durable Fizeau result is
   recorded as interrupted after DDx worker-death proof; Fizeau's caller-death
   contract guarantees its provider tree is gone. The server does not invent a
   post-restart session query/cancel API and never adopts, signals, or reaps a
   provider process (named chaos test, WB-3).
3. **Worker-truth parity, as a named test** (review: "any sampled moment" was
   unmeasurable, and `ddx bead status`'s Active-workers count derives from the
   bead store — a different surface than the server registry): during a fixture
   drain with 2 managed DDx workers, wait 2 seconds after both registrations are
   acknowledged, then poll once per second for at least 30 samples and through
   5 seconds after terminal drain state. Every sample must satisfy GraphQL
   DDx-worker count == DDx-worker-PID process-table count == bead-store active
   count. Provider processes and Fizeau sessions are explicitly outside this
   count. Zero mismatched samples.
4. At least one Playwright spec drives UI → GraphQL → managed DDx worker → fake/fixture Fizeau public operation result → bead store against the real fixture-booted server (no GraphQL mocks or provider CLI) and passes in its CI lane — **including the CI plumbing WB-3 step 6 builds** (the current frontend-e2e job runs mocked specs only and never boots the Go server).
5. FEAT-002/FEAT-020/ADR-022 stamps pass the Phase 2 spechonesty lint against the rebuilt reality.

## Assumptions

- Phase 1 delivered the corrected Fizeau public-result consumer, the worktree
  lifecycle owner, and synced claim liveness.
- Fizeau exclusively owns provider selection/routing, stream parsing,
  continuation, cancellation, and process lifetime. ddx-server manages only DDx
  orchestration workers and consumes public Fizeau execution results and opaque
  `SessionLogPath` references; it has no independent session-state query model.
- DDx workers submit task facts and set `MinPower`. They may raise the floor
  only for stronger-review intent or a distinct new attempt after
  capability-sensitive DDx evidence; route, transport, quota, authentication,
  setup, operator-action, and generic failures never raise power. Operator
  `MaxPower`, harness/provider/model pins, and public Fizeau `Policy` pass
  through unchanged. Current v0.14.50 has no per-request `Profile`; legacy
  profile settings are not translated. DDx never chooses or directs a concrete harness,
  provider, or model.
- The SvelteKit frontend is worth keeping; only its data sources change.
- tsnet auth, read surfaces, and operator-prompt intake stay serving throughout.
- ADR-022 rev 6 (2026-07-11) is directionally right; its **verified** remaining gaps are the journal file path + on-disk schema and the named integration tests (review: the reconcile states `applied`/`already_applied`/`conflict` are already enumerated at ADR:394-396, and the three-state freshness model is already implemented server-side at `worker_ingest.go:368-380` — the completion work is narrower than first planned).

## Work Breakdown

### WB-1: Demote to observer + intake (spawn-side only — revised per review)

- Objective: while the execution layer is rebuilt, the server observes, browses, and accepts intake; it spawns no DDx workers — **but it keeps repairing DDx orchestration state**, because disabling repair would wedge crashed workers' claims for the whole observer period. Provider-session lifecycle remains Fizeau's responsibility.
- Files or systems: `cli/internal/server/workers_supervisor.go` (scale-up path `:298`), watchdog restart in `workers.go`; **`cli/cmd/worker.go:99-113`** (review: `ddx worker set/enable` writes `desired.json` directly, outside the GraphQL mutations — a spawn trigger the original plan missed); dirty-root parking (`workers.go:704`, `workers_supervisor.go:531,663`); `ReconcileStaleWorkers`/`Prune`/`UnjamStaleClaims` (`workers.go:2506-2618` — errors currently discarded; these **stay active**).
- Steps: (1) feature-flag `server.manage_workers` (default off) gating **DDx-worker spawn-side only**: `StartExecuteLoop` refuses, supervisor scale-up refuses, watchdog *restart* refuses. Prune, stale-claim reconciliation, and cleanup of the DDx worker PID remain active in observer mode, with discarded errors propagated. A referenced Fizeau execution with no durable result is recorded as interrupted only after DDx worker-death proof and the pinned Fizeau caller-death/process-tree conformance guarantee; DDx never queries an API the contract does not expose or process-kills the session. (2) The flag flip **zeroes every registered project's `desired.json`** and `ddx worker set/enable` respects the flag — re-enablement in WB-3 requires explicitly re-written desired state, so no forgotten `desired_count>=1` can respawn on flag re-enable. (3) `startWorker`/`stopWorker` mutations return typed `management_disabled` for start; stop keeps working for the DDx worker and cancels the live context owned by that worker before termination. After restart there is no live DDx caller context to cancel. (4) Replace silent parking with a loud state: any would-park condition becomes a typed top-of-dashboard state + `ddx doctor` finding with the blocking reason. (5) Verify no managed DDx workers are running (Phase 0 entry already stopped them; assert `desired.json` count is 0 everywhere). (6) Document the interim executor (cron/systemd `ddx work --once`) and its Fizeau dependency — the text lands via Phase 2's FEAT-002 commit.
- Acceptance (split by layer per review): Go: `TestServerManagementDisabledSpawnsNothing` (flag off + stale `desired_count=1` on disk + supervisor tick + watchdog deadline + `ddx worker enable` attempt → zero DDx-worker spawns; repair of a killed fixture worker records an interrupted Fizeau execution without a session query, releases or parks the claim, and emits `bead.stopped`); Go: `TestRestartRecoveryDoesNotInventFizeauSessionAPI`; Go: `TestServerNeverSignalsProviderProcess`; Go: `TestDirtyRootParkingSurfacesTypedState` (typed state + doctor finding — not a rendered banner, which a Go test cannot assert); frontend unit test named for the banner rendering from that typed state; intake round-trip asserted by the existing operator-prompt integration test (named in the bead at cut time).
- Validation: `cd cli && go test -run 'ServerManagementDisabled|RestartRecovery|DirtyRootParking' ./internal/server/...`; 48 h observer-mode soak with zero spawned DDx worker processes (`pgrep` audit, both mount namespaces — the Phase 0 lesson). Fizeau execution evidence comes from its public result and DDx's durable record, not provider-process inspection or an assumed session query.
- Non-scope: deleting DDx-worker supervisor/watchdog code; reintroducing provider-session supervision or provider-process-tree machinery in DDx; UI redesign.
- Dependencies: Phase 0 exit. The demotion may begin immediately; session-reconciliation code and its acceptance test additionally wait for the corrected CONTRACT-003 and FEAT-006 consumer conformance.

### WB-2: The run substrate — `.ddx/runs/` with atomic-write discipline

- Objective: the never-built foundation (FEAT-010/SD-025/TD-010): one durable,
  typed DDx orchestration record per attempt, crash-safe and queryable,
  containing DDx correlation, the opaque Fizeau `SessionLogPath`, and public
  immediate/final result fields but no scraped provider stream — the single
  source WB-3 derives DDx-worker truth from.
- Files or systems: new writer in `cli/internal/agent` — `.ddx/runs/<run-id>/record.json`; minimal atomic-write helper (temp + fsync file + rename + fsync dir, manifest-last; the useful core of Deferred FEAT-028, package-private); readers: `ddx runs list|show` CLI verbs; GraphQL runs resolvers re-pointed at the real substrate. **Legacy path reality (review):** the code reads `.ddx/exec/runs` (`state_runs.go:16`), FEAT-010 says `.ddx/exec-runs`, and the bulk data is `.ddx/executions` (3,234 dirs) — the design note must enumerate all three and explicitly retire the session-synthesis path (`synthesizeRunsFromSessions`) as a source.
- Steps: (1) design note appended to TD-010 (operator-reviewed): versioned
  record schema (layer discriminator, parent/child links, lifecycle state
  `dispatching|running|terminal|interrupted`, DDx correlation, optional opaque
  Fizeau `SessionLogPath`, public immediate/final result fields, DDx
  repository-evidence verdict, timestamps, cost, evidence
  pointers), the three legacy roots, and the read-path strategy below; raw
  provider output and provider PID/process metadata are forbidden fields. (2)
  Atomic writer + property test (kill -9 during write never yields a torn or
  half-visible record). (3) Execute loop publishes the initial `dispatching`
  record before calling Fizeau, transitions it to `running` when the public
  execution stream exists, and atomically transitions the same record to
  `terminal` after a typed immediate error or public final event and repository evaluation.
  Recovery transitions a non-terminal record to `interrupted` only with DDx
  worker-death proof and the pinned Fizeau caller-death guarantee. (4) **Read
  path with a latency budget** (review: `loadRunsForProject` already full-scans
  all legacy dirs per request with no cache — layering migrate-on-read there is
  either a write burst in a request handler or a permanent re-parse tax, and
  GraphQL perf breaches were a gate-breaking failure class): a one-time
  background sweep migrates legacy dirs to records with the shim as fallback for
  stragglers, plus an mtime-keyed reader cache; acceptance bound: runs list query
  < 500 ms against the live 3.2k-dir corpus, asserted in the perf lane. (5) CLI
  verbs + GraphQL reads over one reader package; `ddx bead show` links its
  attempts' records. **`ddx runs children` is deferred** (review: nothing
  produces layer-1 child records yet — one record per attempt is the v1 grain;
  the children verb ships when a layer-1 publish point exists, or never).
- Acceptance: `TestRunRecordExistsBeforeFizeauDispatch`,
  `TestRunRecordAtomicPublishSurvivesKill`,
  `TestInterruptedRunRecordRecoveredWithoutSessionQuery`, and
  `TestOneRecordPerAttempt` (in a fresh isolated project fixture with no legacy
  corpus, drain N → exactly N records, statuses match `attempts.jsonl`);
  `TestLegacyRunMigrationPreservesNewAttemptDelta` (seed legacy directories,
  record the migrated baseline, run N new attempts, and assert the run-count
  delta and new run IDs are exactly N); runs list latency bound green in the
  perf lane; legacy dirs readable through the migrated store; session-synthesis
  retired from the runs read path.
- Validation: `cd cli && go test -run 'RunRecord|LegacyRunMigration' ./internal/...`; in an isolated fixture, capture `ddx runs list --json` before and after a 20-attempt drain and assert the after-minus-before count plus the set of newly observed run IDs is exactly 20; run the perf-lane assertion separately against the migrated live-size corpus.
- Non-scope: general-purpose BlobStore (FEAT-028 stays Deferred); rewriting `.ddx/executions` history; retention changes (existing `executions.retain_days` applies).
- Dependencies: Phase 1 exit; corrected CONTRACT-003 and FEAT-006 consumer conformance. Blocks WB-3.

### WB-3: Rebuild managed workers on one channel of truth + finish ADR-022 rev 6 (attribution and partition corrected per review)

- Objective: re-enable management where **managed DDx-worker truth = journal + run substrate** and restarts reconcile DDx orchestration from durable Fizeau results or an explicit interrupted-session record; prove it end-to-end with real-server tests.
- Files or systems: ADR-022 rev 6 completion **narrowed to verified gaps**: the offline journal's file path + on-disk schema with idempotency keys, and the integration tests — which are **TP-021's named tests, to be imported into ADR-022's test section during step 1** (review: the original plan mis-attributed them to ADR-022; ADR-022's own named tests at ADR:625-628 are the register/event happy-path set, already a different tier). Worker-side probe gap: **a reconciling state between register and online coordination** — `probe.go:155-159` two-value enum gains it; `markConnected` (`:325`) must not permit online mutations until reconcile acks (ADR:384-386). The server-side three-state freshness at `worker_ingest.go:368-380` already exists and is retained. Deletions/moves: the restart-lost in-memory ingest registry; `/tmp` heartbeat sidecars (worker heartbeats follow Phase 1's project-scoped root); `ps`-scrape reconciliation demoted to a `ddx doctor` diagnostic; `List()`'s four-channel merge (`workers.go:1334`); recovery-write error discards (`workers.go:2506-2618`) propagated.
- Steps: (1) complete the ADR to implementable depth (operator-reviewed before code), explicitly separating DDx-worker lifecycle from Fizeau session lifecycle. (2) **Worker-truth partition** (review: "one channel" restated four channels — journal, substrate, heartbeats, ingest log — and manual/remote DDx workers genuinely need ingest since they share no filesystem root): *managed DDx-worker* truth = journal + run substrate only, heartbeat files as liveness input; the ingest path feeds a **clearly-labeled `reported` DDx-worker class** (manual/other-machine workers), which `List()` returns as a distinct class and never merges into managed state; "replayable ingest log" means rebuild of the reported DDx-worker view only. Fizeau session state is referenced, never merged into worker identity. (3) Restart replays the journal + scans the substrate; orphaned DDx worker processes may be adopted or terminated by DDx. A Fizeau execution with a durable result is reconciled from that result; one without a result is marked interrupted after worker-death proof and the pinned caller-death cleanup guarantee. DDx does not query/cancel an absent session API and never adopts, signals, or reaps Claude/Codex/Gemini processes. (4) Re-enable `server.manage_workers` gated on the three TP-021 integration tests passing (`TestIntegration_WorkersCoordinateThroughReachableServer` — which includes manual `ddx try`/`ddx work` participants via the reported class, `TestIntegration_ManagedWorkerDiesWithServer`, `TestIntegration_ManualWorkerContinuesOfflineAndReconciles`); each test uses a fake/fixture Fizeau service returning public execution results, never a provider CLI. Re-enablement requires explicitly re-written desired state (WB-1 zeroed it). (5) Real-server Playwright: boot the fixture server, start DDx worker → submit one fixture session to Fizeau → evaluate repository evidence → observe run record + bead close in the UI, no `page.route` mocks or provider CLI. (6) **CI plumbing** (review: the frontend-e2e job never builds the Go binary): extend or add a CI lane that builds ddx, boots a `build-fixture-repo.sh` server and fixture Fizeau service, and runs the real-server-tagged spec; create the quarantine lane (mandatory triage, never deletion — the AR documents a deleted Playwright spec silently reopening a closed gap). (7) Exit-criterion-3 parity test; managed-drain arm added to weekly MET-003.
- Acceptance: the three TP-021 tests green through the Fizeau fixture; restart-loss chaos test (exit criterion 2); `TestWorkerListPartitionsManagedAndReported` (managed view reads journal+substrate only; reported view clearly classed; no cross-merge); `TestServerNeverSignalsProviderProcess`; worker-truth parity test (exit criterion 3); real-server Playwright in its CI lane.
- Validation: `cd cli && go test -race -run 'Integration_Worker|RestartLoss|WorkerListPartition' ./internal/server/...`; `bun run test:e2e` real-server tag; exit-criterion-1 decision rule computed from MET-003 managed-arm data.
- Non-scope: hub/spoke federation of worker control; worker UI features beyond truthful state.
- Dependencies: WB-2; ADR completion sign-off.

## Validation

- Exit criteria 1–5 map to named tests or stated decision rules; the restart-loss chaos test and the real-server Playwright lane are non-negotiable acceptance, permanent in CI.
- Weekly MET-003 gains the managed-drain arm (WB-3 step 7) so server overhead vs plain `ddx work` is measured; the exit-criterion-1 CI rule accumulates across weeks.
- Durable evidence (soak audits, parity runs, decision-rule computations) archives under `docs/helix/06-iterate/metrics/`, not `.ddx/executions/`.

## Risks And Rollback

- **Demotion breaks a workflow relying on managed workers** — interim recipe documented before the flip; rollback is a config change, and the loud-parking state means the demoted server cannot *silently* under-serve.
- **Stale desired-state respawn on re-enable** — eliminated by design: the flip zeroes `desired.json` everywhere and re-enable requires re-written desired state (review's zombie-state finding).
- **Run-record write amplification** — one atomic write per attempt terminal transition; measured in MET-003; buffer non-terminal events if needed, never the terminal record.
- **Migrate-on-read latency tax** — replaced by one-time background sweep + cache + a perf-lane latency assertion (review's state_runs.go full-scan finding).
- **Schema churn** — versioned from day one; the sweep tolerates stragglers via the shim.
- **Reported-vs-managed partition confuses the UI** — the DDx-worker partition is explicit in the API (distinct class), tested by name, and truer than today's four-channel merge; Fizeau sessions are displayed only as linked execution references, never as DDx workers.
- **Real-server Playwright flakes in CI** — quarantine lane with mandatory triage, never deletion.

## Open Questions

1. WB-2 design note home: TD-010 append (default — Phase 2 re-stamps it anyway) vs fresh TD.
2. In-process (goroutine) DDx workers: retained or subprocess-only after rebuild? Subprocess-only simplifies the journal to one DDx-worker liveness model — default: subprocess-only, decided in the ADR completion. This choice does not affect Fizeau's session/process model.
3. DDx-worker heartbeat transport: file under the project-scoped runtime root (default — preserves P9 network-free drain; server reads the root it owns) vs POST. Fizeau session liveness stays behind CONTRACT-003.
4. Exit-criterion-1 follow-up: after the 10-point rule passes, is the ~400-attempt 5-point tightening worth the spend, or does MET-003's ongoing trend suffice? Operator decides post-rebuild.

## Handoff

WB-1 cuts into 3–4 supervised beads immediately after Phase 0. WB-2/WB-3 beads are cut only after the corrected CONTRACT-003/FEAT-006 consumer conformance is green and their design notes (record schema; ADR-022 completion) pass operator review. Those notes must state that ddx-server manages only DDx orchestration workers and consumes public Fizeau operation results. Phase completion appends an exit note with links to the chaos-test runs, the parity data and decision-rule computation, the no-provider-process-control proof, and the re-stamped specs.

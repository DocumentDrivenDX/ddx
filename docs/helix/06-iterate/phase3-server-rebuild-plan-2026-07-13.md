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
    - FEAT-002
    - FEAT-010
    - FEAT-020
    - ADR-022
---
# Phase 3 Plan: Server — Demote, Substrate, Rebuild

**Date:** 2026-07-13
**Status:** Revised r3 — adversarial review plus execution-readiness corrections applied 2026-07-13
**Source:** AR-2026-07-13-vision-vs-reality.md §7 Phase 3; §3.5 (server gap analysis)
**Mode:** WB-1 (demotion) may begin as soon as Phase 0 completes; WB-2/WB-3 activate only after Phase 1's exit criteria hold. Demoted mode is a fully supported resting state — if Phase 1 slips, the server simply stays an observer.

## Goal

Turn ddx-server from a component that *appears* to manage work (worker truth in four weakly-consistent channels, silent parking) into one that observably does: managed-worker state derives from one durable substrate, restarts lose nothing, and the UI's picture matches the process table. Sequence: demote → build the substrate → rebuild management on it.

## Scope

In scope: demotion of managed-worker **spawning** (repair stays on — see WB-1), the `.ddx/runs` record substrate with atomic-write discipline, ADR-022 rev 6 completion (narrowed to its verified gaps), single-channel managed-worker truth with an explicit partition for reported (manual/remote) workers, real-server e2e for the dispatch chain including its CI plumbing.

Non-goals: federation write path / ADR-029 lease, FEAT-029 managed nodes, multi-node dashboard, evaluation UX (all Deferred per AR §7); UI feature work beyond truthful state display; MCP surface expansion. Doc ownership: the FEAT-002 "Known limitations" note lands inside Phase 2's FEAT-002 re-stamp commit (review: avoids a two-plan edit collision).

### Entry criteria

- WB-1: Phase 0 exit note appended.
- WB-2/WB-3: Phase 1 exit note appended (typed results, worktree owner, claim-liveness fixes are this phase's substrate).

### Exit criteria (revised — measurable rules, not point estimates)

1. **Managed-drain parity, as a decision rule with stated error rates** (review: a 5-point tolerance at n=100/arm is a coin flip — SE≈7.1pp): the 95% CI of the success-rate difference (managed drain vs supervised CLI drain) must exclude a deficit worse than 10 points at n≈100/arm; evidence may accumulate sequentially across weekly MET-003 managed-arm runs. Tightening to a 5-point bound requires ~400 attempts/arm and is optional follow-up, not the gate.
2. **Restart-loss test green:** kill the server mid-drain with 2 managed workers; on restart, worker state, claims, and run records reconcile — zero orphaned process trees, zero wedged `in_progress` beads (named chaos test, WB-3).
3. **Worker-truth parity, as a named test** (review: "any sampled moment" was unmeasurable, and `ddx bead status`'s Active-workers count derives from the bead store — a different surface than the server registry): during a fixture drain with 2 managed workers, poll every N seconds for the drain duration; every sample must satisfy GraphQL workers count == pgid-derived process-table count == bead-store active count, with the truth set defined as server-managed workers on the local host. Zero mismatched samples.
4. At least one Playwright spec drives UI → GraphQL → managed subprocess → bead store against the real fixture-booted server (no GraphQL mocks) and passes in its CI lane — **including the CI plumbing WB-3 step 6 builds** (the current frontend-e2e job runs mocked specs only and never boots the Go server).
5. FEAT-002/FEAT-020/ADR-022 stamps pass the Phase 2 spechonesty lint against the rebuilt reality.

## Assumptions

- Phase 1 delivered typed terminal results, the worktree lifecycle owner, and synced claim liveness.
- The SvelteKit frontend is worth keeping; only its data sources change.
- tsnet auth, read surfaces, and operator-prompt intake stay serving throughout.
- ADR-022 rev 6 (2026-07-11) is directionally right; its **verified** remaining gaps are the journal file path + on-disk schema and the named integration tests (review: the reconcile states `applied`/`already_applied`/`conflict` are already enumerated at ADR:394-396, and the three-state freshness model is already implemented server-side at `worker_ingest.go:368-380` — the completion work is narrower than first planned).

## Work Breakdown

### WB-1: Demote to observer + intake (spawn-side only — revised per review)

- Objective: while the execution layer is rebuilt, the server observes, browses, and accepts intake; it spawns nothing — **but it keeps repairing**, because disabling reap/repair would wedge crashed workers' claims for the whole observer period (the exact phantom-worker symptom this phase exists to kill).
- Files or systems: `cli/internal/server/workers_supervisor.go` (scale-up path `:298`), watchdog restart in `workers.go`; **`cli/cmd/worker.go:99-113`** (review: `ddx worker set/enable` writes `desired.json` directly, outside the GraphQL mutations — a spawn trigger the original plan missed); dirty-root parking (`workers.go:704`, `workers_supervisor.go:531,663`); `ReconcileStaleWorkers`/`Prune`/`UnjamStaleClaims` (`workers.go:2506-2618` — errors currently discarded; these **stay active**).
- Steps: (1) feature-flag `server.manage_workers` (default off) gating **spawn-side only**: `StartExecuteLoop` refuses, supervisor scale-up refuses, watchdog *restart* refuses; reap, prune, stale-claim release, and process-tree kill remain active in observer mode, with their discarded errors propagated (same fix class as Phase 0 WB-7b); (2) the flag flip **zeroes every registered project's `desired.json`** and `ddx worker set/enable` respects the flag — re-enablement in WB-3 requires explicitly re-written desired state, so no forgotten `desired_count>=1` can respawn on flag re-enable; (3) `startWorker`/`stopWorker` mutations return typed `management_disabled` for start; stop keeps working (it is repair); (4) replace silent parking with a loud state: any would-park condition becomes a typed top-of-dashboard state + `ddx doctor` finding with the blocking reason; (5) verify no managed workers are running (Phase 0 entry already stopped them; assert `desired.json` count is 0 everywhere); (6) document the interim executor (cron/systemd `ddx work --once`) — the text lands via Phase 2's FEAT-002 commit.
- Acceptance (split by layer per review): Go: `TestServerManagementDisabledSpawnsNothing` (flag off + stale `desired_count=1` on disk + supervisor tick + watchdog deadline + `ddx worker enable` attempt → zero spawns; reap of a killed fixture worker still releases its claim and emits `bead.stopped`); Go: `TestDirtyRootParkingSurfacesTypedState` (typed state + doctor finding — not a rendered banner, which a Go test cannot assert); frontend unit test named for the banner rendering from that typed state; intake round-trip asserted by the existing operator-prompt integration test (named in the bead at cut time).
- Validation: `cd cli && go test -run 'ServerManagementDisabled|DirtyRootParking' ./internal/server/...`; 48 h observer-mode soak with zero spawned processes (`pgrep` audit, both mount namespaces — the Phase 0 lesson).
- Non-scope: deleting supervisor/watchdog code (WB-3 reuses the process-tree machinery); UI redesign.
- Dependencies: Phase 0 exit.

### WB-2: The run substrate — `.ddx/runs/` with atomic-write discipline

- Objective: the never-built foundation (FEAT-010/SD-025/TD-010): one durable, typed record per attempt, crash-safe, queryable — the single source WB-3 derives worker truth from.
- Files or systems: new writer in `cli/internal/agent` — `.ddx/runs/<run-id>/record.json`; minimal atomic-write helper (temp + fsync file + rename + fsync dir, manifest-last; the useful core of Deferred FEAT-028, package-private); readers: `ddx runs list|show` CLI verbs; GraphQL runs resolvers re-pointed at the real substrate. **Legacy path reality (review):** the code reads `.ddx/exec/runs` (`state_runs.go:16`), FEAT-010 says `.ddx/exec-runs`, and the bulk data is `.ddx/executions` (3,234 dirs) — the design note must enumerate all three and explicitly retire the session-synthesis path (`synthesizeRunsFromSessions`) as a source.
- Steps: (1) design note appended to TD-010 (operator-reviewed): versioned record schema (layer discriminator, parent/child links, typed outcome from Phase 1, timestamps, cost, evidence pointers), the three legacy roots, and the read-path strategy below; (2) atomic writer + property test (kill -9 during write never yields a torn or half-visible record); (3) execute loop publishes exactly one record per attempt at terminal transition; (4) **read path with a latency budget** (review: `loadRunsForProject` already full-scans all legacy dirs per request with no cache — layering migrate-on-read there is either a write burst in a request handler or a permanent re-parse tax, and GraphQL perf breaches were a gate-breaking failure class): a one-time background sweep migrates legacy dirs to records with the shim as fallback for stragglers, plus an mtime-keyed reader cache; acceptance bound: runs list query < 500 ms against the live 3.2k-dir corpus, asserted in the perf lane; (5) CLI verbs + GraphQL reads over one reader package; `ddx bead show` links its attempts' records. **`ddx runs children` is deferred** (review: nothing produces layer-1 child records yet — one record per attempt is the v1 grain; the children verb ships when a layer-1 publish point exists, or never).
- Acceptance: `TestRunRecordAtomicPublishSurvivesKill`; `TestOneRecordPerAttempt` (in a fresh isolated project fixture with no legacy corpus, drain N → exactly N records, statuses match `attempts.jsonl`); `TestLegacyRunMigrationPreservesNewAttemptDelta` (seed legacy directories, record the migrated baseline, run N new attempts, and assert the run-count delta and new run IDs are exactly N); runs list latency bound green in the perf lane; legacy dirs readable through the migrated store; session-synthesis retired from the runs read path.
- Validation: `cd cli && go test -run 'RunRecord|LegacyRunMigration' ./internal/...`; in an isolated fixture, capture `ddx runs list --json` before and after a 20-attempt drain and assert the after-minus-before count plus the set of newly observed run IDs is exactly 20; run the perf-lane assertion separately against the migrated live-size corpus.
- Non-scope: general-purpose BlobStore (FEAT-028 stays Deferred); rewriting `.ddx/executions` history; retention changes (existing `executions.retain_days` applies).
- Dependencies: Phase 1 exit. Blocks WB-3.

### WB-3: Rebuild managed workers on one channel of truth + finish ADR-022 rev 6 (attribution and partition corrected per review)

- Objective: re-enable management where **managed-worker truth = journal + run substrate** and restarts reconcile; prove it end-to-end with real-server tests.
- Files or systems: ADR-022 rev 6 completion **narrowed to verified gaps**: the offline journal's file path + on-disk schema with idempotency keys, and the integration tests — which are **TP-021's named tests, to be imported into ADR-022's test section during step 1** (review: the original plan mis-attributed them to ADR-022; ADR-022's own named tests at ADR:625-628 are the register/event happy-path set, already a different tier). Worker-side probe gap: **a reconciling state between register and online coordination** — `probe.go:155-159` two-value enum gains it; `markConnected` (`:325`) must not permit online mutations until reconcile acks (ADR:384-386). The server-side three-state freshness at `worker_ingest.go:368-380` already exists and is retained. Deletions/moves: the restart-lost in-memory ingest registry; `/tmp` heartbeat sidecars (worker heartbeats follow Phase 1's project-scoped root); `ps`-scrape reconciliation demoted to a `ddx doctor` diagnostic; `List()`'s four-channel merge (`workers.go:1334`); recovery-write error discards (`workers.go:2506-2618`) propagated.
- Steps: (1) complete the ADR to implementable depth (operator-reviewed before code); (2) **worker-truth partition** (review: "one channel" restated four channels — journal, substrate, heartbeats, ingest log — and manual/remote workers genuinely need ingest since they share no filesystem root): *managed* worker truth = journal + run substrate only, heartbeat files as liveness input; the ingest path feeds a **clearly-labeled `reported` worker class** (manual/other-machine workers), which `List()` returns as a distinct class and never merges into managed state; "replayable ingest log" means rebuild of the reported view only; (3) restart replays journal + scans substrate; orphaned managed process trees adopted or killed via the existing process-group machinery, keyed by the journal; (4) re-enable `server.manage_workers` gated on the three TP-021 integration tests passing (`TestIntegration_WorkersCoordinateThroughReachableServer` — which includes manual `ddx try`/`ddx work` participants via the reported class, `TestIntegration_ManagedWorkerDiesWithServer`, `TestIntegration_ManualWorkerContinuesOfflineAndReconciles` — this phase owns it exclusively; review removed it from Phase 1 to break a cross-phase deadlock); re-enablement requires explicitly re-written desired state (WB-1 zeroed it); (5) real-server Playwright: boot the fixture server, start worker → drain one fixture bead → observe run record + bead close in the UI, no `page.route` mocks; (6) **CI plumbing** (review: the frontend-e2e job never builds the Go binary): extend or add a CI lane that builds ddx, boots a `build-fixture-repo.sh` server, runs the real-server-tagged spec; create the quarantine lane (mandatory triage, never deletion — the AR documents a deleted Playwright spec silently reopening a closed gap); (7) exit-criterion-3 parity test; managed-drain arm added to weekly MET-003.
- Acceptance: the three TP-021 tests green; restart-loss chaos test (exit criterion 2); `TestWorkerListPartitionsManagedAndReported` (managed view reads journal+substrate only; reported view clearly classed; no cross-merge); worker-truth parity test (exit criterion 3); real-server Playwright in its CI lane.
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
- **Reported-vs-managed partition confuses the UI** — the partition is explicit in the API (distinct class), tested by name, and truer than today's four-channel merge; the UI labels reported workers as such.
- **Real-server Playwright flakes in CI** — quarantine lane with mandatory triage, never deletion.

## Open Questions

1. WB-2 design note home: TD-010 append (default — Phase 2 re-stamps it anyway) vs fresh TD.
2. In-process (goroutine) workers: retained or subprocess-only after rebuild? Subprocess-only simplifies the journal to one liveness model — default: subprocess-only, decided in the ADR completion.
3. Heartbeat transport: file under the project-scoped runtime root (default — preserves P9 network-free drain; server reads the root it owns) vs POST.
4. Exit-criterion-1 follow-up: after the 10-point rule passes, is the ~400-attempt 5-point tightening worth the spend, or does MET-003's ongoing trend suffice? Operator decides post-rebuild.

## Handoff

WB-1 cuts into 3–4 supervised beads immediately after Phase 0. WB-2/WB-3 beads are cut only after their design notes (record schema; ADR-022 completion) pass operator review — the AR's core lesson is that this subsystem shipped code ahead of implementable contracts; this plan re-sequences that permanently. Phase completion appends an exit note with links to the chaos-test runs, the parity data and decision-rule computation, and the re-stamped specs.

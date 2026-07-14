---
ddx:
  id: AR-2026-07-13-vision-vs-reality
  status: reviewed
  depends_on:
    - helix.prd
    - FEAT-002
    - FEAT-004
    - FEAT-006
    - FEAT-010
    - FEAT-012
    - FEAT-022
    - ADR-022
    - ADR-024
    - ADR-026
    - SD-025
    - TP-021
---
# AR-2026-07-13: Vision vs Reality — Why DDx Is Foundering, and the Plan Back

**Date:** 2026-07-13
**Scope:** Full-stack alignment review: product vision → PRD → feature specs → designs → implementation → live operational evidence.
**Method:** 25 parallel investigations — doc-promise extraction and independent code assessment for six subsystems, spec-vs-code gap verification for all six, empirical mining of `.ddx/metrics/attempts.jsonl` / `.ddx/executions/` / `.ddx/attachments/` / git history / live host state, and a four-hypothesis diagnostic panel arguing scope vs specification vs implementation vs architectural premise. Headline claims were re-verified at file:line; one panelist executed the failing test on clean main to confirm it.

**Operator framing (verbatim, 2026-07-13):** *"90% of the declines are actually failures. The underlying agents can do the work just fine when given the task outside ddx. Inside ddx it refuses."* This review adopts that framing: an attempt only counts as success if work **landed**. The rationale evidence supports it — of 248 sampled `no_changes` rationales, nearly all cite conditions DDx created (ENOSPC 76–144 depending on counting method, lefthook/gate failures 106–138, sandbox restrictions 59–106, timeouts 37; counts overlap). A live specimen (`20260713T161454-3e24f7dd`) shows a fully capable agent producing a correct root-cause analysis and fix design, then unable to run `true` in a shell because DDx's own scratch churn had exhausted the sandbox's `/tmp` — the 7th identical failure across unrelated beads in one day. The model works; the environment DDx builds around it fails.

---

## 1. Executive Verdict

The vision is coherent and most specs are unusually concrete. The project is foundering anyway. The diagnostic panel's verdicts, each argued adversarially against the full evidence dossier:

| Hypothesis | Verdict | Core finding |
|---|---|---|
| **H-IMPL** — implementation execution is failing | **Primary** (high confidence) | Every dominant, quantified failure signature traces to a verified defect in DDx-owned code — much of it *violating specs that were adequate* |
| **H-PREMISE** — the architecture fights its substrate | **Primary** (medium-high) | Reliability-critical decisions all cross a text-only boundary; the compensation layer is itself the top failure generator |
| **H-SPEC** — key parts mis-specified | Contributing | Real defects (boundary contradiction, status rot, the #2 symptom has *no spec at all*) — but the dominant killers were spec **violations**, not spec holes |
| **H-SCOPE** — bit off too much | Contributing | Breadth is real, but effort data refutes dilution: the core got 3–5× the effort of breadth in every month and is still unreliable |

**The two primary causes interlock.** DDx supervises opaque harness CLIs from outside — so every decision (is the run healthy? did it finish? why did it fail?) is inferred from scraped text: 22 stderr kill-regexes that cancel a live run on any matching line, ~50-substring failure classification, un-locale-pinned English git stderr as a control plane, JSON verdicts scraped from reviewer stdout. Every novel failure lands outside the regexes; each incident was answered with another patch at the same altitude (more reapers, more watchdogs, more classifiers); and that compensation layer is now where the failures come from. The watchdog that deletes live `.git/index.lock` files, the cleanup that treats `os.TempDir()` as deletable, the claim leases leaked 27,000-fold into `/tmp` — all of it is machinery a direct sub-agent invocation never needs.

**The July collapse is the controlled experiment.** Attempt success was a stable ~48% plateau (May, June) and fell off a cliff to 18% in July — under unchanged specs, unchanged agents, unchanged volume, and *essentially zero breadth work* (2 breadth-scoped commits vs 19 core). Only the platform's own code churned. That is the signature of implementation regression at the wrong architectural altitude, not of scope dilution or spec drift.

## 2. The Numbers

All figures recomputed from on-disk evidence 2026-07-13. Per the operator framing, "success" = work landed; declines count as system failures unless model incapacity is shown.

| Signal | Value | Source |
|---|---|---|
| System failure rate (work did not land) | **62.7% of 1,444 attempts** (May 1–Jul 13); **82.0% in July** | `.ddx/metrics/attempts.jsonl` |
| Attempt success by month | May **47.6%** → Jun **48.0%** → Jul 1–13 **18.0%** (plateau, then cliff) | same |
| System-induced refusals (`no_changes`) | 23.2% of all attempts; **39.1% in July**; rationales overwhelmingly cite ENOSPC / gates / sandbox / timeout | rationale corpus (n=248) |
| Closed beads/month | Apr **1,073** → May **698** → Jun **417** → Jul pace **~310** | beads + archive JSONL |
| Open backlog composition | **76 of 105** open beads `kind:reliability`; ≥5 named `incident:niflheim-*` classes | `ddx bead list` |
| Routing preflight refusals | **7,823** "no viable routing candidate" = **96%** of all 8,181 disruption events | attachments event streams |
| Review subsystem error rate | **41%** (345 review-error / 502 review events); 308 with `output_bytes=0` | same |
| Attempt latency | median 10 min, p90 37 min; successful mean 23.4 min; 1.92 attempts/bead (worst: 28); July: ~5 attempts per landed bead | attempts.jsonl |
| Tracker perf vs spec | `bead status` **3.11 s** and `bead ready` 0.65–0.92 s at 348 beads, vs FEAT-004's "<100 ms at 10,000 beads" — missed 6–31× | measured live |
| Commit noise | 27,544 commits since 2026-05-01; **5.7%** are work commits; 13,265 are landing-checkpoint noise; 25 of the last 100 main commits are per-attempt tracker-audit commits | git log |
| Live host damage | `/tmp` at **100% inode exhaustion**; 406 leaked `ddx-home-*` dirs (~19 GB); **~27,091 leaked lease dirs** under `/tmp/ddx-claim-heartbeats`; 2 permanently stuck `locked initializing` worktrees; 4 GB exec-wt scratch | `df -i`, `git worktree list`, ls |
| Spend | $1,020 recorded; ≥25% on attempts that landed nothing (undercounted — codex rows record cost 0 at 5.4M+ input tokens) | attempts.jsonl |
| Evidence exhaust | 83% of 3,235 execution dirs (2.4 GB) contain no `result.json`; the specced unified `.ddx/runs/` substrate has **0 entries** | `.ddx/executions/` |

## 3. Gap Analysis (vision → PRD → specs → code)

All six subsystems below were verified by dedicated gap passes that re-checked claims against code at file:line.

### 3.1 Vision & PRD

The vision (v2.2.0) is internally coherent. The PRD detaches from reality in three ways: (a) it commits to a multi-team platform-company roadmap (~28 feature areas) while its own top risk is *"Framework requires its author to explain it"* (High/High) and several success criteria presuppose second parties that don't exist; (b) its success metrics are largely unmeasurable as written, and both real metric artifacts are broken — MET-001's budget (400 ms full-suite walltime) is off by ~3 orders of magnitude and **has never produced an observation** (`ddx metric validate MET-001` fails; the exec definition was never registered), while MET-002's ingestion job has **zero implementation** and depends on FEAT-016 (Status: Not Started) and a join key known-null since 2026-04-20; (c) five feature specs (FEAT-022/-023/-026/-028/-029) exist outside the PRD's own enumeration.

**Spec-status rot is systemic and bidirectional**, which matters more than any single defect: FEAT-002, FEAT-004, FEAT-009, FEAT-012 are stamped Complete atop unbuilt or contradicted contracts; FEAT-018 is stamped Not Started but is substantially implemented. The docs cannot be used to aim effort — they actively hide which gaps are real. And the single most symptom-relevant area — **worktree lifecycle under failure — has no owning spec anywhere in the doc set** (independently flagged by the doc reviewer, the gap verifier, and the platform investigator).

### 3.2 Execution engine

12 promises: 3 delivered, 7 partial, 2 diverged. The specs are concrete and implementation tracks them closely; the divergences are telling:

- FEAT-006 forbids in-band `ddx bead create` by workers; the shipped prompt *instructs* workers to do it (`execute_bead.go:2029,2114,2150`); `discovered_subtasks` appears nowhere in code.
- The unified `.ddx/runs/` substrate (FEAT-010/SD-025/TD-010) was never built — no writer, no `ddx runs` verbs — while the GraphQL layer grew a `runs` vocabulary over the missing substrate.

Structural findings:

- The work→harness chain is ~9 hops, and the seam where reliability ships is **excluded from CI by a static guard** (`no_live_harness_in_default_suite_test.go`). Runtime behavior there rests on text: 22 stderr kill-regexes (`executor.go:101-140,275-295` — a test log printing "429" aborts a healthy run), substring classification deciding retry/escalate/park (`provider_failure.go:85-192`, `execute_bead_status.go:124+`), stdout parsing with silent raw fallback (`runner.go:583-676`). The PATH-shim re-exec exists because fizeau never sets pdeathsig — production incident: 28 orphaned codex processes at ~90% CPU (ddx-01b89378).
- **Failure interpretation is quintuplicated** — five components independently re-classify the same attempt from concatenated free text; one transient failure can be simultaneously retried, power-escalated, cooled down, and parked.
- The pre-close pipeline is architecture-by-spec cost: intake hook (gating *stricter* than ADR-023's WARN-ONLY default — an implementation divergence toward more blocking), attempt, reflog-regex integrity parsing, doc gate, landing retries, then a default-on two-slot elevated-power review with repair cycles. 3+ LLM invocations and ~6 rejection points per close **mathematically guarantees** lower completion probability than one direct sub-agent call, before counting the 41% review transport error rate.
- Orchestrator contract confusion destroys correct work: 51 attempts rejected as `mixed_commit_and_no_changes_rationale`; ddx-232aa2c0 documents the orchestrator synthesizing commits and running full hooks *after* the agent wrote a valid retry rationale.
- The one fully honored invariant — FEAT-022 evidence caps, mechanically enforced end-to-end with CI lint — proves the team can land invariants when they are specified as mechanisms.

### 3.3 Worktree/git lifecycle

Core design (per-attempt isolated worktrees, land coordinator, ff-or-merge --no-ff, preserve-refs) is sound and largely delivered. Around it:

- **No single owner.** The no-orphan invariant is three overlapping best-effort reapers (git-English-string prune-retry at `attempt_backend.go:110-157`; a 1,570-line heuristic cleanup manager; liveness/max-age guessing in `execute_loop_shared.go:782-855`), with cleanup errors pervasively discarded (~449 `_ =` sites in `internal/agent`).
- **Locked worktrees are a vocabulary gap**: a crash during `git worktree add` (e.g. from `lifecycle_dispatch.go:81`, whose teardown is a defer with ignored errors) leaks a `locked initializing` registration that *no code path can remove* — "worktree unlock" appears in zero non-test files; `doctor --unjam` filters these out (`doctor_unjam.go:667-676`). Two such registrations exist in this repo now.
- **The hygiene machinery causes hygiene failures**: `execution_cleanup.go:829-845` treats `os.TempDir()` as an owned scratch root (the verified ENOSPC root cause — and a violation of SD-025's explicit "must not delete paths outside configured DDx roots"); the lockmetrics watchdog `os.RemoveAll`s the **live** `.git/index.lock` / tracker lock after its cap while the holder continues (`lockcap.go:106-158`), after which the holder's deferred remove deletes the *new* owner's lock — and the durable-audit critical section runs 4–6 git subprocesses each with a 30 s timeout under that same 30 s cap, so the race is routine, not exotic.
- **Landing is dangerous by construction**: hand-rolled checkout emulation mutating the operator's live branch (`execute_bead_land.go:387-526`) carries an in-code comment documenting a real operator-edit clobber incident (2026-07-06, eitri); blob forensics run inside the global lock (`:1532-1541 → :584-609`); one mkdir lock serializes every worker's landing/tracker-commit/checkpoint, and lock-budget exhaustion is surfaced as ordinary `execution_failed` by design (`tracker_lock.go:112-115`).
- FEAT-012 is stamped Complete while contradicting Accepted ADR-026 (tracked vs never-committed evidence — code deliberately never commits), containing a never-built epic contract (`ddx/epics`: zero hits; epics diverged to container/auto-decompose), and a violated preserve-ref uniqueness AC (`orchestrator.go:26-28`, second-granular). Two TDs share the TD-040 id; the one aimed at the live leak class is an AC-less Draft. TP-021's two most operationally telling required tests were never written.

### 3.4 Bead tracker

15 promises: 5 delivered, 8 partial, 2 diverged. The functional core is genuinely strong (enforced 6-state lifecycle, atomic JSONL with repair, 387 tests incl. chaos/property/fuzz). The operational layer is where it hurts:

- **Claim liveness diverged into structural double-execution**: US-025's `claimed-at`/`claimed-pid` on the tracked row was replaced by lease sidecars in `os.TempDir()/ddx-claim-heartbeats` (`claim_liveness.go:51-58`) with same-machine-PID-only staleness (`:287-302`) — on a *git-synced* tracker, another machine's active claim looks stale and is reclaimable. ~27k lease scope dirs have leaked because cleanup never removes them (`:147-183`).
- **Perf NFR missed 6–31×** via designed-in sleeps on the normal path (`claim_liveness.go:202-212`: 1 ms×3 per unclaimed bead), 4 full 2.2 MB reparse-and-classify passes per `Status()`, and git shell-outs per pass — paid on every drain iteration.
- **Silent closure-gate rejection** (verified): `store.go:2058-2072` returns success when `ClosureGate` rejects; the worker counts it as success (`execute_bead_loop.go:4247-4257`) and the still-open bead re-enters the queue.
- **TD-027's boundary refactor was abandoned mid-flight but documented as landed** — `internal/storage` is 237 lines of inverted alias stubs re-exporting the parent; `store.go` remains a 3,340-line god file; the named invariant tests exist nowhere.
- **Three parallel lock implementations share a duplicated TOCTOU stale-break** (`lock.go:89-110` — two waiters can both break, and the slower `RemoveAll` deletes the winner's fresh lock). Cross-process contention — exactly what parallel workers create — sits entirely outside the otherwise-strong in-process test suite.

### 3.5 Server & web UI

15 promises: 7 delivered, 6 partial, 1 missing, 1 diverged. The server can dispatch work at the API level and its worker process-tree tests are among the best in the repo. But:

- **The coordination contract the server needs did not exist as an implementable spec until two days ago**: ADR-022 rev 6 (offline journal, idempotent reconciliation, server-serialized claims) is dated **2026-07-11**; `workerprobe/probe.go` still self-describes as rev-5 "observability only"; FEAT-002 is stamped Complete on top. Worker/server views of who-holds-what structurally drift.
- **Stale-claim release is broken on clean main**: `TestWorkerManagerStopStaleDiskEntry` fails (run and confirmed, 0.093 s) — stopping a stale disk-only worker leaves the bead `in_progress` with no `bead.stopped` event — and every recovery write in `ReconcileStaleWorkers`/`Prune`/`UnjamStaleClaims` discards errors (`workers.go:2506-2618`). Beads wedge; worktrees pinned to dead attempts accumulate.
- **Worker truth spans four weakly-consistent channels** (in-memory handles, `record.json`, restart-lost ingest registry, `/tmp` heartbeat sidecars) reconciled by PID/pgid/`ps`-scrape heuristics; the server currently reports 0 active workers while 2 managed workers run.
- **Parts of the surface are a facade**: 15 GraphQL resolvers `panic("not implemented")` in the published schema; `workerDispatch` kinds `realign-specs`/`run-checks` and the whole `comparisonDispatch` mutation return fake "queued" results **nothing ever executes** — the UI can honestly display work that will never happen. FEAT-029 (managed nodes) is entirely missing.
- **Zero real verification of the dispatch chain**: all five operator-workbench Playwright gates mock `/graphql` (one spec predates the feature it gates); no automated test exercises UI → server → managed subprocess → bead store. A worker exiting on a dirty project root is parked (`operator_attention`) until the root is clean — the verified mechanism for "the server silently stops doing work."

### 3.6 Metrics / evaluation

13 promises: 5 delivered, 6 partial, 1 diverged, 1 missing. The measure-half is built; the decide-half never was:

- **The feedback loop was never closed**: `QuotaSignal` is declared (`types.go:309`) but never constructed; zero quota references in `profile_select.go`; no routing or escalation decision reads any recorded history. Dispatch runs on static labels plus substring matching (`escalation/infrastructure.go:22-78`) whose patterns ("no such file or directory", "429") routinely appear inside genuine test output — misrouting real failures into same-tier retry.
- **The metrics layer taxes and corrupts the hot path it observes**: per-attempt durable-audit commits of tracked files under the shared tracker lock (`durable_audit.go:32-60`; 25 of the last 100 main commits), and the lock-cap watchdog described in §3.3 — built to observe hygiene, provably a cause of it.
- SD-023's comparison engine was silently downgraded to prose skills (its flags declared "unsupported" in `run_dispatch_spec.go:45-65`) while FEAT-019's UI doesn't exist — docs never updated.

### 3.7 Platform breadth

14 promises: 4 delivered, 7 partial, 1 diverged, 2 missing. Mostly faithful, often well-engineered (federation's read path may be the best code in scope), and mostly off the core loop. What matters:

- **FEAT-028 BlobStore is missing in full** (`cli/internal/blob/` does not exist) — execution evidence and attachments get no atomic-write/fsync/manifest-last discipline, so a killed attempt can leave partial evidence that downstream review/merge misreads.
- **ADR-029's pre-claim lease has zero implementation** — the coordination half of federation exists only as routes and specs.
- FEAT-009 is Complete-stamped with its multi-registry core never built (hardcoded 2-package list) and state written to a forbidden `~/.ddx` path; FEAT-001's performance ratchets are decorative (benchmarks assert nothing — "slow" has no regression gate anywhere).
- A repo-wide "deadcode reachability anchor" pattern pins dead code alive to defeat the project's own reachability check; the skill tree is tracked in triplicate; two parallel install implementations coexist.

## 4. Answering the Three Questions

**Did we bite off too much?** Yes — but not in the way it feels. The effort ledger refutes "breadth starved the core": core-loop areas got ~993 closed beads vs ~200 for breadth, out-worked breadth 3–5× every month, and July's collapse happened during essentially pure core focus. Breadth's real damage was indirect: (a) **gate coupling** — the monorepo-wide green-suite closure gate lets any breadth regression (e.g. the GraphQL perf budget breach) convert unrelated finished core work into `no_changes`; (b) **doc-truth dilution** — a solo maintainer cannot keep 26 spec surfaces honest, so status rot hid which core gaps were real; (c) **review bandwidth** — the binding constraint the vision itself names. Cut breadth to fix those three transmissions, not to reallocate keystrokes.

**Did we mis-specify something key?** Yes, three things — while noting the panel's finding that the *dominant* killers were spec violations, not spec holes. (1) The execution-boundary contradiction: "opaque passthrough consumer" (FEAT-006) plus total supervision duty (FEAT-010/ADR-023/-024) is only implementable as text-scraping. (2) **Worktree lifecycle under failure has no spec at all** — the #2 operator symptom is a spec hole filled by three ad-hoc reapers. (3) ADR-024's mandatory two-slot adversarial review with no risk-proportional path chose the latency and false-rejection profile operators now feel. Beneath these, the specification *system* is broken: Complete stamps on unbuilt contracts, an implementable server-coordination spec that appeared two days ago, hard parts deferred while trivia is frozen.

**Are we failing at implementation?** Yes — primary cause, highest confidence. The failure signature is uniform: adequate specs violated at load-bearing points (SD-025's cleanup scope, ADR-023's WARN-ONLY default, US-025's claim rows, TD-027's boundary — implemented inverted); soft invariants everywhere (best-effort cleanup, ~449 discarded errors, fire-and-forget cascades); text-scraped control flow at every seam; and verification that systematically stops where the symptoms live (the harness boundary CI-banned, dispatch e2e mocked, cross-process locking untested, 8/19 TP-021 named tests missing, a red reliability test on clean main). The meta-defect that keeps regenerating all of it: **bead closure is never verified against the defect class staying dead.** Cleanup requirements have existed for months across P8/FEAT-010/SD-025 and multiple closed beads — `/tmp` sits at 100% inodes today.

## 5. Root-Cause Synthesis

Two primary causes interlock and compound; two contributing causes amplify:

```
                 ┌─ H-PREMISE (primary) ─────────────────────┐
                 │ Supervise opaque CLIs from outside         │
                 │ → every decision inferred from text        │
                 │ → novel failures land outside the guards   │
                 └──────────────┬────────────────────────────┘
                                │ each incident answered by
                                ▼
                 ┌─ H-IMPL (primary) ────────────────────────┐
                 │ Patch accretion, no invariant ownership:   │
                 │ 5 classifiers, 3 reapers, watchdogs that   │
                 │ corrupt, cleanup that leaks, gates that    │
                 │ jam, errors discarded, seams untested      │
                 └──────────────┬────────────────────────────┘
        H-SCOPE (contrib):      │        H-SPEC (contrib):
        gate coupling, doc-     │        status rot hides gaps;
        truth dilution, review  │        worktree lifecycle
        bandwidth               │        never specced
                                ▼
          48% plateau → 18% cliff; 82% July failure rate;
          72% of backlog = self-repair; throughput −61%/quarter
```

The plateau-then-cliff shape carries the actionable message: the system held ~48% while the operator's patch rate kept up with the regenerating failure surface, then July's self-inflicted incidents (cleanup-io, provider-test-cpu) outran it. **~3 concrete fixes (scratch off `/tmp`, routing fail-open, gate scoping) plausibly restore the ~48% plateau quickly — but the plateau itself is the altitude problem.** A pipeline with ~6 rejection points and 3+ LLM invocations per close cannot beat a direct sub-agent's completion rate even implemented perfectly. Recovery has two stages: stop the self-harm, then lower the altitude.

## 6. What to Keep

- **Bead tracker core** — enforced state machine, atomic JSONL, chaos/fuzz coverage. Fix the operational seams; keep the design.
- **Worktree-in / evidence-out / queue-drain as DDx's ownership** — the vision's layer boundary is right; unattended drain, isolation, landing, and durable evidence are things the substrate genuinely does not provide. What must go is DDx-owned *supervision-by-scraping*.
- **Preserve-refs, land coordinator, no-ff landing** — sound design needing an owner, locale pinning, and an end to live-branch mutation.
- **FEAT-022 evidence caps** — the template: invariant = mechanism + CI lint, never prose.
- **Worker process-tree lifecycle tests and the federation chaos suite** — the best tests in the repo; extend that standard to the seams that are currently mocked.

## 7. The Plan (final)

> **Refinement note (2026-07-13):** each phase below has been refined into a standalone, adversarially-reviewed plan document — see `../phase0-stop-the-self-harm-plan-2026-07-13.md`, `../phase1-lower-the-altitude-plan-2026-07-13.md`, `../phase2-doc-truth-plan-2026-07-13.md`, `../phase3-server-rebuild-plan-2026-07-13.md`. A subsequent execution-readiness pass corrected lifecycle-state semantics, cross-phase cleanup ownership, cohort-scoped measurements, spec-evidence requirements, and phase dependencies. The plan documents supersede this section's task-level detail (review corrected several anchors and premises here, e.g. the perf suite was already lane-isolated, the red test is host-config leakage, `/tmp` inode state moved); this section remains the strategic summary.

### The architectural decision

**Invert the control relationship at the supervision layer.** DDx keeps what the substrate lacks — the queue, worktree lifecycle, landing, durable evidence, cost accounting — and exposes them as tools the harness calls (MCP + skills, both partially existing), while the harness owns in-flight supervision, retries, and completion detection **in-band**. Concretely: an agent finishes by calling a completion tool with a typed result, instead of DDx inferring completion from commits, rationale files, and stderr afterward; gate failures return control to the *same live session* for repair instead of killing and re-spawning. Where DDx must still spawn headless work, it consumes a typed result contract (Agent SDK / structured stream / fizeau typed terminal events) — never scraped text. This retires, rather than repairs, the kill-regexes, the substring classifiers, the rationale-file contract, and the out-of-band review spawner. The weekly A/B baseline (P0.6) is the arbiter that this direction is converging.

### Phase 0 — Stop the self-harm (operator-driven, days)

- **P0.1 Reliability freeze with a WIP ratchet.** No new feature beads while open `kind:reliability` beads exceed a threshold; park (don't cancel) server-UI/federation/dashboard/metrics/website beads; re-triage the 33 ready beads against this review, consolidating the ~29 `niflheim-*` symptom beads into per-class fixes.
- **P0.2 Reclaim the host; make the leak class impossible.** Clear `/tmp/ddx-home-*` (19 GB, 100% of inodes), the ~27k lease dirs, the two locked registrations (`git worktree unlock` + `remove`), stale exec-wt scratch. Root fixes: delete the `os.TempDir()` append at `execution_cleanup.go:829-845`; relocate runner homes and all attempt scratch to one project-scoped, quota'd root with ownership metadata written *before* resource creation. Tripwire: a default-suite test that fails if an attempt leaves anything unowned under `os.TempDir()`.
- **P0.3 Remove the two active corruption sources.** (a) Lock-cap watchdog: never `RemoveAll` a live lock — observe-and-alert, or cancel the holder then release. (b) Fix the TOCTOU stale-break shared by the three lock implementations; collapse to one implementation.
- **P0.4 Make gates truthful.** Fix `TestWorkerManagerStopStaleDiskEntry` (red on main, 0.093 s) and the rest of the red suite; then scope closure gates to the packages a bead's diff touches plus a curated core-invariant suite, with the full suite running async post-land and auto-filing regression beads. Move `internal/server/perf` to its own CI lane. This kills the largest refusal bucket after ENOSPC (unrelated-red-gate `no_changes`).
- **P0.5 Stop destroying correct work.** Honor the agent's retry rationale — never synthesize commits over it (ddx-232aa2c0); propagate `ClosureGate` rejections as errors (`store.go:2058-2072`); take the per-attempt durable-audit *commit* out of the tracker-lock hot path (batch or async).
- **P0.6 Stand up the baseline that arbitrates everything.** Weekly: the same ~20-bead sample through direct Claude Code sub-agents vs `ddx work`; publish landed-rate, wall-clock, and cost side-by-side. **Closing that delta becomes the project's top-line metric**, replacing the PRD's unmeasurable criteria.

*Exit: `/tmp` inodes stable over a 20-attempt drain; zero unrelated-gate refusals for a week; attempt success back at the ~48% plateau.*

### Phase 1 — Lower the altitude (weeks)

- **P1.1 One typed failure owner.** Amend FEAT-006: fizeau must deliver typed terminal events (outcome enum + machine-readable cause + pdeathsig-safe spawning) or be replaced; delete the 22 kill-regexes, the ~50-substring classifiers, and the four redundant re-classification layers. One classifier, one retry policy, one escalation ladder — everything downstream consumes its verdict. (This also lands the standing harness-as-provider unification: one registry.)
- **P1.2 Dispatch-and-observe.** Preflight becomes advisory everywhere; the 7,823 predict-and-refuse rejections must go to ~zero. Cooldowns keyed on typed evidence only, and they must clear offline (`originMainHead` returning `""` on error currently pins them).
- **P1.3 Risk-proportional close gate (amend ADR-024) + in-band repair.** Default: deterministic checks + one cheap reviewer, gate failure returns to the live session; two-slot elevated review only for high-risk labels; review transport failures retry-or-downgrade, never fail the attempt. Target ≤1 LLM invocation of overhead per routine close.
- **P1.4 Write the missing worktree-lifecycle spec, then implement its single owner.** Enumerated states (created→claimed→landed/preserved→reaped), journal record written before `git worktree add`, crash-recovery semantics including `locked` (unlock-if-owner-dead→prune), lifecycle-classifier scratch routed through the same owner. Then delete the three reapers. Pin `LC_ALL=C` in `git.Command`. Land onto a non-checked-out integration ref — delete the checkout emulation that clobbered operator edits. Chaos test: SIGKILL mid-`worktree add`, assert full reclamation.
- **P1.5 Fix claim liveness structurally.** Leases return to synced tracker state (or in-row with machine identity) — never per-machine `/tmp` sidecars on a git-synced store; kill the 1 ms×3 sleeps and the 4× reparse in `Status()` (single-pass classification, in-process cache).
- **P1.6 Decompose `execute_bead_loop.go`** (8,199 lines, 261 commits since April) along its existing seams — intake → attempt → verify → land → report — as mechanical extraction with tests kept green.
- **P1.7 Closed means the class is dead.** A `kind:reliability`/`kind:bug` bead may close only with a named regression tripwire wired into the default suite. Retro-apply per-class to the niflheim backlog.

*Exit: `ddx try` ≥ direct sub-agent baseline on landed-rate AND wall-clock (P0.6 arbitrates); DDx-owned stages < 10% of losses; zero niflheim-class recurrence for 3 weeks.*

### Phase 2 — Make the documents true (parallel, cheap)

- **P2.1 Mechanical spec honesty.** CI check: any spec marked Complete whose named tests/commands are grep-absent fails. Re-stamp FEAT-002/-004/-009/-012/-018 against verified reality.
- **P2.2 Resolve the named contradictions once**: evidence tracked-vs-ignored (ADR-026 wins — code already complies), auto-commit default, reviewer tool access, `discovered_subtasks` vs in-band create, US-126 vs req 24 rebase, duplicate TD-040 id, duplicated requirement numbers, US-087/088 collisions, `bead.backend` vs `bead_tracker.backend`.
- **P2.3 Retire facades and dead surfaces**: the 15 panicking resolvers, `comparisonDispatch` and the placeholder `workerDispatch` kinds, `ddx templates` help text, the `metric` package and every deadcode-keepalive anchor (delete dead code; fix the reachability check if it blocks that), one of the two install paths, two of the three skill-tree copies.
- **P2.4 Metrics that feed decisions or don't exist.** Fix MET-001's unit and register its exec definition or retire it; MET-002 re-stamped aspirational until the join key works. Steering metrics = P0.6 baseline delta + landed-rate + defect-class recurrence, all computable from `attempts.jsonl` today.

### Phase 3 — Server: demote, substrate, then rebuild (after Phase 1)

- **P3.1 Demote to observer + intake.** Keep read surfaces, operator-prompt→bead intake, browsing. Suspend managed-worker lifecycle; a cron `ddx work --once` loop is the honest interim executor. Remove dirty-root silent parking in favor of a loud operator-visible state.
- **P3.2 Build the run substrate** (`.ddx/runs/` record.json, atomic temp-then-rename publish per FEAT-028's BlobStore discipline, `ddx runs list/show/children`) — the foundation the GraphQL vocabulary already assumes; single writer, typed, append-only.
- **P3.3 Reintroduce managed workers on one channel of truth** — worker state derived from the run substrate + project-scoped heartbeats; delete the restart-lost ingest registry and `ps`-scrape reconciliation; finish ADR-022 rev 6 to implementable depth (journal schema, reconcile states, named integration tests) *before* stamping anything Complete. UI e2e must then drive UI → server → subprocess → store against the real fixture server.

### Explicitly cut

Federation write path + ADR-029 lease, FEAT-029 managed nodes, multi-node dashboard, FEAT-019 evaluation UX, delivery metrics (FEAT-016/MET-002), website investment, P10-style auto-remediation loops — all Deferred with tombstones; their open beads leave the ready queue.

## 8. Confidence

- Empirical figures (§2), closure-gate defect, resolver stubs, worktree/inode/lease state, churn/LOC, monthly rates: **operator-verified directly**.
- All six subsystem gap analyses (§3.2–3.7): independently extracted, code-assessed, then **re-verified at file:line by dedicated gap passes**; the red test on main was executed, not inferred.
- Panel verdicts (§1, §4, §5): four adversarial single-hypothesis evaluations over the full dossier, each required to argue both sides and commit.
- Known unknowns: no historical baseline exists for direct-sub-agent success on these beads (P0.6 creates it); rationale-cause counts vary by counting method (file-level vs mention-level) — the environmental-dominance conclusion holds across all methods.

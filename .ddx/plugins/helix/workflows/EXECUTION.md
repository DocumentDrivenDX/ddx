---
dun:
  id: helix.workflow.execution
  depends_on:
    - helix.workflow
    - helix.workflow.tracker
---
# HELIX Execution Guide

This guide covers operator-facing HELIX execution flow: how to run bounded work
passes, how to decide whether more work remains, and how the HELIX wrapper
controls the queue.

For tracker integration, labels, `spec-id`, and `ddx bead` conventions,
see `ddx bead --help` (DDx FEAT-004). The wrapper now delegates bead
commands directly to `ddx bead`.

## Scope

This document owns HELIX execution behavior.

- Follow this file for queue guards, loop shape, and `NEXT_ACTION` handling.
- Follow the bounded action prompts under `.ddx/plugins/helix/.ddx/plugins/helix/workflows/actions/` for
  action-specific behavior.
- Treat examples elsewhere in `.ddx/plugins/helix/.ddx/plugins/helix/workflows/` as supportive summaries, not
  alternate execution contracts.

This is the HELIX-specific layer, not the portable skill packaging layer. Skill
installation lives at `.agents/skills`; queue control and action semantics live
here.

## The Double Helix

HELIX is built from two interleaved cycles — the double helix:

### Planning Helix

Identification and improvement of plans in bead format.

```
Review → Plan → Validate → (ready beads)
```

- **Review**: Assess current state (`check`, `align`). What work exists? What's
  missing? What concerns aren't threaded?
- **Plan**: Create or refine plan beads with consolidated context (`design`,
  `polish`, `evolve`, `triage`). Every plan bead consolidates inputs,
  cross-cutting concerns, current state, and acceptance criteria.
- **Validate**: Verify plan quality — are beads well-specified? Are concerns
  threaded? Are dependencies correct? Are acceptance criteria testable? This is
  what `polish` refinement passes do.

Output: a queue of well-specified, concern-threaded beads ready for execution.

### Execution Helix

Executing plans to update documents, reconcile artifacts, research next-stage
plans, implement code, test it, and optimize metrics.

```
Execute → Measure → Report → (new beads)
```

- **Execute**: Claim a bead, do the work it describes (`build`, `review`,
  `experiment`, `backfill` — any action that modifies files on disk).
- **Measure**: Verify results against the bead's acceptance criteria. Run
  concern-declared quality gates. Run ratchets. Record results on the bead.
- **Report**: Analyze measurement results. Open new beads for issues found,
  regressions, or follow-on work. Close the executed bead with evidence. New
  beads feed back into the planning helix.

### Crossover

The helices interleave at two points:

1. **Planning → Execution**: Ready beads move from the planning helix to the
   execution queue.
2. **Execution → Planning**: Report creates new beads that enter the planning
   helix for refinement.

`helix check` is the crossover point — it reads both helices and decides which
one needs attention next. It already does this implicitly (`BUILD` vs
`DESIGN`/`POLISH`/`ALIGN`), but the double-helix model makes the two cycles
explicit.

### Bead-First Principle

**Every action that modifies files must be governed by a bead.** No file
modifications without a plan bead — analogous to entering plan mode before
writing code.

Every action (except `triage` and `check`) follows this structure:

1. **Bead acquisition**: Find or create the governing bead for this work.
2. **Execution**: Do the work the bead describes.
3. **Measure**: Verify results and record evidence on the bead.
4. **Report**: Create follow-on beads and close the governing bead.

`triage` is the entry point that bootstraps the bead graph — it creates beads
and therefore cannot itself require one (that would be infinite regress).
`check` is read-only and does not modify files.

Planning-helix beads use the `kind:planning` label to distinguish them from
execution beads. Combined with an `action:<name>` label (e.g.,
`action:design`, `action:polish`), this makes the governing bead's purpose
visible in the tracker.

See `.ddx/plugins/helix/workflows/references/bead-first.md` for the full bead acquisition pattern,
`.ddx/plugins/helix/workflows/references/measure.md` for measurement recording, and
`.ddx/plugins/helix/workflows/references/report.md` for the report phase.

## Core Actions

HELIX supervision is built from bounded actions with distinct roles:

- `helix build`
  Executes one ready execution issue end-to-end, then exits.
- `helix status`
  Reports the persisted run-controller snapshot: current work, blockers, cycle
  timing, token counts, and next recommended action.
- `helix check`
  Performs the queue-drain decision and returns the maintained
  `NEXT_ACTION` vocabulary: build, design, issue refinement,
  alignment, backfill, waiting, guidance, or stopping.
- `helix align <scope>`
  Runs a top-down reconciliation review and can emit follow-up execution issues.
- `helix evolve <requirement>`
  Threads a requirement change through the artifact stack and updates the
  tracker when authority shifts.
- `helix design <scope>`
  Creates or extends the design stack when supervisory routing detects missing
  design authority for the requested scope.
- `helix polish <scope>`
  Decomposes design plans into implementable beads, then refines issue
  definitions and dependencies. This is the mandatory step between design and
  build — without it, agents attempt ad-hoc decomposition during implementation.
- `helix review [scope]`
  Performs fresh-eyes review after build before additional execution
  continues when review automation is enabled.
- `helix measure [bead-id|scope]`
  Runs acceptance criteria, concern-declared quality gates, and ratchet
  enforcement against a bead or scope. Records results on the bead. Can be
  invoked standalone or runs as an embedded phase within other actions.
- `helix report [bead-id|scope]`
  Analyzes measurement results, creates follow-on beads for identified work,
  and closes the governing bead with evidence. Per-bead by default; batch mode
  aggregates across a scope.
- `helix triage`
  Creates tracker issues via the `ddx bead` create command. This is the entry
  point that bootstraps the bead graph — the one action that does not require
  a governing bead.
- `helix backfill <scope>`
  Reconstructs missing HELIX docs conservatively from current evidence.

## Execution Model

Use a supervisory control loop with an explicit queue-drain sub-step:

1. Guard on true ready work with `ddx bead ready`, not `ddx bead list --ready`
2. Route to the least-power bounded subroutine required by user intent and repository state:
   - `evolve` when a requirement change must propagate through canonical artifacts
   - `design` when requested work lacks sufficient design authority
   - `polish` when plans need decomposition into beads, or governing specs changed and open issues need refinement
   - `build` when safe ready execution work exists
   - `review` after successful build when review automation is enabled
3. When the execution queue drains or supervisory routing needs a queue-health decision, run the bounded `check` action
4. Follow `check` exactly for queue-drain outcomes, without inventing a new code:
   - `BUILD`: continue the build loop
   - `DESIGN`: run one bounded design pass, then re-check
   - `POLISH`: run one bounded issue-refinement pass, then re-check
   - `ALIGN`: run reconciliation once if enabled, then re-check
   - `BACKFILL`: stop and hand off to `helix backfill <scope>`
   - `WAIT`: stop; do not attempt an unblock build pass
   - `GUIDANCE`: stop and ask for user or stakeholder input
   - `STOP`: stop because no actionable work remains

`ddx bead ready` is blocker-aware. `ddx bead list --ready` is not equivalent and should not
control an autonomous execution loop.

`design`, `polish`, and `review` participate in supervisory dispatch. `design` and
`polish` are now explicit `check` `NEXT_ACTION` codes for queue-drain routing;
`review` remains a post-build supervisory step rather than a
queue-drain code.

Execution principles:

- bead-first: every action that modifies files must have a governing bead
  before execution begins. No ad-hoc file changes without a plan bead.
- tracker-as-steering-wheel: use tracker primitives, not side channels, to
  redirect execution
- measure-and-record: verification results are recorded on the bead, not just
  logged ephemerally. A closed bead carries its measurement evidence.
- report-and-feed-back: measurement findings create new beads that re-enter the
  planning helix, closing the feedback loop
- do-hard-things: stay on the active epic and retry governed work with bounded
  exponential backoff before giving up
- cross-model verification: prefer `--review-agent` for post-build review when
  available
- continuous useful work: absorb small adjacent work when clearly required,
  and end with an explicit blocker report when progress stops

## Queue Guard

These examples assume `ddx` is available.

```bash
helix_ready_count() {
  ddx bead ready --json | ddx jq 'length'
}
```

## Manual Loop

This is the minimal safe operator loop:

```bash
while [ "$(helix_ready_count)" -gt 0 ]; do
  helix build
done

helix check
```

Interpret `check` as follows:

- `NEXT_ACTION: BUILD`
  More safe ready work exists; continue.
- `NEXT_ACTION: DESIGN`
  Run `helix design <scope>` once, then re-run `check`.
- `NEXT_ACTION: POLISH`
  Run `helix polish <scope>` once to decompose plans and refine issues, then
  re-run `check`.
- `NEXT_ACTION: ALIGN`
  Run `reconcile-alignment` once for the indicated scope if auto-alignment is
  enabled, then re-run `check`.
- `NEXT_ACTION: BACKFILL`
  Stop and hand off to `backfill-helix-docs` for the indicated scope.
- `NEXT_ACTION: WAIT`
  Stop. Do not attempt to build around the blocker or auto-unblock it.
- `NEXT_ACTION: GUIDANCE`
  Stop and get user or stakeholder input.
- `NEXT_ACTION: STOP`
  No actionable work remains for the current scope.

`helix run` is a bounded controller, not a repair loop.

- It counts only completed build passes toward `--max-cycles`.
- It may dispatch `helix design` or `helix polish` before build when
  supervisory state indicates missing design authority, undecomposed plans,
  or stale issue refinement.
- It may run fresh-eyes review after a successful build when review
  automation is enabled; `--no-auto-review` disables that post-implementation
  review.
- It may switch to `--review-agent` for cross-model verification during review.
- It may run `reconcile-alignment` every `N` completed implementation passes
  when `--review-every N` is set; `--no-auto-align` disables that post-drain
  alignment step.
- It may persist run-controller state for `helix status` including current
  issue, focused epic, attempt counters, cycle timing, and token totals.
- It should refresh `.helix/context.md` at run start, on epic switch, and
  every 5 completed implementation passes so long-lived sessions keep current
  build/test commands and tracker counts in view.
- It may stay on a selected epic until completion, then run a scoped post-epic
  review before leaving that scope.
- It should absorb small adjacent work that is clearly part of the current
  governed slice instead of creating avoidable tracker noise.
- It must emit a blocker report when it stops with skipped or intractable
  issues.
- It must capture Codex stdout and stderr together before token extraction so
  observability totals do not silently drop stderr-only token footers.
- It should batch related issues by shared parent or `spec-id`, and fall back
  to shared `area:*` labels when tracker data has no parent/spec sibling
  structure.
- It must not auto-dispatch backfill.
- It must not attempt an unblock build pass after `WAIT`.
- If a run is interrupted, recovery must be issue-scoped and non-destructive:
  do not clear a claim, revert files, or touch unrelated work without tracker
  evidence that the abandoned work belongs to that issue.
- After a failed or timed-out implementation attempt, retry is allowed only
  after issue-scoped cleanup leaves the worktree clean or the wrapper stops
  with a blocker; stale claims must be released before a fresh retry.

## `helix run`

This repo also provides a small wrapper CLI at `scripts/helix`.

Installation methods:

1. **Plugin mode** (recommended for Claude Code):
   ```bash
   claude --plugin-dir /path/to/helix
   ```

2. **DDx install** (for use in other repos):
   ```bash
   ddx install helix
   ```

3. **Health check** (verify or repair installation):
   ```bash
   helix doctor          # check status
   helix doctor --fix    # auto-repair symlinks
   ```

Skills are symlinked from `skills/` into `.agents/skills/` and `.claude/skills/`
within the repo. `ddx install helix` creates `~/.ddx/plugins/helix` pointing to
the repo, which resolves `.ddx/plugins/helix/workflows/...` paths used in skill
and action prompts.

Main commands:

- `helix run`
- `helix status`
- `helix build`
- `helix check`
- `helix align`
- `helix evolve`
- `helix design`
- `helix polish`
- `helix review`
- `helix measure`
- `helix report`
- `helix triage`
- `helix backfill`

`helix run`:

- loops only while true ready HELIX execution work exists
- routes to `helix design` or `helix polish` when supervisory state requires
  bounded design, plan decomposition, or issue refinement before build can resume
- runs one bounded build pass at a time
- runs `check` when the queue drains
- can trigger `reconcile-alignment` every `N` completed build passes
  or when `check` returns `ALIGN`
- may run `helix review` after each successful build pass when review
  automation is enabled; review findings are filed as tracker issues with
  label `review-finding` and the loop continues
- files acceptance check failures as tracker issues with label
  `acceptance-failure` instead of only logging to stderr
- may use `--review-agent` for cross-model review
- can stay focused on one epic through decomposition, child execution, and
  post-epic scoped review
- stops on `WAIT`, `BACKFILL`, `GUIDANCE`, or `STOP`
- uses the built-in tracker for queue state
- does not attempt an unblock build pass after `WAIT`
- does not auto-dispatch `helix backfill`
- treats interrupted runs as recoverable only when the abandoned work can be
  attributed safely and without reverting unrelated changes
- writes blocker reports and persisted lifecycle state for `helix status`

### `--summary` mode

Use `--summary` (or `-s`) when launching `helix run` as a background process
managed by an outer agent. This routes verbose output (tool calls, thinking,
prompt echo, gate detail) to the log file only, while emitting concise progress
lines with log-file line-range pointers:

```
helix: [14:24:01] cycle 1: hx-42 (5 ready)
helix: [14:24:35] codex complete (rc=0, 34s, 892 tokens) — log L12–L340 in .helix-logs/helix-...log
helix: [14:24:36] cycle 1: hx-42 → COMPLETE (1/3 done, 892 tokens)
```

When a cycle fails, read the referenced log line range for full diagnostics.
All verbose output is preserved in `.helix-logs/helix-*.log`.

Examples:

```bash
helix run
helix run --summary
helix run --review-every 5
helix status
helix check repo
helix align auth
helix design auth
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HELIX_AGENT` | `codex` | Default agent (`codex` or `claude`) |
| `HELIX_AGENT_TIMEOUT` | `2700` | Agent timeout in seconds (45 minutes) |
| `HELIX_MODEL` | — | Override AI model name (e.g., `claude-sonnet-4-6`) |
| `HELIX_EFFORT` | — | Set reasoning effort level for Claude/Codex |
| `HELIX_REVIEW_AGENT` | auto | Agent for cross-model reviews (defaults to the other agent) |
| `HELIX_CHECK_MODEL` | — | Cheaper model for queue-drain decisions |
| `HELIX_POLISH_MODEL` | — | Cheaper model for issue refinement |
| `HELIX_LIBRARY_ROOT` | `<repo>/workflows` | Override the workflow library root |
| `HELIX_TRACKER_DIR` | `.helix/` | Override the tracker directory |
| `HELIX_BEADS_DIR` | `.beads` | Override the beads interop directory |
| `HELIX_FORCE_EPHEMERAL` | `0` | Force ephemeral sessions (no resume) |
| `HELIX_AUTO_ALIGN` | `1` | Enable auto-alignment on ALIGN/STOP |
| `HELIX_ORPHAN_THRESHOLD` | `7200` | Staleness threshold in seconds for orphan recovery |
| `HELIX_BACKOFF_SLEEP` | — | Override exponential backoff delay (useful for testing) |
| `HELIX_TRACKER_LOCK_TIMEOUT` | `10` | Lock acquisition timeout in seconds |
| `HELIX_TRACKER_LOCK_POLL_INTERVAL` | `0.05` | Sleep interval while waiting for tracker lock |

### Run Options

| Flag | Default | Description |
|------|---------|-------------|
| `--review-every N` | `15` | Periodic alignment every N completed cycles |
| `--max-cycles N` | `0` (unlimited) | Stop after N completed cycles |
| `--review-threshold N` | `100` | Skip review for changes below N lines |
| `--no-auto-align` | — | Disable auto-alignment on ALIGN/STOP |
| `--no-auto-review` | — | Disable post-implementation review |
| `--summary`, `-s` | — | Concise output with log-file line pointers |
| `--quiet`, `-q` | — | Suppress agent startup and progress output |
| `--dry-run` | — | Print agent commands without executing |

### Command Aliases

| Alias | Canonical |
|-------|-----------|
| `helix implement` | `helix build` |
| `helix plan` | `helix design` |
| `helix verify` | `helix measure` |
## Orphan Recovery

At run start and after each failed implementation cycle, `helix run`
checks for stale in-progress issues and reclaims them automatically.

For each `in_progress` issue with the `helix` label:

1. **Skip** if another helix process is actively working on the issue.
2. **Skip** if `claimed-pid` is still alive.
3. **Skip** if the claim age (from `claimed-at`, or `updated` as fallback)
   is below `HELIX_ORPHAN_THRESHOLD` (default 2 hours).
4. **Reclaim** via `tracker update <id> --unclaim`.

Recovery resets tracker state only — it does not revert worktree changes.

## BUILD Loop Breaker

When `check` returns `NEXT_ACTION: BUILD` but no execution-eligible issue can
be selected, the loop tracks `consecutive_empty_builds`. After 2 consecutive
empty BUILD cycles:

1. Run orphan recovery to free any stale in-progress issues.
2. If ready count increased, reset the counter and continue.
3. Otherwise, stop with "no selectable issues after orphan recovery".

## Exponential Backoff

When an issue fails implementation, the wrapper retries with bounded
exponential backoff: `delay = min(5 * 2^(attempt-1), 40)` seconds.

| Attempt | Delay |
|---------|-------|
| 1 | 5s |
| 2 | 10s |
| 3 | 20s |
| 4+ | 40s (cap) |

After 4 failed attempts (75s total backoff), the issue is blocked as
intractable. If it is a child of the focused epic, the parent epic is also
blocked.

Override the delay with `HELIX_BACKOFF_SLEEP=0` for testing.

## Reproducible Testing

The wrapper is tested with deterministic command stubs rather than live
agent sessions.

Run:

```bash
bash tests/helix-cli.sh
```

This harness (133 tests):

- creates temporary git workspaces
- stubs `codex` and `claude` binaries via PATH injection
- seeds `.ddx/beads.jsonl` with known issue graphs
- drives exact ready-queue and `NEXT_ACTION` sequences
- verifies tracker CRUD, run loop orchestration, epic focus, queue drift,
  orphan recovery, summary mode, backoff, acceptance filing, review trailers,
  cross-model review, blocker reports, and installer behavior
- is implementation-language-agnostic: change `run_helix()` to invoke a
  different binary to verify a port

## Pre-Execution Pipeline

Before the implementation loop, the recommended sequence for new work is:

1. `helix design [scope]` — create a comprehensive design document through
   iterative refinement. The action acquires a `kind:planning,action:design`
   bead before writing the design doc.
2. `helix polish [scope]` — **decompose the plan into implementable beads**,
   then refine: deduplication, coverage verification, acceptance criteria
   sharpening, dependency wiring, concern threading (required before
   implementation). Polish acquires its own governing bead.
3. `helix run` — execute the bounded build loop. Each build cycle claims a
   ready bead, executes, measures, and reports.

**Every step is bead-governed.** Design creates a planning bead before writing
docs. Polish creates a planning bead before decomposing. Build claims an
execution bead before writing code. No files change without a governing bead.

**Polish is the bridge between design and build.** A design plan produces a
document with a work breakdown, but it does not create execution beads. Polish
reads the plan, creates one bead per implementable slice, wires dependencies,
threads concerns into context digests and acceptance criteria, and then refines
the resulting queue. Without this step, agents encounter epics or vague work
items and attempt ad-hoc decomposition during build.

**Measure and report close each cycle.** After execution, every action measures
results against its bead's acceptance criteria and records evidence on the bead.
The report phase creates follow-on beads for any new work identified and closes
the governing bead. These follow-on beads re-enter the planning helix.

The public command names for this sequence are `helix design`, `helix polish`,
and `helix run`.

`helix check` enforces this pipeline: it recommends `POLISH` when a plan
exists but has not been decomposed into beads, even if epics appear in the
ready queue. It also recommends `POLISH` when concerns have changed since the
last polish pass.

These steps are optional for small changes but strongly recommended for any
scope that will produce more than a handful of issues.

## Cross-Cutting Context in Beads

`helix triage` and `helix evolve` assemble a **context digest** into every
bead they create. The digest is a compact ~1000-1500 token summary of active
principles, area-matched concerns, merged practices, relevant ADRs, and
governing spec context. It is prepended to the bead description as a
`<context-digest>` XML block.

`helix polish` refreshes stale digests against current upstream state and
verifies that concern-appropriate acceptance criteria are present on every bead
in scope. When concerns change, polish propagates the change to all affected
beads — not just their digests but their acceptance criteria and quality gates.

`helix build` and `helix review` read the digest from the bead and use it
as working authority — they do not redundantly read the upstream files that
the digest summarizes.

`helix measure` verifies concern-declared quality gates as part of its
acceptance criteria check. Measurement results are recorded on the bead so
that a closed bead carries its verification evidence.

Concern threading is end-to-end: once a concern is introduced in
`docs/helix/01-frame/concerns.md`, it must propagate through context digests,
acceptance criteria, quality gates, and measurement evidence on every bead
whose area matches the concern's scope.

See `.ddx/plugins/helix/workflows/references/context-digest.md` for the assembly algorithm,
`.ddx/plugins/helix/workflows/references/concern-resolution.md` for concern loading, and
`.ddx/plugins/helix/workflows/references/principles-resolution.md` for principles loading.

## Next Issue

`helix next` prints the recommended next issue without spawning an agent:

```bash
helix next          # uses ddx bead ready ranking
```

## Fresh-Eyes Review

After implementing an issue, `helix review` performs 1-3 self-review passes
looking for bugs, integration issues, and security concerns with fresh
perspective:

```bash
helix review                  # review last commit
helix review ddx-abc123       # review changes for a specific issue
helix review src/auth/        # review specific files
```

Review findings are durable: the review action files each actionable finding
as a tracker issue with label `review-finding`. The run loop continues after
review rather than stopping, because the findings are now in the tracker and
will surface via `ddx bead list --label review-finding` or
`ddx bead ready` once they are ready for implementation.

Similarly, when acceptance checks fail in the run loop, the specific failures
are filed as tracker issues with label `acceptance-failure` so they appear in
the ready queue for the next cycle.

Operators can query and manage these findings like any other issue:

```bash
ddx bead list --label review-finding    # all unresolved review findings
ddx bead list --label acceptance-failure # all unresolved acceptance failures
ddx bead close <id>                     # resolve a finding
```

## Experiment Loop

`helix experiment` runs a single iteration of a metric-optimization loop for
`phase:iterate` issues. Each invocation: hypothesize → edit → test → benchmark →
keep/discard → log → exit.

The loop is driven externally by the `helix-experiment` skill or by the
operator re-invoking the command. This preserves the bounded-action model.

Experiments are operator-invoked only — `helix check` does not produce a
`NEXT_ACTION: EXPERIMENT` code. The operator chooses `helix experiment` instead
of `helix build` for optimization work.

The experiment action requires a clean worktree. The CLI prompts the user to
commit uncommitted changes before proceeding.

The optimization target is a HELIX metric definition at
`docs/helix/06-iterate/metrics/<name>.yaml`. If one exists, the experiment
reads it; if not, the experiment creates one during setup. This connects
experiments to ratchets and monitoring through a shared metric definition.

Session artifacts (`autoresearch.*`, `experiments/`) are untracked local files,
gitignored on the experiment branch. At session close (`helix experiment
--close`), the action squash-merges the experiment branch back to produce a
single commit and records the result in the issue close comment. Experiments
are execution-layer work tracked by issues, not canonical HELIX docs.

`--close` is unique to the experiment command — it directs the action to
execute session close (squash-merge, ratchet update, issue close) instead of
running another iteration.

Experiments validate governing artifacts at session setup and close (not
per-iteration). Per-iteration guardrails are: scoped files, mandatory test
passage, and the experiment's own constraints. All existing tests must pass
after every kept iteration.

## Practical Rules

- Keep execution bounded to one issue per implementation pass.
- Do not use an unconditional `while true` loop.
- Treat `check` as the queue-drain decision point, not `reconcile-alignment`.
- Use alignment to expose or refine the next work set, not as the default work
  picker.
- Do not auto-run backfill unless you are intentionally reconstructing missing
  canonical docs.

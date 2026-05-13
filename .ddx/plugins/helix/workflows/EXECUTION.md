---
ddx:
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

- HELIX models each governed execution step as a workspace-state
  transformation. Given workspace state `W`, executing bead `B` attempts to
  produce successor workspace state `W'`.
- Shorthand: `B : W -> W'`
- The bead is the intended transformation, not the evidence record.
- The execution run is the bounded attempt and evidence record for trying to
  realize `W -> W'`.
- The execution outcome records how that attempt landed (`merged`,
  `preserved`, `blocked`, `failed`, or equivalent workflow-visible result).
- The realized state delta is the material change between `W` and `W'`:
  docs, code, tracker state, generated artifacts, and other workspace changes.

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

Operationally, a bead should describe the intended transformation from current
workspace state `W` to successor workspace state `W'`. `measure` determines
whether the resulting `W'` satisfies the bead's acceptance criteria and quality
gates. `report` records evidence about the `W -> W'` transition and creates any
required follow-on beads.

Every action (except `triage` and `check`) follows this structure:

1. **Bead acquisition**: Find or create the governing bead for this work.
2. **Execution**: Do the work the bead describes.
3. **Measure**: Verify results and record evidence on the bead.
4. **Report**: Create follow-on beads and close the governing bead.

`triage` is the entry point that bootstraps the bead graph — it creates beads
and therefore cannot itself require one (that would be infinite regress).
`input` is also an entry point: it accepts sparse intent and creates or updates
the bead/workflow context that later actions execute. `check` is read-only and
does not modify files.

Planning-helix beads use the `kind:planning` label to distinguish them from
execution beads. Combined with an `action:<name>` label (e.g.,
`action:design`, `action:polish`), this makes the governing bead's purpose
visible in the tracker.

See `.ddx/plugins/helix/workflows/references/bead-first.md` for the full bead acquisition pattern,
`.ddx/plugins/helix/workflows/references/measure.md` for measurement recording, and
`.ddx/plugins/helix/workflows/references/report.md` for the report phase.

## Core Actions

HELIX supervision is built from bounded actions with distinct roles:

- `helix input "<natural language request>" [--autonomy low|medium|high]`
  Accepts sparse intent, applies HELIX autonomy semantics, and creates or
  updates the bead/workflow context needed for later execution. This is the
  planning-helix intake surface for the slider-autonomy model; the expected
  default autonomy is `medium`.
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
  Convenience entrypoint for a top-down reconciliation review. It first
  creates or claims the governing `kind:planning,action:align` bead, then
  runs the stored alignment prompt and emits properly ordered follow-on beads.
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

Use a supervisory control loop with an explicit queue-drain sub-step.

For sparse operator intent that is not yet represented as a bead or bounded
scope, start with `helix input` before entering the normal execution loop.
`helix input` shapes intent into governed work; `helix run`, `helix build`, and
`helix check` operate on the resulting bead/workflow state.

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

`ddx agent execute-loop` is the primary queue-drain substrate for
execution-ready beads. `helix run` remains useful only where it adds
supervisory routing, policy, or compatibility value above that substrate.

Execution principles:

- bead-first: every action that modifies files must have a governing bead
  before execution begins. No ad-hoc file changes without a plan bead.
- tracker-as-steering-wheel: use tracker primitives, not side channels, to
  redirect execution
- queue topology is explicit: if order matters, encode it with parent-child
  structure and dependencies instead of prose or operator memory
- measure-and-record: verification results are recorded on the bead, not just
  logged ephemerally. A closed bead carries its measurement evidence.
- report-and-feed-back: measurement findings create new beads that re-enter the
  planning helix, closing the feedback loop
- do-hard-things: stay on the active epic, prefer DDx-owned cooldown and
  preserve signals as the long-term retry surface, and file deterministic
  follow-on work instead of carrying hidden wrapper heuristics
- cross-model verification: prefer `--review-agent` for post-build review when
  available
- continuous useful work: absorb small adjacent work when clearly required,
  and surface blocked work through tracker state rather than prose-only memory

## Queue Guard

These examples assume `ddx` is available.

```bash
helix_ready_count() {
  # Strip advisory lines (e.g. upgrade notices) before piping to ddx jq.
  # awk skips lines until the first JSON delimiter, preserving multi-line JSON.
  ddx bead ready --json | awk 'found || /^[{[]/ { found=1; print }' | ddx jq 'length'
}
```

## Manual Loop

This is the canonical operator path once work is execution-ready:

```bash
while [ "$(helix_ready_count)" -gt 0 ]; do
  ddx agent execute-loop --once
done

helix check
```

`helix run` and `helix build` may still be used as compatibility wrappers when
an operator wants HELIX to provide transitional routing or wrapper ergonomics,
but the durable queue-drain primitive is `ddx agent execute-loop`, not an
independent HELIX-owned claim/execute/close loop.

### Architecture

HELIX owns **queue curation**: maintaining accurate bead topology (dependencies,
`execution-eligible`, `superseded-by`, epic hierarchy) so DDx's deterministic
`ReadyExecution()` ordering produces the intended sequence. HELIX does not
predict which bead DDx will select.

DDx owns **loop, selection, and execution**: bead selection, managed worktree
execution, close-with-evidence, retry suppression, and orphan recovery.

The intended adoption end state is: after each
`ddx agent execute-loop --once --json` call, HELIX parses `results[].bead_id`
and `results[].status`, then applies post-cycle supervisory policy to the bead
DDx actually executed, not a pre-selected bead.

### Current compatibility behavior

- `helix run` keeps HELIX-owned supervisory routing, queue-health decisions,
  post-build review/alignment hooks, and persisted run state, while delegating
  each build cycle to `ddx agent execute-loop --once`.
- `helix build` delegates the managed execution attempt to
  `ddx agent execute-bead` for explicit single-bead execution.
- Epic focus: HELIX sets `execution-eligible: false` on non-child beads to bias
  DDx's deterministic ordering toward epic children; no DDx flag needed.
- Current gap: the wrapper still pre-selects a bead and exports
  `HELIX_SELECTED_ISSUE` before DDx chooses the actual execution target. That is
  a temporary internal seam, not part of the HELIX/DDx contract.
- Current gap: review and periodic alignment still run as wrapper-owned
  post-cycle hooks. The queued-bead model is the target state, but it is not
  yet the shipped wrapper behavior.
- Current gap: the wrapper still carries legacy retry/backoff, skip-tracking,
  and blocker-report logic. The durable contract remains DDx-owned cooldown via
  `execute-loop-retry-after` plus tracker visibility through `ddx bead blocked`.

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

`helix run` is a bounded compatibility controller, not a repair loop.

- It counts only completed build passes toward `--max-cycles`.
- Contract target: parse `ddx agent execute-loop --once --json`, use
  `results[].bead_id` for all post-cycle bookkeeping, and avoid any private
  selector handoff that DDx does not expose.
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
- It may persist run-controller state for `helix status` including focused epic,
  attempt counters, cycle timing, and token totals.
- It should refresh `.helix/context.md` at run start, on epic switch, and
  every 5 completed implementation passes so long-lived sessions keep current
  build/test commands and tracker counts in view.
- It may stay on a focused epic until all children are done, then run a scoped
  post-epic review before leaving that scope. Epic focus uses `execution-eligible`
  curation, not DDx selection flags.
- Queue drift (superseded-by, parent change, spec-id change) is caught at `helix check`
  time before the bead enters the ready queue; `helix run` does not reopen beads after
  DDx close-with-evidence.
- Current wrapper behavior still includes legacy retry/backoff and blocker-report
  handling; treat that as transitional until the DDx-powered cleanup lands.
- Current wrapper behavior still runs review and alignment as post-cycle hooks;
  queued review/alignment beads remain the target design, not current fact.
- It must not auto-dispatch backfill.
- It must not attempt an unblock build pass after `WAIT`.
- DDx owns worktree orphan recovery, while the wrapper still performs its own
  tracker-claim orphan cleanup for stale `in_progress` beads.

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
  label `review-finding` plus scope-appropriate `area:*` labels derived from
  the reviewed bead or reviewed scope, and the loop continues
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

`helix run` is now a transitional HELIX-owned wrapper around a DDx-owned
execution substrate. The target contract is:

- `ddx agent execute-loop` owns single-project queue draining, claim/execute/
  close-with-evidence mechanics
- `ddx agent execute-bead` owns the bounded single-bead managed execution
  attempt inside that loop
- direct `ddx agent run` remains for planning, review, alignment, and other
  non-managed prompts that should not auto-claim and auto-close beads

As DDx parity hardens, HELIX should stop growing independent claim/execute/close
logic in the wrapper and instead focus on bead shaping, supervisory routing,
and interpretation of preserved or blocked outcomes.

### Command Boundary

After DDx queue-drain adoption, execution-oriented surfaces should be treated
as follows:

| Surface | Status | Intended use |
|---------|--------|--------------|
| `helix input` | first-class | Shape sparse intent into governed work before execution begins |
| `helix check` | first-class | Interpret queue state and DDx outcomes to choose the next bounded HELIX action |
| `helix align` | first-class | Launch bead-governed alignment planning work, not queue-drain execution |
| `helix review`, `helix design`, `helix polish`, `helix backfill` | first-class | Retained HELIX planning/review/reconciliation entrypoints |
| `helix run` | compatibility-only | Transitional wrapper over `ddx agent execute-loop --once` plus HELIX supervisory policy |
| `helix build` | compatibility-only | Transitional wrapper that resolves one ready bead, then launches `ddx agent execute-bead` |
| `helix run`, `helix build` | deprecation candidates | Remove only after DDx parity covers the HELIX-visible routing and evidence contract |

Migration guidance:

- Prefer `helix input` plus `ddx agent execute-loop` in new docs, quickstarts,
  and demo recordings.
- Keep public skill names aligned only with retained HELIX command surfaces;
  do not introduce `helix-*` aliases for DDx substrate commands.
- Plugin packaging may continue shipping retained compatibility wrappers, but
  their docs should present them as wrappers over DDx, not as the canonical
  queue-drain substrate.

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
| `HELIX_TRACKER_DIR` | `<repo>/.ddx` | Override the tracker directory |
| `HELIX_BEADS_DIR` | `.beads` | Override the beads interop directory |
| `HELIX_FORCE_EPHEMERAL` | `0` | Force ephemeral sessions (no resume) |
| `HELIX_AUTO_ALIGN` | `1` | Enable auto-alignment on ALIGN/STOP |
| `HELIX_ORPHAN_THRESHOLD` | `7200` | Staleness threshold in seconds for orphan recovery |
| `HELIX_BACKOFF_SLEEP` | — | Legacy wrapper backoff override retained only until DDx-powered retry cleanup lands |
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

DDx handles git worktree orphan recovery automatically for crashed agent sessions.

HELIX handles tracker-state orphan recovery: at run start and after each failed
implementation cycle, `helix run` checks for stale `in_progress` issues and
reclaims them.

For each `in_progress` issue with the `helix` label:

1. **Skip** if another helix process is actively working on the issue.
2. **Skip** if `claimed-pid` is still alive.
3. **Skip** if the claim age (from `claimed-at`, or `updated` as fallback)
   is below `HELIX_ORPHAN_THRESHOLD` (default 2 hours).
4. **Reclaim** via `ddx bead update <id> --unclaim`.

This is distinct from DDx worktree orphan recovery: HELIX reclaims abandoned tracker
claims, while DDx reclaims orphaned git worktrees from crashed agent runs.

## BUILD Loop Breaker

When `check` returns `NEXT_ACTION: BUILD` but no execution-eligible issue can
be selected, the loop tracks `consecutive_empty_builds`. After 2 consecutive
empty BUILD cycles:

1. Run orphan recovery to free any stale in-progress issues.
2. If ready count increased, reset the counter and continue.
3. Otherwise, stop with "no selectable issues after orphan recovery".

## Retry Suppression

Contract target:

- DDx sets `execute-loop-retry-after` to suppress immediate re-selection after
  a failed managed attempt.
- HELIX surfaces blocked or cooling-down work through `ddx bead blocked`.

Current wrapper gap:

- `scripts/helix` still implements legacy per-issue backoff, skip-tracking, and
  blocker-report behavior while the DDx-powered adoption cleanup remains open.
- Treat `HELIX_BACKOFF_SLEEP` and `.helix-logs/blockers-*.md` as compatibility
  residue, not as the enduring HELIX/DDx boundary contract.

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
  orphan recovery, summary mode, legacy backoff behavior, acceptance filing,
  review trailers, cross-model review, legacy blocker reports, and installer
  behavior
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

Execution-ready beads must also carry deterministic success-measurement
criteria. A bead meant for `ddx agent execute-loop` should name the exact
commands, checks, files, fields, or ratchets that demonstrate success. Prefer:

- `bash tests/helix-cli.sh` passes and `git diff --check` passes
- `.ddx/plugins/helix/workflows/EXECUTION.md` names `ddx agent execute-loop` as the queue-drain substrate

Avoid:

- `queue draining works`
- `docs are aligned`

If a bead cannot be closed from explicit evidence, it is not ready for a
DDx-managed execution lane and should be refined by `helix polish` or
`helix triage` before entering the execution queue.

If execution order matters, encode that order in the tracker as well: use
parent-child structure for grouped scope and `ddx bead dep add` for hard
prerequisites. `ddx agent execute-loop` should never rely on prose-only
sequencing or operator memory to know what is safe to land next.

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

Inside `helix run`, the post-implementation review target is resolved from the
executed bead first. When the implementation pass closes the bead and a
tracker-sync commit lands after the code commit, the loop reviews the bead's
`closing_commit_sha` instead of raw `HEAD~1`, so the threshold and review scope
still inspect the implementation diff rather than the tracker bookkeeping diff.

Review findings are durable: the review action files each actionable finding
as a tracker issue with label `review-finding` plus at least one
scope-appropriate `area:*` label derived from the reviewed bead or scope. The
run loop continues after review rather than stopping, because the findings are
now in the tracker and will surface via `ddx bead list --label review-finding` or
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

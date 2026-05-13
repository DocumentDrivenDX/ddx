# Feature Specification: FEAT-002 - HELIX CLI

**Feature ID**: FEAT-002
**Status**: backfilled
**Backfill Date**: 2026-03-25
**Scope**: supervisory shell CLI on top of DDx beads and agent service

## Summary

`helix-cli` is a shell entry surface on top of DDx primitives. It delegates
work-item storage to `ddx bead`, non-managed prompts to `ddx agent run`, and
managed execution to DDx execution surfaces (`ddx agent execute-bead` today,
with `ddx agent execute-loop` as the queue-drain contract). HELIX then adds
the workflow semantics that make it more than "call agent in a loop": prompt
selection, authority-aware routing, epic focus policy, queue drift detection,
auto-alignment, auto-review, and blocker reporting. The CLI is therefore a
convenience and compatibility surface, not the long-term owner of queue-drain
mechanics.

After DDx queue-drain adoption, the preferred execution path is:

1. `helix input "<natural language>"` when work still needs HELIX shaping
2. `ddx agent execute-loop` for execution-ready queue drain
3. `helix check`, `helix review`, `helix align`, `helix design`, or
   `helix polish` when HELIX supervision needs to interpret outcomes or route
   the next bounded planning action

Retained execution wrappers (`helix run`, `helix build`) exist to preserve
operator convenience and backward compatibility while DDx parity and migration
land. They must not be documented as a permanent parallel execution substrate.

The CLI provides one command surface for bounded execution (`run`, `build`,
`check`, `align`, `backfill`), supervisory steering (`status`, `evolve`,
`design`, `polish`, `review`, `triage`, `experiment`), tracker access
(`tracker` — thin wrappers around `ddx bead`), and helper navigation (`next`).
Users may still choose to interact directly with an agent and the tracker
instead of the CLI; the contract here is that the CLI stays thin enough that
both paths share the same underlying workflow rules.

Shell is the right form factor: HELIX is glue code that calls `ddx bead`,
DDx agent/execution surfaces, `git`, and project build tools. The workflow
action specs (~2,600 lines of markdown under `workflows/actions/`) are loaded
and interpolated by the shell — no compilation step needed.

## Users

- Repository operators running HELIX actions from a local checkout
- AI-assisted sessions that need a stable shell entrypoint
- Maintainers who need deterministic wrapper tests before changing CLI behavior

## Required Behavior

### Command Surface

The CLI must expose these top-level commands:

- `run`
- `status`
- `build`
- `check`
- `align`
- `backfill`
- `evolve`
- `design`
- `polish`
- `next`
- `review`
- `experiment`
- `triage`
- `commit`
- `tracker`
- `frame`
- `help`

Command aliases: `implement` → `build`, `plan` → `design`,
`tracker migrate` → `tracker import`.

### Execution Model

#### Post-DDx Queue-Drain Boundary

Execution-oriented surfaces fall into three categories:

| Surface | Status | Owner | Guidance |
|---------|--------|-------|----------|
| `helix input` | first-class | HELIX | Preferred intake surface when user intent is not yet represented as governed work |
| `helix check` | first-class | HELIX | Owns queue-health interpretation and `NEXT_ACTION` routing over tracker and DDx outcomes |
| `helix align` | first-class | HELIX | Retained as a bead-governed planning prompt launcher, not a parallel execution loop |
| `helix review`, `helix design`, `helix polish`, `helix backfill` | first-class | HELIX | Retained planning/review surfaces that shape or reconcile work without DDx-managed auto-close behavior |
| `helix run` | compatibility-only | HELIX over DDx | Wrapper for operators who still want one HELIX entrypoint; should delegate queue drain to `ddx agent execute-loop` as parity lands |
| `helix build` | compatibility-only | HELIX over DDx | Wrapper for one bounded execution pass when operators still want HELIX command ergonomics over `execute-bead` |
| `helix run`, `helix build` | deprecation candidates | HELIX | Eligible only after DDx exposes the required HELIX-visible routing and evidence hooks without wrapper-owned claim/close logic |

Migration rules:

- New docs, quickstarts, and demo scripts should prefer `helix input` plus
  `ddx agent execute-loop` for the default execution path.
- Plugin packaging continues to ship `bin/helix` and the mirrored
  `helix-<command>` skills, including retained compatibility wrappers, until a
  separate deprecation decision removes them.
- Future public HELIX skills should mirror HELIX-owned workflow entrypoints,
  not DDx substrate commands. DDx-managed surfaces stay documented as DDx
  commands rather than being reintroduced as HELIX skill names.
- Compatibility wrappers may remain implemented and installed, but must be
  labeled as compatibility or migration surfaces rather than canonical queue
  drain.

- `run` must continue only while true ready work exists, then call `check` when
  the queue drains.
- `build` must execute one bounded build pass.
- Managed bounded execution must flow through `ddx agent execute-bead`.
- The queue-drain contract should converge on `ddx agent execute-loop`; any
  wrapper-owned queue-drain behavior is compatibility logic, not the long-term
  execution substrate.
- `align` must not act as an ad hoc standalone review silo; it should create
  or claim the governing `kind:planning,action:align` bead and dispatch the
  stored alignment prompt against that bead-governed scope.
- Direct `ddx agent run` remains appropriate for non-managed prompts such as
  `check`, `review`, `align`, `design`, and `polish`.
- `status` must report a structured lifecycle snapshot sourced from persisted
  run-controller state.
- `check` must return a `NEXT_ACTION` code used to decide whether to build,
  design, polish, align, backfill, wait, ask for guidance, or stop.
- `run` must treat `NEXT_ACTION` as authoritative:
  - `BUILD`: continue with the next bounded build pass.
  - `DESIGN`: run one bounded design pass, then re-evaluate queue state.
  - `POLISH`: run one bounded issue-refinement pass, then re-evaluate queue
    state.
  - `ALIGN`: run the alignment workflow once, then re-evaluate queue state.
  - `BACKFILL`: stop and surface the explicit `helix backfill <scope>` command
    before execution resumes.
  - `WAIT`: stop without attempting implementation.
  - `GUIDANCE`: stop and surface the required decision.
  - `STOP`: stop because no actionable work remains.
- `run` must support epic focus mode: when an epic is selected, remain on that
  epic's children until it is complete or explicitly blocked.
- `run` must retry difficult work with bounded exponential backoff before
  declaring it blocked.
- `run` must absorb small adjacent work into the current slice when the change
  is clearly part of satisfying the same governing acceptance.
- `run` must capture Codex token-footers even when Codex writes them to stderr
  so token accounting and status reporting do not silently drop usage.
- Only successfully completed build passes count as completed cycles.
- Failed implementation attempts, reviews, alignment, backfill, and recovery
  retries must not be counted as completed cycles.
- `run` must refresh `.helix/context.md` at run start, on epic switch, and at
  least every 5 completed build cycles so long-lived sessions do not execute
  from stale repository context.
- The refreshed context file must include the repository build and test
  commands from the `AGENTS.md` Quick Reference section plus current open,
  in-progress, ready-execution, and closed issue counts.
- After each successful build pass, `run` must perform a fresh-eyes review
  before advancing to the next cycle.
- When `--review-agent` is configured, post-build review must switch to the
  alternate agent for cross-model verification.
- When an epic closes, `run` must perform a scoped post-epic review against
  the epic's governing spec before leaving that scope.
- A review with findings must be surfaced as actionable follow-up before the
  loop advances.
- When parent- and `spec-id`-based sibling detection finds no related ready
  work, `run` must fall back to matching execution-safe siblings by shared
  `area:*` labels before batching unrelated issues together.
- After a failed or timed-out implementation attempt, `run` must clean up
  issue-scoped state before any retry: leave the worktree clean or stop with a
  blocker, and return the issue to `open` when the failed attempt should be
  retried fresh.
- When the loop stops with skipped work, `run` must emit a blocker report and
  persist enough state for `helix status` to explain the stop condition.
- `run` must support `--summary` (or `-s`) mode which routes verbose output to
  the log file only and emits concise progress lines with log-file line-range
  pointers, reducing token consumption when monitored by an outer agent.
- `run` must detect consecutive empty BUILD cycles (check returns BUILD but no
  issue can be selected) and stop after 2 consecutive empties, attempting
  orphan recovery first.
- `run` must use bounded exponential backoff: `min(5 * 2^(attempt-1), 40)`
  seconds, blocking the issue as intractable after 4 failures. The backoff
  delay can be overridden via `HELIX_BACKOFF_SLEEP`.
- When a child issue is blocked as intractable during epic focus, the parent
  epic must also be blocked.
- The CLI must document which parts of the run loop remain HELIX-owned
  supervision versus DDx-owned execution mechanics so the migration boundary
  stays explicit.
- The CLI must document which commands remain first-class workflow entrypoints
  versus thin compatibility wrappers or deprecation candidates as DDx reaches
  parity.

### Tracker Model

- The CLI must expose `ddx bead` as thin wrappers around `ddx bead`.
- `ddx bead create` delegates to `ddx bead create` with HELIX-specific
  validation enforced via a DDx validation hook at
  `.ddx/hooks/validate-bead-create`.
- HELIX validation requires: `helix` label, one phase label, `--spec-id` for
  tasks, and deterministic `--acceptance` for tasks and epics.
- Execution-ready implementation beads must also encode the real ordering
  constraints using parent-child relationships and `ddx bead dep add`, rather
  than relying on prose descriptions of order.
- Compatibility wrappers must only dispatch beads whose readiness, ordering,
  and success contract are already explicit in tracker state; `helix run` and
  `helix build` must not hide sequencing assumptions that belong in the bead
  graph.
- Tracker data lives in `.ddx/beads.jsonl` (DDx bead configured with
  `DDX_BEAD_DIR=.ddx`, `DDX_BEAD_PREFIX=hx`).
- Ready work is determined by `ddx bead ready` (open beads with all deps
  closed). Execution-eligible filtering uses `ddx bead ready --execution`.
- Claim semantics (`--claim`, `--unclaim`, `claimed-at`, `claimed-pid`) are
  provided by DDx bead's `Claim`/`Unclaim` operations.
- Orphan recovery checks PID liveness and claim age before reclaiming.
  The staleness threshold defaults to 7200 seconds (2 hours) and is
  configurable via `HELIX_ORPHAN_THRESHOLD`.
- Recovery preserves unrelated worktree changes — it resets tracker state
  only, it does not revert files.

### Commit

- `commit [issue-id]` must stage all modified files if nothing is staged,
  run the build gate (lefthook, cargo check, or npm test), commit with the
  issue title as the summary, push with rebase, and close the tracker issue.
- `commit` without an issue ID must generate a summary from changed filenames.
- `commit` must fail if there are no changes to commit.
- `commit` must fail if the build gate fails.

### Operator Safeguards

- The experiment flow must require a clean worktree before continuing.
- Backfill output must include machine-readable `BACKFILL_STATUS`,
  `BACKFILL_REPORT`, and `RESEARCH_EPIC` trailers, and the declared report file
  must exist.

### Installation

#### Primary: Plugin mode (see [[FEAT-004]])

- The HELIX repo root is a Claude Code plugin. Loading it via
  `claude --plugin-dir /path/to/helix` or project-level plugin settings must
  make all HELIX skills, the CLI (`bin/helix`), and shared resources available
  automatically.
- No manual symlink step is required. New skills are available in the next
  session after the file is created.
- `bin/helix` is added to PATH by the plugin loader and delegates to
  `scripts/helix` via `${CLAUDE_PLUGIN_ROOT}`.
- Plugin docs should present `helix input` plus `ddx agent execute-loop` as
  the default managed-execution path. `helix run` and `helix build` remain
  available in plugin mode as retained compatibility wrappers, not as the
  preferred long-term queue-drain contract.

#### Legacy: Symlink installer

- `scripts/install-local-skills.sh` remains as a development convenience for
  contributors who want HELIX skills available outside of plugin mode.
- The installer must install HELIX skill entrypoints into `~/.agents/skills`
  and mirror them into `~/.claude/skills` for Claude compatibility.
- The installed skill links must preserve package-relative access back to the
  shared `workflows/` resource library in the HELIX repo.
- The installer must create `~/.local/bin/helix` as a launcher that invokes
  the repository's `scripts/helix`.
- The installer should print a notice recommending plugin mode as the primary
  installation path.

## Acceptance Criteria

- Running `helix help` shows the command surface and key options.
- Running `helix status` reports a structured lifecycle snapshot derived from
  persisted run-controller state.
- Running `ddx bead` subcommands supports create/show/update/close/list,
  ready/blocked queries, dependency management, and status summaries.
- Running `helix triage` produces tracker-valid issues with required labels,
  governing artifact reference, and deterministic acceptance criteria.
- Running `helix align` first acquires a governing `kind:planning,action:align`
  bead before it writes reports or follow-on execution issues.
- Running `helix run` follows the explicit `NEXT_ACTION` contract for
  `BUILD`, `DESIGN`, `POLISH`, `ALIGN`, `BACKFILL`, `WAIT`, `GUIDANCE`, and
  `STOP`.
- Running `helix run` treats DDx-managed execution as the implementation
  substrate: `execute-bead` for bounded managed work, and `execute-loop` as
  the target queue-drain contract.
- Running `helix run` and related docs make clear that HELIX CLI execution
  surfaces are convenience wrappers over DDx-managed execution rather than a
  permanent parallel substrate.
- Governing docs explicitly classify execution-oriented HELIX surfaces into
  first-class workflow entrypoints, compatibility-only wrappers, and
  deprecation candidates after DDx queue-drain adoption.
- Plugin-mode usage, skill naming, and demo guidance all prefer
  `helix input` plus `ddx agent execute-loop` as the default execution path,
  while retaining `helix run` / `helix build` only as compatibility surfaces.
- Running `helix run` does not attempt implementation after `WAIT`.
- Running `helix run` stops and surfaces the exact backfill command after
  `BACKFILL`.
- Running `helix run` counts only completed build passes as completed
  cycles.
- Running `helix run` captures Codex token usage even when the token footer is
  emitted on stderr.
- Running `helix run` refreshes `.helix/context.md` every 5 completed cycles
  and the refreshed context contains Quick Reference build/test commands plus
  current issue counts.
- Running `helix run` surfaces review findings before the loop advances.
- Running `helix run` stays focused on a chosen epic until the epic finishes
  or an explicit blocker forces release.
- Running `helix run` emits blocker-report output and observability metadata
  for cycle timing and token accounting.
- Running `helix run` batches sibling work by shared `area:*` labels when
  parent and `spec-id` metadata are absent.
- Running `helix run` does not discard unrelated worktree changes during
  recovery.
- Running `helix run` does not retry a failed or timed-out implementation
  attempt with stale claims or leftover issue-scoped worktree state.
- Running `helix run --summary` produces concise one-liner output with
  log-file line-range pointers while routing verbose detail to the log file.
- Running `helix run` stops after 2 consecutive BUILD cycles with no
  selectable issues, attempting orphan recovery before stopping.
- Running `helix run` reclaims orphaned issues when PID is dead and claim age
  exceeds threshold.
- Running `helix run` blocks the parent epic when a child is intractable.
- Running `ddx bead update <id> --claim` records `claimed-at` and
  `claimed-pid` metadata.
- Running `ddx bead update <id> --unclaim` restores `open` status and
  clears claim metadata.
- Running `helix backfill <scope>` enforces the required trailers and durable
  report creation contract.
- Running `bash tests/helix-cli.sh` remains the required deterministic
  verification path for wrapper behavior changes (133 tests).

## Evidence

- `docs/helix/01-frame/prd.md`
- `docs/helix/01-frame/features/FEAT-004-plugin-packaging.md`
- `docs/helix/02-design/solution-designs/SD-001-helix-supervisory-control.md`
- `docs/helix/02-design/technical-designs/TD-002-helix-cli.md`
- `docs/helix/02-design/contracts/CONTRACT-001-ddx-helix-boundary.md`
- `docs/helix/01-frame/features/FEAT-011-slider-autonomy.md`
- `docs/helix/03-test/test-plans/TP-002-helix-cli.md`
- `workflows/README.md`
- `workflows/EXECUTION.md`
- DDx FEAT-004 (beads) and FEAT-006 (agent service)

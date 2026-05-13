# Technical Design: TD-002-helix-cli

**Status**: backfilled
**Backfill Date**: 2026-03-25

## Story Reference

**User Story**: [HELIX CLI wrapper execution] | **Feature**: [FEAT-002-helix-cli] | **Solution Design**: [SD-001-helix-supervisory-control]

## Acceptance Criteria Review

- The wrapper keeps HELIX execution bounded.
- Queue-drain decisions follow the documented control contract.
- Tracker-backed execution remains deterministic and testable.
- Local installation exposes the mirrored HELIX command surface safely.

## Technical Approach
Provide a thin Bash wrapper that keeps HELIX execution bounded, delegates
supervisory decisions to documented HELIX actions, and delegates managed
execution to DDx execution surfaces without requiring operators to assemble
prompts manually. The wrapper is an entrypoint and compatibility layer, not
the durable home of queue-drain mechanics.

This design defines the orchestration contract for `helix run`. The wrapper is
not allowed to invent its own workflow policy, and it must not grow a parallel
managed-execution substrate now that DDx ships `execute-bead` and is adopting
`execute-loop` as the queue-drain contract.

## Orchestration Contract

`helix run` is a bounded supervisor around HELIX queue state.

- It always starts from tracker-backed ready work, not from assumptions about
  in-progress work.
- It delegates managed bead execution to DDx instead of owning the git-aware
  execution mechanics itself.
- It should converge on `ddx agent execute-loop` as the queue-drain substrate,
  with `ddx agent execute-bead` as the bounded execution primitive underneath.
- It treats alignment as a bead-governed planning action: acquire the
  `kind:planning,action:align` bead, then run the stored prompt and file
  properly ordered follow-on beads.
- It treats `check` as the source of next-step guidance after the ready queue
  drains.
- It revalidates selected issue state at safe boundaries so concurrent local
  refinement does not lead to stale claim or close behavior.
- It keeps alignment and backfill as distinct follow-up actions.
- It does not auto-build blocked work on `WAIT`.
- It does not silently continue after a review failure.
- It persists run-controller state so `helix status` can report lifecycle
  progress, cycle timing, focused epic, blocked work, and token usage.
- It prefers cross-model verification when `--review-agent` is configured.
- It stays focused on a chosen epic until the epic completes or a blocker is
  recorded.

## Component Changes

### Modified: Wrapper entry and run loop
- **Current State**: Bash wrapper around HELIX actions.
- **Changes**: Enforce bounded execution, queue-drain checks, and review-aware
  cycle control while keeping DDx as the queue-drain owner.

### Modified: Tracker integration
- **Current State**: Local JSONL issue tracker.
- **Changes**: Dependency-aware ready/blocked selection, ownership, and
  recovery-aware orchestration.

### Modified: Installer and harness
- **Current State**: Local launcher and deterministic wrapper tests.
- **Changes**: Keep mirrored command install and deterministic verification in
  sync with the wrapper contract.

## State Machine

### Ready Work

When `ddx bead ready --json` reports one or more ready execution issues:

1. Select or scope the next ready execution issue using the tracker ranking
   rules and HELIX queue policy.
2. Re-read the issue and verify it is still a safe execution target.
3. Dispatch DDx-managed execution:
   - target contract: `ddx agent execute-loop --once`
   - compatibility path while loop parity lands: `ddx agent execute-bead <id>`
4. Read the structured DDx outcome and treat landed vs preserved state as the
   source of truth for what happened during the attempt.
5. Re-check workflow drift before any HELIX-owned post-execution review or
   follow-on work.
6. Count the pass as a completed cycle only if the DDx-managed attempt lands
   and the bead closes.

### Queue Drain

When the ready queue is empty, `helix run` must execute `check` once and obey
the first `NEXT_ACTION` result exactly:

- `NEXT_ACTION: BUILD`
  Re-enter ready-work selection and continue only if the tracker now reports
  ready execution work.
- `NEXT_ACTION: DESIGN`
  Run one bounded design pass, then re-check queue state before any
  build resumes.
- `NEXT_ACTION: POLISH`
  Run one bounded issue-refinement pass, then re-check queue state before any
  build resumes.
- `NEXT_ACTION: ALIGN`
  Run one alignment pass if auto-alignment is enabled. If the follow-up check
  still returns `ALIGN`, stop and surface the alignment command rather than
  looping forever.
- `NEXT_ACTION: BACKFILL`
  Stop and surface the explicit `helix backfill <scope>` command. Backfill is a
  separate cross-phase action and is not auto-run by `helix run`.
- `NEXT_ACTION: WAIT`
  Stop immediately. `WAIT` means execution is blocked by claimed work or a
  truly external dependency; `helix run` must not attempt an unblock build
  pass.
- `NEXT_ACTION: GUIDANCE`
  Stop immediately and surface the required user or stakeholder decision.
- `NEXT_ACTION: STOP`
  Stop cleanly because no actionable work remains for the scope.

### Cycle Accounting

The loop must distinguish between attempted work and completed work.

- `attempted_cycles`: increment when the wrapper starts a build
  attempt.
- `completed_cycles`: increment only after build succeeds, the issue
  is closed, and post-implementation review passes when review is enabled.
- `--review-every N` and `--max-cycles N` must use `completed_cycles`, not
  attempted cycles.
- Failed build attempts do not count toward the cycle limit.
- `cycle_start`, `cycle_end`, `cycle_duration`, and `total_tokens` belong in
  persisted run-controller state and in the loop's progress output.
- The Codex runner must capture stdout and stderr together before token
  extraction so the `tokens used` footer is accounted for regardless of which
  stream Codex used.
- `.helix/context.md` must be regenerated at run start, on epic switch, and
  after every 5 completed build cycles. The generator must include:
  - the Quick Reference build and test commands from `AGENTS.md`
  - current open, in-progress, ready-execution, and closed issue counts
  - current focused epic metadata when epic mode is active

### Exponential Backoff

When an issue fails implementation, the wrapper retries with bounded
exponential backoff before declaring it blocked:

- Formula: `delay = min(5 * 2^(attempt-1), 40)` seconds
- Attempt 1: 5s, Attempt 2: 10s, Attempt 3: 20s, Attempt 4+: 40s (cap)
- After 4 failed attempts (75s total backoff), the issue is blocked as
  intractable and added to the skip list.
- If the blocked issue is a child of the focused epic, the parent epic is
  also blocked.
- The backoff sleep can be overridden with `HELIX_BACKOFF_SLEEP` (useful for
  testing).

### Summary Mode

The `--summary` (or `-s`) flag routes verbose output to the log file only
while emitting concise progress lines with log-file line-range pointers:

```
helix: [14:24:01] cycle 1: hx-42 (5 ready)
helix: [14:24:35] codex complete (rc=0, 34s, 892 tokens) — log L12–L340 in .helix-logs/helix-...log
helix: [14:24:36] cycle 1: hx-42 → COMPLETE (1/3 done, 892 tokens)
```

Implementation uses two output helpers:
- `summary_line`: writes to fd 3 (original stderr, always visible to the
  operator)
- `verbose_line`: writes to the log file only in summary mode, or to stderr
  in normal mode

Summary mode implies `--quiet` (no agent startup messages or stream-json
tool-call output).

## Recovery and Ownership

`helix run` needs a safe recovery model for crashed or orphaned sessions.

### Ownership Model

Claimed work is advisory state in the tracker, not a license to rewrite the
worktree blindly.

- A claimed execution issue is represented by `status: in_progress` and
  `assignee: helix`.
- The tracker records claim freshness metadata on `--claim`:
  - `claimed-at`: ISO-8601 UTC timestamp of the claim
  - `claimed-pid`: OS process ID of the claiming HELIX session
- The inverse operation `--unclaim` restores the issue to `open`, clears
  `assignee`, and nulls `claimed-at` and `claimed-pid`.
- Recovery may only reclaim work when the claim is demonstrably stale or when
  the user explicitly requests recovery.
- Absence of a currently running process is not sufficient evidence that a
  claim is orphaned — the claim age must also exceed the staleness threshold.

### Recovery Algorithm

Recovery runs at the start of `helix run` and after each failed implementation
cycle. For each `in_progress` issue with `assignee = helix`:

1. **Skip** if another helix process is actively working on the issue
   (`pgrep` check).
2. **Skip** if `claimed-pid` is still alive (`kill -0` check).
3. **Skip** if the claim age (from `claimed-at`, or `updated` as fallback for
   legacy issues) is below `HELIX_ORPHAN_THRESHOLD` (default: 7200 seconds /
   2 hours).
4. **Reclaim** via `tracker update <id> --unclaim`.

Recovery resets tracker state only — it does not revert worktree changes or
attribute partial work to files.

### Recovery Rules

Recovery must be non-destructive by default.

- The wrapper must not perform a broad `git checkout -- .` or otherwise erase
  unrelated local changes.
- After any implementation failure or timeout, the wrapper releases the
  advisory claim via `--unclaim` so the next cycle does not inherit stale
  ownership.
- Recovery results are visible in the run's stderr output and log file.

### Queue Drift Fingerprinting

The wrapper detects concurrent modifications to claimed issues by computing
a fingerprint before implementation and comparing it after:

- **Fingerprint fields**: `spec-id`, `parent`, `superseded-by`, `replaces`
- **Computed**: immediately after selection, before claim
- **Recomputed**: immediately after implementation, before close
- **On mismatch**: the issue is unclaimed and skipped (queue drift detected)

### BUILD Loop Breaker

When `check` returns `NEXT_ACTION: BUILD` but no execution-eligible issue can
be selected, the loop tracks consecutive empty BUILD cycles. After 2
consecutive empty BUILDs:

1. Run orphan recovery to free any stale in-progress issues.
2. If orphan recovery increased the ready count, reset the counter and
   continue.
3. If still no selectable issues, stop the loop with a clear message.

## Concurrent Interactive Refinement

`helix run` must support the local operating mode where one session advances
execution while another session refines specs or tracker issues, including
direct conversational agent work that creates or updates beads without going
through a CLI wrapper.

### Material Queue Drift

Material drift includes any change that can invalidate execution authority or
completion status for the currently selected issue:

- changed `spec-id`
- changed dependencies
- changed parent or replacement relationship
- superseded execution issue
- execution-eligibility change that removes the issue from the runnable set

### Revalidation Rules

- Before claim:
  - re-read the issue and verify it is still runnable
  - if materially changed, skip claim and return to queue evaluation
- Before close:
  - re-read the issue and verify it has not been superseded or structurally
    invalidated
  - if materially changed, do not close it from stale assumptions; stop, reopen
    the decision path, or create follow-up work as appropriate

### Execution Eligibility

The wrapper must distinguish execution-safe work from general open work so
interactive refinement issues do not get treated as build targets by
accident.

### Batch Selection

Batching exists to save context-loading cost without collapsing unrelated work.

- The primary related-work heuristics remain shared parent and shared
  `spec-id`.
- When those heuristics produce no sibling candidates, batching must fall back
  to matching ready execution-safe issues by shared `area:*` labels.
- Area-label fallback must still exclude skipped issues, epics, and the
  primary issue itself.
- If no parent, `spec-id`, or area-label siblings exist, the wrapper must
  execute the primary issue alone.

## Review Handling

Post-build review is part of the orchestration contract.

- A clean review allows the loop to continue.
- Review findings must create or reopen follow-up work before the loop
  advances to the next issue.
- Review should be machine-readable enough for the wrapper to distinguish
  `CLEAN` from actionable findings.
- If review cannot be interpreted safely, the loop must stop instead of
  assuming success.
- When `--review-agent` is configured, review must run under the alternate
  model rather than the implementation model.
- When an epic closes, the loop must run a scoped post-epic review against the
  epic's governing artifact before releasing focus.
- Actionable findings from review, alignment, measure, or report must be filed
  as beads with explicit parent/dependency topology whenever ordering matters;
  the wrapper must not depend on operator memory to infer the next runnable
  slice.

## Blockers And Work Absorption

- Small adjacent work discovered while satisfying the same governing
  acceptance, such as related manifest updates or directly coupled file edits,
  should be absorbed into the current issue instead of spawning avoidable
  tracker noise.
- Work that is adjacent but not clearly required by the current acceptance
  must still become separate follow-up issues.
- When the loop exits with skipped or intractable work, it must emit a blocker
  report that names the issue, reason, attempts, and any epic-level impact.

## Components

### 1. Wrapper Entry Script

`scripts/helix` resolves the repository root, selects the workflow library
root, sources the tracker library, opens a session log, parses CLI flags, and
dispatches commands.

### 2. Prompt Builders

Each agent-facing command builds a prompt that points at the authoritative
workflow action file and injects repository-local instructions such as tracker
usage and required output trailers.

### 3. Agent Runner

`run_agent_prompt` remains the surface for non-managed prompts such as
`check`, `review`, `align`, `design`, and `polish`. Managed implementation
attempts should flow through DDx execution commands instead of raw
`ddx agent run`.

### 4. Built-In Tracker Library

`ddx bead` manages issues stored as JSONL in `.ddx/beads.jsonl`. It
provides creation, read/update/close flows, dependency management, ready and
blocked queue queries, and tracker health summaries.
For this design, it is the source of truth for claim ownership and claim
freshness metadata.

### 5. Loop Controller

`run_loop` is the orchestration layer for `helix run`. It:

- checks ready work before implementation
- persists run-controller state for status and observability
- revalidates the selected issue immediately before claim and before close
- delegates one bounded managed execution pass at a time through DDx
- calls `check` after the queue drains
- honors `DESIGN` and `POLISH` queue-drain results before build resumes
- can auto-run alignment once after `ALIGN`
- stops on `WAIT`, `GUIDANCE`, or `STOP`
- stops on `BACKFILL` and surfaces the explicit follow-up command
- can trigger periodic alignment with `--review-every` using completed cycles
- uses epic focus, bounded exponential backoff, and blocker reporting to keep
  useful work moving without losing traceability
- must not attempt an unblock build pass after `WAIT`

Migration note:
- HELIX-owned issue selection, epic focus, batching, and queue-drain routing
  still exist today.
- The execution contract being designed for is DDx-managed queue drain via
  `execute-loop`, not indefinite growth of wrapper-owned claim/execute/close
  logic.
- If `execute-loop` cannot yet express a required HELIX supervisory behavior,
  that gap should become explicit DDx follow-on work, ordered in the tracker
  with parents and dependencies where needed, rather than hidden shell policy.

### 6. Installer

`scripts/install-local-skills.sh` links the HELIX skill entrypoints into
`~/.agents/skills`, mirrors them into `~/.claude/skills` for Claude
compatibility, preserves package-relative access back to the repository's
shared `workflows/` library, makes `scripts/helix` executable, and installs a
local `helix` launcher under `~/.local/bin`.

### 7. Deterministic Test Harness

`tests/helix-cli.sh` creates temporary git workspaces, stubs agent binaries,
seeds tracker state, and verifies command behavior without relying on live
agent sessions.

## Data and Filesystem Surfaces

- Workflow docs root: `workflows/`
- Tracker state: `.ddx/beads.jsonl`
- Session logs: `.helix-logs/helix-YYYYMMDD-HHMMSS.log`
- Run-controller state: `.ddx/run-state.json`
- Blocker reports: `.helix-logs/blockers-YYYYMMDD-HHMMSS.md`
- Installed launcher: `~/.local/bin/helix`
- Installed skill links: `${CODEX_HOME:-$HOME/.codex}/skills`,
  `${CLAUDE_HOME:-$HOME/.claude}/skills`

## External Dependencies

- Required for normal operation: `bash`, `jq`
- Required per agent choice: `codex` or `claude`
- Optional for swarm mode: `ntm`, `tmux`

## Constraints

- The wrapper is intentionally bounded; it is not allowed to replace the HELIX
  loop with an unconditional `while true`.
- Tracker readiness is dependency-aware.
- Experiment mode must refuse dirty worktrees.
- Backfill must fail if the report trailer is missing or the declared report
  file does not exist.
- `WAIT` is a terminal orchestration result, not a cue to try another
  implementation pass.
- Recovery must preserve unrelated local changes and may not rely on process
  absence alone.

## Evidence

- `scripts/helix:15-37`
- `scripts/helix:109-239`
- `scripts/helix:250-359`
- `scripts/helix:381-519`
- `scripts/helix:542-570`
- `scripts/helix:579-784`
- `scripts/install-local-skills.sh:4-67`
- `tests/helix-cli.sh:46-176`
- `tests/helix-cli.sh:347-414`
- `tests/helix-cli.sh:571-646`

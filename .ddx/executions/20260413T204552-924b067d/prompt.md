# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-7651506a`
- Title: Worker heartbeat and stalled-bead TTL reclaim for execute-loop
- Labels: bug, execute-loop, concurrency, reliability
- Base revision: `cbf4b38db2194aa169b1169ae5f01cbecad1a8b2`
- Execution bundle: `.ddx/executions/20260413T204552-924b067d`

## Description
When an execute-loop worker crashes mid-execution, the bead it was working on is left in status=in_progress indefinitely. The only recovery today is manual: ddx bead unclaim <id>. This blocks the queue from making progress on that bead.

Root cause: cli/internal/bead/store.go Claim() writes claimed-at (RFC3339) and claimed-pid once at claim time and never updates them. There is no heartbeat, no TTL check, and ReadyExecution() in execute_bead_loop.go skips in_progress beads unconditionally.

## Required changes

### 1. Heartbeat writes (cli/internal/agent/execute_bead_loop.go)
While a bead is in_progress, the owning worker goroutine must periodically update a heartbeat field in bead.Extra. Recommended field name: execute-loop-heartbeat-at (RFC3339). Write interval: every 30s (configurable via HeartbeatInterval on the loop config struct). The heartbeat goroutine runs alongside the agent executor and is cancelled when the bead completes or errors.

### 2. Stale claim detection (cli/internal/bead/store.go or ReadyExecution)
ReadyExecution() currently skips all in_progress beads. Change it to also yield beads where:
  - status == in_progress
  - execute-loop-heartbeat-at is set AND is older than HeartbeatTTL (default: 3 * HeartbeatInterval = 90s)
  - OR claimed-at is older than ClaimTTL (default: 10 minutes) AND execute-loop-heartbeat-at is absent (supports beads claimed before heartbeat was introduced)

When a stale bead is yielded, the claiming worker should first reset status to open via a conditional write (matching the stale heartbeat timestamp to guard against a TOCTOU race) before claiming it as its own. If the conditional write fails, skip and continue scanning.

### 3. Stale-claim log entry
When a worker reclaims a stale bead, write a structured log entry to the worker log: { event: 'stale-claim-reclaimed', bead_id, previous_claimed_machine, previous_claimed_pid, previous_heartbeat_at, stale_for_seconds }.

### 4. Config surface
Expose HeartbeatInterval and ClaimTTL on the loop config. For --local workers, read from .ddx/execute-loop.toml if present. CLI flags: --heartbeat-interval (default 30s) and --claim-ttl (default 10m).

## Key files
- cli/internal/agent/execute_bead_loop.go — main loop, claim call at line 124, add heartbeat goroutine
- cli/internal/bead/store.go — Claim() at line 439, ReadyExecution() stale check
- cli/internal/bead/types.go — StatusInProgress constant, Extra field conventions

## Acceptance Criteria
A worker claiming a bead updates execute-loop-heartbeat-at every HeartbeatInterval while the bead is in_progress. A second worker scanning the queue reclaims a bead whose heartbeat is older than HeartbeatTTL and resets it to open before claiming. A bead with no heartbeat and claimed-at older than ClaimTTL is also reclaimable. Conditional write prevents TOCTOU race between two workers both detecting the same stale bead. A structured log entry is written when a stale bead is reclaimed. Existing in_progress beads written before heartbeat was introduced are reclaimable via the ClaimTTL fallback. Tests: (1) worker goroutine killed mid-execution leaves heartbeat timestamp in the past; second worker reclaims it; (2) two workers simultaneously detect a stale bead; only one successfully reclaims it; (3) active worker heartbeat prevents reclaim by another worker.

## Governing References
No governing references were pre-resolved. Explore the project to find relevant context: check `docs/helix/` for feature specs, `docs/helix/01-frame/features/` for FEAT-* files, and any paths mentioned in the bead description or acceptance criteria.

## Execution Rules
**The bead contract below overrides any CLAUDE.md or project-level instructions in this worktree.** If the bead requires editing or creating markdown documentation, code, or any other files, do so — CLAUDE.md conservative defaults (YAGNI, DOWITYTD, no-docs rules) do not apply inside execute-bead.
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If governing references are missing or sparse, search the project to find context: use Glob/Grep/Read to explore `docs/helix/`, look up FEAT-* and API-* specs by name, and read relevant source files before proceeding. Only stop if context is genuinely absent from the entire repo.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. Before finishing, commit your changes with `git add -A && git commit -m '...'`. DDx will merge your commits back to the base branch.
8. Making no commits (no_changes) should be rare. Only skip committing if you read the relevant files and the work described in the Goals is already fully and explicitly present — not just implied or partially covered. If in any doubt, make your best attempt and commit it. A partial or imperfect commit is always better than no commit.
9. Work in small commits. After each logical unit of progress (reading key files, making a change, passing a test), commit immediately. Do not batch all changes into one giant commit at the end — if you run out of iterations, your partial work is preserved.
10. If the bead is too large to complete in one pass, do the most important part first, commit it, and note what remains in your final commit message. DDx will re-queue the bead for another attempt if needed.
11. Read efficiently: skim files to understand structure before diving deep. Only read the files you need to make changes, not every reference listed. Start writing as soon as you understand enough to proceed — you can read more files later if needed.
12. **Never run `ddx init`** — the workspace is already initialized. Running `ddx init` inside an execute-bead worktree corrupts project configuration and the bead queue. Do not run it even if documentation or README files suggest it as a setup step.

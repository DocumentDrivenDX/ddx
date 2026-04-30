---
ddx:
  id: FEAT-023
  depends_on:
    - helix.prd
    - FEAT-001
    - FEAT-004
    - FEAT-010
---
# Feature: Multi-Machine Sync

**ID:** FEAT-023
**Status:** Implemented
**Priority:** P1
**Owner:** DDx Team

## Overview

`ddx sync` is a first-class CLI command that synchronizes DDx-managed files
(`.ddx/beads.jsonl`, `.ddx/executions/`, `.ddx/plugins/`) with `origin/main`
in a single canonical flow. It exists in both one-shot and watch (daemon)
modes and is constrained to never touch any path outside the DDx-managed
allowlist.

Sync solves the multi-machine drift problem operators see when DDx is checked
out on more than one machine: workers churn `beads.jsonl` every few minutes
during execute-loop drains, the working tree is constantly dirty in tracked
paths, and manually pulling/stashing/popping/committing/pushing is friction
that gets skipped, causing tracker divergence.

## Problem Statement

**Current situation:** Operators with DDx on multiple machines manually run
the sequence `git fetch && git stash && git pull && git stash pop && git add
.ddx/beads.jsonl && git commit -m "tracker" && git push` (with execution
evidence variants) several times per session to keep tracker state aligned.
A 2026-04-29 dogfood session ran this flow ~6 times in ~3 hours.

**Pain points:**

- Manual sync is constant friction; missing a sync risks divergent tracker
  state between machines.
- No remote-only solution exists: cloud routines cannot touch a local
  working tree, and a server-side sync only helps if every endpoint is the
  server (DDx is per-project local).
- Operators forget to sync, then have to reconcile divergent
  `beads.jsonl` lines by hand.

**Desired outcome:** A single `ddx sync` keystroke (or zero, with
`--watch`) performs the canonical flow safely. Anything that cannot be
auto-resolved — stash-pop conflict, double push failure, divergent base —
aborts cleanly with a structured non-zero exit, writes a failure record,
and surfaces through `ddx doctor` so the operator notices without having
to remember to look.

## Scope

### In Scope

- `ddx sync` one-shot command running the canonical flow:
  1. `git fetch origin`
  2. Stash tracked dirty files in the DDx-managed allowlist (untracked
     additions are not stashed; merge does not touch them).
  3. `git merge origin/main` (no rebase — preserves execute-bead history
     per the project's standing merge policy).
  4. `git stash pop` if a stash was created.
  5. Commit DDx-managed dirty paths with structured messages:
     `.ddx/beads.jsonl` → `chore: tracker`;
     `.ddx/executions/` and `.ddx/plugins/` → `chore: add execution evidence`.
  6. `git push origin main`; on non-fast-forward, retry from step 1 once.
- `ddx sync --watch [--interval=15m]` running the same flow on an
  interval until interrupted. Local-only, foreground process; the operator
  is responsible for backgrounding.
- Strict allowlist enforcement: only `.ddx/beads.jsonl`,
  `.ddx/executions/`, and `.ddx/plugins/` are ever stashed, staged, or
  committed. Unrelated dirty changes survive untouched across the entire
  flow.
- No destructive flags: sync never invokes git with `--force`,
  `--no-verify`, `--hard`, or any other destructive option.
- Structured abort: when any step cannot be auto-resolved (stash-pop
  conflict, double push fail, fetch/merge failure), sync exits non-zero,
  writes `.ddx/sync-failure.json` containing timestamp and reason, and
  leaves the working tree in a state the operator can inspect.
- Doctor integration: `ddx doctor` reads `.ddx/sync-failure.json` (when
  present) and reports a `sync_aborted` diagnostic with timestamp, reason,
  and remediation steps.
- Cross-platform: works on macOS and Linux. No symlinks, no shell-isms;
  implemented in Go using the existing `cli/internal/git` shell-out
  helpers.

### Out of Scope

- Conflict resolution beyond stash-pop (the preserved-iteration problem
  is tracked separately in ddx-0097af14).
- Sync of paths outside the DDx-managed allowlist — by design, sync
  never touches user code.
- Multi-remote sync. v1 supports a single `origin` only.
- Server-loop integration: when ddx-server runs continuously per-project,
  it can call `ddx sync` as part of its loop. The hook is not part of v1
  but the command is designed to be re-entrant and safe for that use.

## Acceptance Criteria

1. `ddx sync` exists and runs the canonical flow described above.
2. `ddx sync --watch [--interval=15m]` runs the flow on the configured
   interval until killed.
3. Both modes refuse to touch paths outside the DDx-managed allowlist
   (`.ddx/beads.jsonl`, `.ddx/executions/`, `.ddx/plugins/`). Verified by
   tests that inject unrelated dirty changes and assert they survive
   untouched.
4. Both modes never invoke git with `--force`, `--no-verify`, or any
   destructive flag. Verified by tests asserting on the captured shell
   args.
5. Stash-pop conflict and double push failure abort with a structured
   non-zero exit and write `.ddx/sync-failure.json`. Covered by tests
   using a fake git runner.
6. `ddx doctor` surfaces a recent sync failure as a `sync_aborted`
   diagnostic when `.ddx/sync-failure.json` is present.
7. Tests run green on macOS and Linux.

## Implementation Notes

The command lives in `cli/cmd/sync.go` with tests in `cli/cmd/sync_test.go`.
Git invocations route through a `syncGitRunner` function type so tests can
inject a fake without spawning real git processes. The allowlist is encoded
as a package-level `ddxManagedPaths` slice; every status check, stash, add,
and commit references that slice rather than building paths ad hoc.

`ddx doctor` integration is via `checkSyncFailure(failurePath)` which reads
the JSON record (if any) and returns a `DiagnosticIssue` with remediation
guidance.

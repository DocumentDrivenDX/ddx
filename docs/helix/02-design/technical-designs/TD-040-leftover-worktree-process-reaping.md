---
ddx:
  id: TD-040
  depends_on: []
  status: draft
---
# Technical Design: Leftover-Worktree-Keyed Helper-Process Reaping

## Status

Draft. Written for ddx-a9218e18 (epic: leak-proof helper-process reaping).
No implementation lands in this document; it authorizes the child beads
listed at the end.

## Motivation

Operators have had to manually hunt and kill leaked helper processes left
behind by `ddx work` runs — observed cases include orphan `find` processes
23+ days old and orphan harness processes (source: session-harvest-2026-06-27).
The naive fix — teaching the existing harness-cmdline allowlist to also
recognize `find` — does not work, because the allowlist model cannot
attribute a bare `find` invocation to a bead or worktree at all. This
document specifies detection on **worktree residency**, not command name.

## Existing Mechanisms (as-built)

There are already two independent reaping paths in `cli/internal/agent`.
Both matter to this design: the first shows why a cmdline allowlist is a
dead end, and the second is the correct foundation to build on — it already
does most of what the epic asks for, but has an attribution gap.

### 1. `orphan_harness_reaper.go` — cmdline-allowlist reaper

`reapOrphanedHarnessChildren` (orphan_harness_reaper.go:45-153) only acts on
a process when all three hold:

- `proc.PPID == 1` (reparented to init) — orphan_harness_reaper.go:82.
- `looksLikeHarnessProcess(proc.Command)` matches a fixed argv[0] allowlist
  (`claude|codex|gemini|opencode|pi`) — orphan_harness_reaper.go:155-169.
- `workerstatus.InferBead(proc.Command, proc.Cwd)` returns both a bead ID and
  a worktree — orphan_harness_reaper.go:88-91.

The harness runs in its own process group (`Setpgid`), and the reaper kills
that whole group (`killGroup(proc.PID)`, orphan_harness_reaper.go:108), so a
`find` still inside the harness's group already dies when the harness is
reaped. A `find` (or any other helper) that **escaped the group** —
double-forked, or spawned by something other than the harness itself — has
no harness cmdline to match and no way to be PPID-attributed to one. Adding
`find` to `looksLikeHarnessProcess` would never fire, because the function
gates on `proc.Command`, and an escaped `find`'s cmdline is just `find ...`,
not a harness binary. This path is structurally unable to reach the reported
class of leak.

### 2. `execution_cleanup_process.go` — attempt-process census (closer, but gapped)

`ExecutionCleanupManager.CleanupAttemptProcesses` (execution_cleanup_process.go:58-82)
already does most of what this epic wants: it does not use a cmdline
allowlist at all. It scans `/proc`, groups processes by process group ID
(`groupExecutionCleanupAttemptProcesses`, execution_cleanup_process.go:334-373),
and classifies each group by **worktree**, derived from `cwd` via
`executionCleanupAttemptProcessFromWorkerStatus` (execution_cleanup_process.go:415-437):

- Primary path: `workerstatus.InferBead(cmdline, cwd)` extracts any substring
  containing the `.execute-bead-wt-*` segment, then
  `executionCleanupAttemptWorktreeRoot` (execution_cleanup_process.go:387-409)
  truncates that string at the first path separator following the prefix —
  so a cwd or cmdline token nested arbitrarily deep inside a bead worktree
  (`.../.execute-bead-wt-<id>/some/nested/dir`) still resolves to the
  worktree **root**, regardless of the process's cmdline.
- Fallback path (execution_cleanup_process.go:420-426): when no
  `.execute-bead-wt-*` segment is present anywhere, but `cwd` is inside the
  configured `tempRoot`, `Worktree` is set to `filepath.Clean(trimmedCwd)` —
  the **raw cwd, with no root truncation**.

Classification (`classifyExecutionCleanupAttemptProcessGroup`,
execution_cleanup_process.go:201-332) then requires an **exact** match on
that `Worktree` value: either `ReadExecutionCleanupMetadata(proc.Worktree)`
finds a metadata file literally at `<Worktree>/<metadata-filename>`
(execution_cleanup.go:1135), or, on `os.ErrNotExist`,
`matchingRunStateForMeta` (execution_cleanup.go:877-900) does a
`filepath.Clean` **string-equality** comparison against each known
`RunState.WorktreePath` (execution_cleanup.go:895). There is no
prefix/contains matching anywhere in this chain.

**Gap:** the fallback path is only exercised for cwd's that do not contain a
`.execute-bead-wt-*` segment (e.g. a flat scratch/execution directory under
`tempRoot` with no such prefix in its name — see the existing regression
`TestStaleAttemptProcessScanner_DetectsReparentedDescendantsByCwd`,
execution_cleanup_process_test.go:371-407, whose fixture cwd is
`tempRoot/scratch-2026-orphan`, a single path segment). If a helper process
in that category has a cwd **nested inside** such a directory
(`tempRoot/scratch-2026-orphan/sub/dir`, e.g. it `chdir`'d deeper before its
parent died, or was invoked with a subdirectory as an argument that became
its cwd), the fallback assigns the full nested path as `Worktree` with no
truncation. Neither the metadata-file lookup nor the exact `RunState`
match can succeed against that nested path, so the process falls through to
`preserved_uncertain_attempt_process` (execution_cleanup_process.go:266-272)
and is never classified stale — it survives every cleanup pass indefinitely.
This is a plausible mechanism for the reported 23-day-old orphan `find`
processes: `find` is frequently invoked with a subdirectory as its search
root, which becomes its cwd only if the shell that spawned it had already
descended into that subdirectory.

A second, narrower detection gap exists one layer earlier: the scanner
(`procExecutionCleanupAttemptProcessScanner.inspect`,
execution_cleanup_process_linux.go:57-74) drops a process outright when
`os.Readlink("/proc/<pid>/cwd")` fails or returns empty
(execution_cleanup_process_linux.go:63,70-72) — e.g. a permission race, or a
cwd whose backing directory has since been removed such that the kernel
cannot resolve the symlink target. Such a process is invisible to worktree
attribution today even if it still holds open file descriptors inside a
leased worktree.

## Design: harden the existing worktree-keyed scan

The correct model already exists in principle — key off worktree residency,
not cmdline — so this design does not propose a new mechanism. It hardens
the two attribution gaps identified above.

### Change 1 — root-normalize the fallback cwd path

In the fallback branch of `executionCleanupAttemptProcessFromWorkerStatus`
(execution_cleanup_process.go:420-426), a cwd inside `tempRoot` must resolve
to the same **root** that owns it, not the raw cwd. The owning root is the
first path component directly under `tempRoot` (mirroring
`executionCleanupAttemptWorktreeRoot`'s truncate-at-next-separator rule, but
anchored at `tempRoot` instead of at the `.execute-bead-wt-*` prefix):
truncate `trimmedCwd` to `filepath.Join(tempRoot, firstComponentAfterTempRoot)`
before assigning `Worktree`. This makes a process nested at any depth under
a scratch/execution root resolve to the same `Worktree` value that
`ReadExecutionCleanupMetadata` and `matchingRunStateForMeta` already expect,
without changing either of those two lookup functions.

### Change 2 — add an open-fd attribution fallback

When cwd-based attribution (both the primary and the Change-1 fallback)
yields an empty `Worktree` — because `cwd` could not be read, or points
outside every known root — attempt a second, independent signal: enumerate
`/proc/<pid>/fd/*`, resolve each symlink target, and apply the same
root-truncation rule used above to the first target that resolves inside
`tempRoot`. A process can still hold an open file handle rooted in its
original worktree even after `cwd` has become unreadable or has changed.
This is strictly additive: cwd-based attribution keeps priority, and a
process with no cwd signal and no matching fd signal remains unclassified
(never killed), which preserves today's conservative default.

## Safety model (unchanged, restated for this design)

- Attribution alone never authorizes a kill. A process is only reaped after
  it is attributed to a worktree **and** that worktree is confirmed not
  live: not in the `registered` set of active worktrees
  (execution_cleanup_process.go:230-240, 254-263) and not passing
  `probe.IsLive` against matched metadata/run-state
  (execution_cleanup_process.go:289-297). Neither change above touches this
  liveness gate.
- A worktree with no run-state and no cleanup metadata match remains
  `preserved_uncertain_attempt_process` and is left alone — attribution
  failure must fail closed, not open.
- Reaping kills the whole process **group** (`KillGroup`, backed by
  `killProcessGroup` / `syscall.Kill(-pid, SIGKILL)`,
  orphan_harness_reaper_linux.go:109-114), consistent with the existing
  group-kill semantics, so a helper's own descendants are swept together in
  one action.
- Never act on PID 1 or on a resolved PGID `<= 0`.

## Linux-specificity

All process introspection here is `/proc`-based and already lives behind
`//go:build linux` files (`execution_cleanup_process_linux.go`,
`orphan_harness_reaper_linux.go`), with `_other.go` no-op counterparts
(`execution_cleanup_process_other.go`, `orphan_harness_reaper_other.go`) for
non-Linux platforms. The fd-based fallback (Change 2) follows the same
split — it has no cross-platform equivalent in this design, matching current
behavior where non-Linux platforms report `attempt_process_cleanup_unavailable`
rather than reaping.

## Composition with existing group-kill + `execution_cleanup_loop.go`

Both changes are internal to the existing attempt-process census; they add
no new loop, lock, or schedule. `CleanupAttemptProcesses` is already invoked
as part of `ExecutionCleanupManager.Cleanup()`, which `runExecutionCleanupPass`
(execution_cleanup_loop.go:27-97) already runs under the project-level
cleanup lock, on the existing jittered periodic interval and shutdown pass
(`startExecutionCleanupWorker`, execution_cleanup_loop.go:185-230). No new
wiring is needed at the loop layer — only the attribution logic inside
`execution_cleanup_process.go` and its Linux scanner changes.

## Non-Scope

- No Fizeau routing changes.
- No implementation in this bead (design + child beads only).
- No cross-platform (macOS/Windows) reaping — remains a documented no-op, as
  it is today.
- No change to the cmdline-allowlist reaper's semantics
  (`orphan_harness_reaper.go`) — it stays as an independent, narrower path.
- No change to the liveness/registered-worktree safety gates.

## Child Beads

- Change 1 (root-normalize the fallback cwd path) and Change 2 (open-fd
  attribution fallback) are filed as separate child beads under this epic
  (`ddx-a9218e18`), each with command-based acceptance criteria, so they can
  land and be verified independently.

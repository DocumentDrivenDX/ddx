# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-9a5df084`
- Title: Fix pre-push no-debug hook regex compatibility and failure semantics
- Labels: helix, phase:build, area:tooling, area:quality
- spec-id: `FEAT-001`
- Base revision: `9a266e06633cdf848f921f0bea082ac8591436af`
- Execution bundle: `.ddx/executions/20260411T171014-ef29d046`

## Description
<context-digest>
Observed during `git push` in /Users/erik/Projects/ddx after landing tracker commits for execute-loop alpha findings. The DDx pre-push hook `no-debug` invoked `rg` with a look-ahead pattern and emitted a regex parse error, but the hook summary still reported success and the push continued.
</context-digest>

The DDx pre-push `no-debug` hook uses an `rg` pattern that requires PCRE2 look-around support, but it invokes plain `rg` without `--pcre2`. In the current environment that produces a parse error while the hook still exits successfully.

## Reproduction
1. In `/Users/erik/Projects/ddx`, run `git push`.
2. Observe pre-push hook output from `no-debug` containing:
   `rg: regex parse error: ... look-around ... is not supported`
3. Observe the hook summary still marks `no-debug` as passed and the push continues.

## Why this matters
The hook currently gives a false sense of enforcement. A broken regex should either be written in ripgrep-compatible syntax or use `--pcre2`, and failures in the debug scan should not be silently treated as success.

## Acceptance Criteria
The DDx pre-push `no-debug` hook runs without regex parse errors in a default ripgrep environment; the debug-scan command fails closed when the scan itself errors; and the hook still blocks actual debug statements it is meant to catch.

## Governing References
- `FEAT-001` — `docs/helix/01-frame/features/FEAT-001-cli.md` (Feature: DDx CLI)

## Execution Rules
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If the bead is missing critical context or the governing references conflict, stop and report the gap explicitly instead of improvising hidden policy.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. DDx owns landing and preservation. Agent-created commits are optional; coherent tracked edits in the worktree still count as produced work.
8. If you choose to create commits, keep them coherent and limited to this bead. If you leave tracked edits without commits, DDx will still evaluate them.
9. If the work is already satisfied with no tracked changes needed, stop cleanly and let DDx record a no-change attempt instead of inventing a commit.

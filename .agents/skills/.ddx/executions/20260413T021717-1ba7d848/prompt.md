# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-45f6d738`
- Title: Detect execute-bead worktree edits even when the agent makes no commits
- Labels: helix, area:exec, area:git, area:agent, kind:correctness
- Base revision: `3ffced524c5258a68ccc31cf3527366ef9ffa7ef`
- Execution bundle: `.ddx/executions/20260413T021717-1ba7d848`

## Description
A Codex execute-loop run on ddx-28c15f0e updated FEAT-002, SD-019, and website server docs inside the managed worktree, but DDx classified the attempt as no_changes because it only looked for agent-created commits. Execute-bead must treat a dirty worktree with tracked edits as real output, preserve or land it appropriately, and record accurate result status instead of discarding useful work.

## Acceptance Criteria
execute-bead and execute-loop no longer report no_changes when the worktree contains tracked file modifications without agent-created commits; the attempt preserves or lands the changes and result artifacts reflect the actual diff state

## Governing References
No governing references were resolved from the current execution snapshot.

## Execution Rules
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If the bead is missing critical context or the governing references conflict, stop and report the gap explicitly instead of improvising hidden policy.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Complete the requested implementation and any local checks the bead contract requires. DDx will classify the final outcome.

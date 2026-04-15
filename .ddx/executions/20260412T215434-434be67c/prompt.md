# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-dae12e46`
- Title: Materialize local skill links correctly inside execute-bead worktrees
- Labels: helix, area:exec, area:skills, kind:correctness
- Base revision: `1ef87598ea3d0260c7bc0b7f32d8907f67d69a7f`
- Execution bundle: `.ddx/executions/20260412T215434-434be67c`

## Description
Managed execute-bead worktrees still emit repeated skill-loader errors for missing helix skill symlink targets under .agents/skills, which pollutes stderr and can change harness behavior. The execution worktree should preserve valid local skill resolution or suppress broken project-local links deterministically.

## Acceptance Criteria
execute-bead worktrees no longer emit repeated failed-to-stat errors for project-local skill symlinks, and local skills resolve consistently inside managed worktrees

## Governing References
No governing references were resolved from the current execution snapshot.

## Execution Rules
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If the bead is missing critical context or the governing references conflict, stop and report the gap explicitly instead of improvising hidden policy.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. Before finishing, commit your changes with `git add -A && git commit -m '...'`. DDx will merge your commits back to the base branch.
8. If the work is already satisfied with no tracked changes needed, stop cleanly and let DDx record a no-change attempt.
9. Work in small commits. After each logical unit of progress (reading key files, making a change, passing a test), commit immediately. Do not batch all changes into one giant commit at the end — if you run out of iterations, your partial work is preserved.
10. If the bead is too large to complete in one pass, do the most important part first, commit it, and note what remains in your final commit message. DDx will re-queue the bead for another attempt if needed.
11. Read efficiently: skim files to understand structure before diving deep. Only read the files you need to make changes, not every reference listed. Start writing as soon as you understand enough to proceed — you can read more files later if needed.

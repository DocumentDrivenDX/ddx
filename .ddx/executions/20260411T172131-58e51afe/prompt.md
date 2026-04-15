# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-dae12e46`
- Title: Materialize local skill links correctly inside execute-bead worktrees
- Labels: helix, area:exec, area:skills, kind:correctness
- Base revision: `892567606f4ed55b0ae4f343271c416d703cbf7f`
- Execution bundle: `.ddx/executions/20260411T172131-58e51afe`

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
7. DDx owns landing and preservation. Agent-created commits are optional; coherent tracked edits in the worktree still count as produced work.
8. If you choose to create commits, keep them coherent and limited to this bead. If you leave tracked edits without commits, DDx will still evaluate them.
9. If the work is already satisfied with no tracked changes needed, stop cleanly and let DDx record a no-change attempt instead of inventing a commit.

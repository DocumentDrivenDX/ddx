# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-dae12e46`
- Title: Materialize local skill links correctly inside execute-bead worktrees
- Labels: helix, area:exec, area:skills, kind:correctness
- Base revision: `613426cae5846702393a081cf0acfd79e1c08dc1`
- Execution bundle: `.ddx/executions/20260413T002116-867e31a3`

## Description
Managed execute-bead worktrees still emit repeated skill-loader errors for missing helix skill symlink targets under .agents/skills, which pollutes stderr and can change harness behavior. The execution worktree should preserve valid local skill resolution or suppress broken project-local links deterministically.

## Acceptance Criteria
execute-bead worktrees no longer emit repeated failed-to-stat errors for project-local skill symlinks, and local skills resolve consistently inside managed worktrees

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
8. Before concluding no changes are needed, explicitly verify each criterion by quoting the exact text from the relevant file that satisfies it. If you cannot quote it directly, the criterion is not yet met — make the edit. Only stop with no commits if every criterion is provably satisfied by existing content.
9. Work in small commits. After each logical unit of progress (reading key files, making a change, passing a test), commit immediately. Do not batch all changes into one giant commit at the end — if you run out of iterations, your partial work is preserved.
10. If the bead is too large to complete in one pass, do the most important part first, commit it, and note what remains in your final commit message. DDx will re-queue the bead for another attempt if needed.
11. Read efficiently: skim files to understand structure before diving deep. Only read the files you need to make changes, not every reference listed. Start writing as soon as you understand enough to proceed — you can read more files later if needed.

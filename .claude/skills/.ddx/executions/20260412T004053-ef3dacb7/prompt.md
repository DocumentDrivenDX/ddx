# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-063d34f0`
- Title: Define epic worker branch, worktree, and merge-gate contract
- Parent: `ddx-cf340665`
- Labels: ddx, phase:planning, kind:architecture, area:agent, area:bead, area:git, area:docs
- spec-id: `FEAT-006`
- Base revision: `1107a8064fd1b4e665ac36ad7d9dcdb1fc408733`
- Execution bundle: `.ddx/executions/20260412T004053-ef3dacb7`

## Description
<context-digest>
Review area: epic-scoped worker behavior. Evidence covers the new single-ticket-first policy, the need for one persistent epic branch/worktree, sequential child execution, and epic-level merge gates before the final merge commit.
</context-digest>
Define the worker, branch, worktree, and merge-gate contract for epic execution.

## Goals
- Define epic branch naming and lifecycle
- Define how child beads are selected and committed sequentially on the epic branch
- Define when child beads close relative to epic-branch commits
- Define epic merge-gate execution and the final regular merge-commit landing contract

## Acceptance Criteria
FEAT-006, FEAT-012, API-001, and SD-019 define epic branch naming, persistent epic worktrees, sequential child-bead commits, child close semantics, epic merge gates, and final merge-commit landing

## Governing References
- `FEAT-006` — `docs/helix/01-frame/features/FEAT-006-agent-service.md` (Feature: DDx Agent Service)

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

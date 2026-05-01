# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-c9ebfa1c`
- Title: Define server and UI surfaces for epic execution
- Parent: `ddx-cf340665`
- Labels: ddx, phase:planning, kind:architecture, area:server, area:ui, area:api, area:docs
- spec-id: `FEAT-008`
- Base revision: `5cfe2cce7334549971e5e62113063748b4deb8ca`
- Execution bundle: `.ddx/executions/20260411T202820-f75728b1`

## Description
<context-digest>
Review area: server and UI epic execution surfaces. Evidence covers the host+user server plan, worker list requirements, queue readiness UI, and the need to show epic workers separately from ordinary single-ticket execute-loop workers.
</context-digest>
Define the server/API/UI surfaces for epic execution.

## Goals
- Define the separate epic lane in queue views
- Define epic worker records, branch/worktree visibility, and child-progress visibility
- Define how users inspect epic merge-gate results and the final merge candidate
- Keep single-ticket workers and epic workers distinct in the server model

## Acceptance Criteria
FEAT-008 and SD-019 define a separate epic lane, epic worker visibility, branch/worktree inspection, child-progress visibility, and epic merge-gate inspection distinct from ordinary single-ticket execute-loop surfaces

## Governing References
- `FEAT-008` — `docs/helix/01-frame/features/FEAT-008-web-ui.md` (Feature: DDx Server Web UI)

## Execution Rules
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If the bead is missing critical context or the governing references conflict, stop and report the gap explicitly instead of improvising hidden policy.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. Before finishing, commit your changes with `git add -A && git commit -m '...'`. DDx will merge your commits back to the base branch.
8. If the work is already satisfied with no tracked changes needed, stop cleanly and let DDx record a no-change attempt.

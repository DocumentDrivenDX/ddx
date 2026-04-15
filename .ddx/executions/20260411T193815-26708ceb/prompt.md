# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-cadb3677`
- Title: Add structured progress output to execute-loop
- Labels: ddx, phase:build, kind:implementation, area:agent, area:cli
- Base revision: `6cc411fc1bcc3da90a00b65df4e8c9948545193d`
- Execution bundle: `.ddx/executions/20260411T193815-26708ceb`

## Description
Execute-loop currently provides minimal output. Add structured progress that: 1) Names the bead being processed at loop start, 2) Reports agent output chunks/iterations as they happen, 3) Summarizes final result (merged/preserved/error), 4) All output goes to structured logging that ddx server can capture for background runs.

## Acceptance Criteria
No explicit acceptance criteria were recorded on the bead.

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

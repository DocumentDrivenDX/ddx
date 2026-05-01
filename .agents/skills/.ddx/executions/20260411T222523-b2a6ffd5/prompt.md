# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-e1548fe2`
- Title: Consume published ddx-agent catalog manifest
- Labels: area:agent, area:routing, phase:build
- Base revision: `395e6ac9d063f5f8dc70d227403b54f91f8e0671`
- Execution bundle: `.ddx/executions/20260411T222523-b2a6ffd5`

## Description
DDx still carries builtin shared-catalog fallback behavior while ddx-agent is moving toward a published versioned manifest and explicit local install path. Update DDx so it can consume the same installed external manifest that ddx-agent uses, prefer that shared file when present, and reduce or remove stale duplicate builtin catalog tables once the shared distribution path is available.

This bead depends on the ddx-agent-side publication and update work landing first. The DDx routing contract should continue to treat the shared catalog as authoritative for aliases, profiles, canonical targets, and surface projections.

## Acceptance Criteria
1. DDx can read the shared externally installed ddx-agent manifest file when configured or discovered at the standard path.
2. DDx routing and capability reporting use the shared manifest content rather than stale duplicate builtin tables when that file is available.
3. DDx retains a deterministic fallback only for the cases explicitly authorized by design, and that fallback is documented.
4. Tests cover shared-manifest consumption and compatibility with the refreshed code-high, code-medium, and code-economy tiers.

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

# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-cb621399`
- Title: Specify ddx server launchd install contract for macOS
- Parent: `ddx-8b6cd40e`
- Labels: helix, area:server, area:ops, area:macos, kind:architecture
- Base revision: `a0e953dfdaeb581d532e84c934ad923d5f9513b8`
- Execution bundle: `.ddx/executions/20260411T222147-ba70466e`

## Description
Specify the macOS launchd lifecycle for ddx server so the service-manager contract is portable even though only the Linux implementation ships now.

## Acceptance Criteria
The server specs define the launchd plist path, working directory, logs/state locations, environment overrides, and lifecycle expectations for future macOS implementation

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

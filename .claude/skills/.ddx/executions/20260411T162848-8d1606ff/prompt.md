# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-cba2dc64`
- Title: Redirect embedded agent logs out of execute-bead worktree root
- Parent: `ddx-32b3008e`
- Labels: helix, phase:planning, kind:architecture, area:agent, area:exec, area:docs
- spec-id: `FEAT-006`
- Base revision: `47468372332d828a6cf55cd468bc7c5e8e857a6a`
- Execution bundle: `.ddx/executions/20260411T162848-8d1606ff`

## Description
<context-digest>
Review area: embedded ddx-agent runtime ownership inside execute-bead worktrees. Evidence covers the tracked execution-evidence lane, the current rule that DDx should own execution artifacts for managed runs, and the need to prevent embedded-agent session/telemetry files from appearing as ad hoc root-level state inside execution worktrees.
</context-digest>
Define and implement the path contract for embedded ddx-agent logs and telemetry when DDx runs the embedded harness inside execute-bead and execute-loop.

## Goals
- Prevent embedded ddx-agent session logs, traces, or telemetry from being written as default root-level runtime state in managed execution worktrees
- Redirect embedded-agent runtime output into DDx-owned execution-evidence or runtime paths
- Keep the split explicit between tracked execution evidence and ignored local runtime scratch
- Update the governing specs so embedded-agent log ownership is documented rather than implied by implementation

## Required spec work
- Update FEAT-006 and the execution-evidence design lane to define where embedded-agent logs/telemetry live during execute-bead runs
- Clarify how embedded-agent-owned logs differ from external provider-native logs

## Required implementation work
- Configure the embedded agent harness so managed execute-bead worktrees do not accumulate root-level agent state
- Add tests proving embedded-agent runtime output is redirected into DDx-owned paths

## Acceptance Criteria
FEAT-006 and the execution-evidence design docs define embedded-agent runtime paths for managed execute-bead runs; the embedded harness no longer writes default root-level agent state inside execution worktrees; and tests prove session/telemetry output is redirected into DDx-owned evidence or runtime directories

## Governing References
- `FEAT-006` — `docs/helix/01-frame/features/FEAT-006-agent-service.md` (Feature: DDx Agent Service)

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

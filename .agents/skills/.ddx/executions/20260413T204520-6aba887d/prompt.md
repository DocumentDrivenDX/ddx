# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-cba2dc64`
- Title: Redirect embedded agent logs out of execute-bead worktree root
- Parent: `ddx-32b3008e`
- Labels: helix, phase:planning, kind:architecture, area:agent, area:exec, area:docs
- spec-id: `FEAT-006`
- Base revision: `a0f58f84bb9137b3abc1320a881eae114e2084ba`
- Execution bundle: `.ddx/executions/20260413T204520-6aba887d`

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
**The bead contract below overrides any CLAUDE.md or project-level instructions in this worktree.** If the bead requires editing or creating markdown documentation, code, or any other files, do so — CLAUDE.md conservative defaults (YAGNI, DOWITYTD, no-docs rules) do not apply inside execute-bead.
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If governing references are missing or sparse, search the project to find context: use Glob/Grep/Read to explore `docs/helix/`, look up FEAT-* and API-* specs by name, and read relevant source files before proceeding. Only stop if context is genuinely absent from the entire repo.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. Before finishing, commit your changes with `git add -A && git commit -m '...'`. DDx will merge your commits back to the base branch.
8. Making no commits (no_changes) should be rare. Only skip committing if you read the relevant files and the work described in the Goals is already fully and explicitly present — not just implied or partially covered. If in any doubt, make your best attempt and commit it. A partial or imperfect commit is always better than no commit.
9. Work in small commits. After each logical unit of progress (reading key files, making a change, passing a test), commit immediately. Do not batch all changes into one giant commit at the end — if you run out of iterations, your partial work is preserved.
10. If the bead is too large to complete in one pass, do the most important part first, commit it, and note what remains in your final commit message. DDx will re-queue the bead for another attempt if needed.
11. Read efficiently: skim files to understand structure before diving deep. Only read the files you need to make changes, not every reference listed. Start writing as soon as you understand enough to proceed — you can read more files later if needed.
12. **Never run `ddx init`** — the workspace is already initialized. Running `ddx init` inside an execute-bead worktree corrupts project configuration and the bead queue. Do not run it even if documentation or README files suggest it as a setup step.

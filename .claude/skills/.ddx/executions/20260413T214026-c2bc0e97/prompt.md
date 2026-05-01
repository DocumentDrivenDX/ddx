# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-f5337613`
- Title: Add structured embedded-agent progress output for workers and UI
- Parent: `ddx-8b6cd40e`
- Labels: helix, phase:planning, kind:architecture, area:agent, area:api, area:ui, area:docs
- spec-id: `FEAT-006`
- Base revision: `ac7c0db9b9646b7c796c760d60c326b4d472e14b`
- Execution bundle: `.ddx/executions/20260413T214026-c2bc0e97`

## Description
<context-digest>
Review area: embedded ddx-agent runtime observability and live worker progress. Evidence covers FEAT-006 execution/evidence ownership, FEAT-002 server activity surfaces, FEAT-008 web UI expectations, execute-loop worker supervision, and the need for DDx-owned progress telemetry beyond coarse session logs when DDx runs the embedded harness.
</context-digest>
Define the progress-event contract for managed embedded-agent runs so DDx can surface meaningful live execution status in the CLI and server/UI.

## Goals
- Define structured phase/heartbeat output for embedded-agent runs (queueing, launch, running, post-checks, landing, preserved, done)
- Define the minimal per-event fields needed for operator visibility and later reporting: worker/project/bead/run identity, phase, elapsed/heartbeat timing, harness/provider/model/profile, and token/cost deltas when available
- Clarify which live progress artifacts remain runtime-only and which summaries/indexes are exposed through DDx-owned APIs and UI surfaces
- Keep provider-native transcript ownership external; DDx progress output should be normalized runtime telemetry, not transcript duplication

## Required spec work
- Update FEAT-006, FEAT-002, and FEAT-008 so embedded-agent progress visibility is an explicit contract rather than an implementation detail
- Clarify how progress events relate to tracked execution artifacts, session logs, and server-managed worker state

## Required implementation planning
- Define the server/API/UI read model for live worker progress and recent phase history
- Define how CLI and UI consume the same structured progress feed without introducing hidden workflow policy

## Acceptance Criteria
FEAT-006, FEAT-002, and FEAT-008 define a structured embedded-agent progress contract for managed runs; the contract names the required phases and fields, distinguishes runtime-only progress from tracked execution evidence, and defines one shared read model for CLI and server/UI consumers

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

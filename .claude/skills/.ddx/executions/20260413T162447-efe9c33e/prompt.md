# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-79a731b4`
- Title: Use native ddx-agent config for the embedded harness
- Labels: ddx, phase:planning, kind:architecture, area:agent, area:config, area:cli
- spec-id: `FEAT-006`
- Base revision: `0d3d72d48189927b881d9c475ed0396687af0aca`
- Execution bundle: `.ddx/executions/20260413T162447-efe9c33e`

## Description
<context-digest>
Review area: embedded-agent configuration ownership. Evidence covers ddx's partial `agent.agent_runner` mirror in `.ddx/config.yaml`, the native `ddx-agent` config model in `.agent/config.yaml`, and the need to support existing provider/backends such as OpenRouter without duplicating or losing configuration surface.
</context-digest>
Replace DDx's lossy embedded-agent config mirror with native `ddx-agent` configuration loading.

## Goals
- Make `.agent/config.yaml` the authoritative config source for the embedded `agent` harness
- Define precedence between native agent config, environment variables, DDx orchestration config, and CLI overrides
- Remove or narrow DDx-owned embedded-agent config fields that duplicate native runtime config
- Ensure DDx `agent run`, `execute-bead`, and `execute-loop` can consume existing OpenRouter/native agent setups without bespoke DDx translation
- Add regression coverage for embedded-agent config resolution and override semantics

## Acceptance Criteria
The design stack defines native `.agent/config.yaml` as the authoritative embedded-agent config source, documents precedence with environment / `.ddx` orchestration config / CLI overrides, and queues implementation work to remove DDx's lossy embedded-agent config mirror while preserving OpenRouter/native runtime compatibility

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

# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-7be16a5c`
- Title: Implement execute-bead required execution gate enforcement
- Parent: `ddx-91fd7e27`
- Labels: helix, phase:build, kind:correctness, area:agent, area:exec, area:cli
- spec-id: `FEAT-006`
- Base revision: `b9be30558612dd2e11c2382b195d1a848a89eddc`
- Execution bundle: `.ddx/executions/20260413T134203-453c7172`

## Description
<context-digest>
Review area: execute-bead post-run gate enforcement. Evidence covers cli/cmd/agent_execute_bead.go landing behavior, FEAT-006's required execution and ratchet language, FEAT-010 execute-bead compatibility requirements, and the need to consume authored execution documents rather than hidden workflow policy.
</context-digest>
Implement the documented post-run merge-gate behavior in execute-bead.

## Goals
- Resolve applicable graph-authored execution documents from the governing snapshot used for the iteration
- Run the required execution documents after the agent completes and before land
- Preserve instead of land when a required execution fails or a ratchet gate blocks the iteration
- Record gate outcomes in the execute-bead result/evidence surface and through the existing exec history/runtime surfaces
- Add automated coverage for merge-by-default, required gate failure, and successful gate-passing land behavior

## Acceptance Criteria
execute-bead resolves applicable graph-authored execution documents from the governing snapshot, runs required post-run gates before landing, preserves iterations when required executions or ratchets fail, records gate outcomes through existing result/evidence surfaces, and automated coverage proves merge-by-default still works when no required gates fail

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

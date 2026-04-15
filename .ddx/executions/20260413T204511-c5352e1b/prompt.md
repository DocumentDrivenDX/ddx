# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-7a01ba6c`
- Title: Clarify execute-loop close semantics and result statuses in CLI help/docs
- Labels: helix, phase:design, kind:docs, area:cli, area:agent, area:api
- spec-id: `API-001`
- Base revision: `ebffca37bd8b86328593f8def0237ef69cad4869`
- Execution bundle: `.ddx/executions/20260413T204511-c5352e1b`

## Description
The shipped `ddx agent execute-loop` close behavior is clear in code but not in user-facing docs. Operators should not need to read `cli/internal/agent/execute_bead_loop.go` to learn that success closes a bead with evidence, non-success unclaims it, and loop events are recorded with the execute-bead status and preserve metadata when present. Clarify the documented result surface and the exact close/unclaim behavior for each status.

## Acceptance Criteria
DDx docs or CLI help state which `execute-bead` statuses `execute-loop` consumes, that `success` closes the bead with recorded session/commit evidence, that non-success statuses leave the bead open and unclaimed, and what loop-level observability or event metadata operators can expect after each attempt.

## Governing References
- `API-001` — `docs/helix/02-design/contracts/API-001-execute-bead-supervisor-contract.md` (API/Interface Contract: Execute-Bead Supervisor)

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

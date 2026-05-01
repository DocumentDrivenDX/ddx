# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-274a8f4b`
- Title: Define execute-loop no-change disposition and retry policy
- Parent: `ddx-d2930ee4`
- Labels: ddx, phase:planning, kind:architecture, area:agent, area:bead, area:docs
- spec-id: `FEAT-006`
- Base revision: `daa8c015776b60e6f9d7437d9f358e272b589aed`
- Execution bundle: `.ddx/executions/20260413T032603-1fcb52bc`

## Description
<context-digest>
Review area: execute-loop `no_changes` disposition semantics. Evidence covers the fixed `no_changes` status in execute-bead results, repeated queue selection of the same ready bead, merge-gate design work in ddx-40a722c3 / ddx-632792d2, and the need for an explicit rule that determines when a no-change attempt means "already satisfied" versus "still unresolved".
</context-digest>
Define the contract for how execute-loop handles `no_changes` attempts.

## Goals
- Define the satisfaction check that can convert `no_changes` into bead closure without calling the execution itself a success
- Define the unresolved path for `no_changes`, including retry suppression / cooldown and operator-visible status
- Clarify how required execution docs, acceptance validation, and future gate enforcement feed the satisfaction decision
- Define the machine-readable result/evidence fields needed for loop control and UI visibility

## Acceptance Criteria
FEAT-006, API-001, and execute-loop docs define how `no_changes` attempts are adjudicated into already-satisfied close vs unresolved cooldown, including the role of required gates/checks and the operator-visible result surface

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

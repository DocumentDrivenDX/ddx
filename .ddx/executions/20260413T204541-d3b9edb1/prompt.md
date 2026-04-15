# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-063d34f0`
- Title: Define epic worker branch, worktree, and merge-gate contract
- Parent: `ddx-cf340665`
- Labels: ddx, phase:planning, kind:architecture, area:agent, area:bead, area:git, area:docs
- spec-id: `FEAT-006`
- Base revision: `4f53862ddcf4e8807e4c577e1e8fb90e8938bf2c`
- Execution bundle: `.ddx/executions/20260413T204541-d3b9edb1`

## Description
<context-digest>
Review area: epic-scoped worker behavior. Evidence covers the new single-ticket-first policy, the need for one persistent epic branch/worktree, sequential child execution, and epic-level merge gates before the final merge commit.
</context-digest>
Define the worker, branch, worktree, and merge-gate contract for epic execution.

## Goals
- Define epic branch naming and lifecycle
- Define how child beads are selected and committed sequentially on the epic branch
- Define when child beads close relative to epic-branch commits
- Define epic merge-gate execution and the final regular merge-commit landing contract

## Acceptance Criteria
FEAT-006, FEAT-012, API-001, and SD-019 define epic branch naming, persistent epic worktrees, sequential child-bead commits, child close semantics, epic merge gates, and final merge-commit landing

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

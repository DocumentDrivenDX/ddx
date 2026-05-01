# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-cf340665`
- Title: Design epic-scoped execution and merge workflow
- Labels: ddx, phase:planning, kind:architecture, area:agent, area:bead, area:git, area:server, area:ui, area:docs
- spec-id: `FEAT-006`
- Base revision: `39c4a3cbc89ba5d0bb0778fd4ed0919041f75892`
- Execution bundle: `.ddx/executions/20260413T151809-648e5ef9`

## Description
<context-digest>
Review area: epic execution semantics after the execute-loop dogfood pass. Evidence covers the need to prioritize single tickets ahead of epics, keep open epics out of the ordinary execute-loop launch set, execute one epic sequentially in one persistent worktree, preserve child-ticket history on the epic branch, and land completed epics with a regular merge commit after epic-level merge gates pass.
</context-digest>
Define epic-scoped execution and merge behavior across DDx queueing, git, server, and UI surfaces.

## Goals
- Define single-ticket-first queue ordering with open epics excluded from the ordinary execute-loop worker
- Define one persistent branch/worktree per active epic and sequential child execution inside that worktree
- Define how child tickets close individually while their commits accumulate on the epic branch
- Define epic-level merge gates and regular merge-commit landing semantics
- Define the server/UI worker and queue surfaces needed to supervise epic workers alongside ordinary workers

## Acceptance Criteria
The governing docs and queue contain a coherent lane for single-ticket-first queue policy, epic-scoped workers, epic branch/worktree naming, child-bead sequencing, and regular merge-commit landing with epic merge gates

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

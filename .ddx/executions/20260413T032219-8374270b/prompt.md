# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-40a722c3`
- Title: Align execute-bead merge-default and required execution gate contract
- Parent: `ddx-91fd7e27`
- Labels: helix, phase:planning, kind:architecture, area:agent, area:exec, area:docs
- spec-id: `FEAT-006, FEAT-007, FEAT-010, API-001`
- Base revision: `b7bef9b94fd0b12d9c694d917966f5f54c3ce94b`
- Execution bundle: `.ddx/executions/20260413T032219-8374270b`

## Description
<context-digest>
Review area: execute-bead merge eligibility semantics. Evidence covers FEAT-006 step 8-11, FEAT-007 execution-document discovery, FEAT-010 required execution behavior, TD-005 ratchet semantics, and API-001's separation between structural readiness checks and post-run result classification.
</context-digest>
Align the governing docs around one merge-gate contract.

## Goals
- State clearly that execute-bead is merge-by-default after a successful agent run with changes
- State clearly that only explicit execution documents resolved from the governing graph snapshot can block landing
- Preserve the separation between the pre-launch execution-readiness validator and post-run merge-gate evaluation
- Clarify how required executions and ratchets interact with preserve-vs-land decisions

## Acceptance Criteria
FEAT-006, FEAT-007, FEAT-010, and API-001 all describe the same contract: execute-bead lands by default after successful execution; graph-authored execution documents with required merge-blocking semantics are the only documented automatic landing gates; structural readiness validation remains distinct from post-run gate evaluation; and ratchet blocking rules are explicit rather than implied

## Governing References
- `FEAT-006` — `docs/helix/01-frame/features/FEAT-006-agent-service.md` (Feature: DDx Agent Service)
- `FEAT-007` — `docs/helix/01-frame/features/FEAT-007-doc-graph.md` (Feature: Document Dependency Graph)
- `FEAT-010` — `docs/helix/01-frame/features/FEAT-010-executions.md` (Feature: Executions (Definitions and Runs))
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

# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-a6d2da0e`
- Title: align: repo after server-plan evolution
- Labels: helix, kind:planning, action:align, area:docs, area:api, area:agent, area:ui
- spec-id: `product-vision`
- Base revision: `ac7c0db9b9646b7c796c760d60c326b4d472e14b`
- Execution bundle: `.ddx/executions/20260413T215017-924d79d1`

## Description
<context-digest>
Top-down repository alignment review after the host+user ddx-server plan evolved to add a single-instance-per-user daemon model, embedded-agent progress output, provider availability/utilization dashboard, SQLite-backed host runtime/index state, replay-backed fixture strategy, and explicit Playwright coverage requirements for new UI surfaces.
</context-digest>
Run a HELIX alignment pass for the repository so the durable alignment report and live queue reflect the updated server planning stack and any new implementation gaps it creates.

Scope areas: server-and-planning-contracts, agent-execution-and-routing, web-ui-and-playwright-coverage, host-runtime-state-and-storage, tracker-governance.

Known evidence: FEAT-002, FEAT-008, FEAT-013, FEAT-014, SD-019, TP-002, and the newly queued server observability / tracker-context beads.

## Acceptance Criteria
Alignment review complete; all server-plan gaps classified; execution issues created or reused for real uncovered gaps; durable report reconciled to current tracker and planning state

## Governing References
- `product-vision` — `docs/helix/00-discover/product-vision.md` (Product Vision: DDx)

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

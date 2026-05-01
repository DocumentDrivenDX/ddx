# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-ffda4fb5`
- Title: Align host+user ddx-server planning stack and test strategy
- Parent: `ddx-8b6cd40e`
- Labels: helix, phase:planning, kind:architecture, area:api, area:agent, area:ui, area:docs
- spec-id: `FEAT-002`
- Base revision: `86fda11685186fe05a0aa48343c506e6e6704a4e`
- Execution bundle: `.ddx/executions/20260413T204522-89293e6f`

## Description
<context-digest>
Review area: host+user ddx-server planning stack after the server model evolved beyond the earlier multi-project topology draft. Evidence covers FEAT-002, FEAT-008, FEAT-013, FEAT-014, SD-019, TP-002, the fresh-eyes server review, and the updated requirements for a per-user host daemon, localhost HTTP UI, SQLite-backed runtime/index state, embedded-agent progress visibility, provider dashboarding, replay-backed fixtures, and Playwright coverage for every new UI surface.
</context-digest>
Update the governing server planning docs so they describe the current intended host+user ddx-server model before follow-on server design and implementation beads continue.

## Goals
- Reconcile FEAT-002, FEAT-008, FEAT-013, FEAT-014, SD-019, and TP-002 around a single host+user ddx-server instance with explicit project registry, worker management boundaries, and queue semantics
- Define the split between git-backed project truth, tracked execution evidence, and host-local runtime/index state backed by an embedded database
- Define the current transport/security model: localhost unauthenticated HTTP for UI/API now, explicit future transport/auth evolution later
- Add the embedded-agent progress, provider dashboard, replay-backed fixture, and Playwright coverage requirements to the governed server plan
- Make the scheduling boundary explicit: server may allocate workers across projects, but bead ordering remains owned by each project queue

## Boundaries
- Keep Axon and multi-host service-backed topology as later-stage work
- Do not reopen the already-shipped single-project execute-bead loop contract except where the new server plan must reference it

## Acceptance Criteria
FEAT-002, FEAT-008, FEAT-013, FEAT-014, SD-019, and TP-002 all describe the same host+user ddx-server model, including runtime/index storage, worker boundaries, provider dashboarding, replay-backed fixtures, and mandatory Playwright coverage for new UI surfaces

## Governing References
- `FEAT-002` — `docs/helix/01-frame/features/FEAT-002-server.md` (Feature: DDx Server)

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

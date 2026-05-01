# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-359706d2`
- Title: Design server agent dashboard for provider availability and utilization
- Parent: `ddx-8b6cd40e`
- Labels: helix, phase:planning, kind:architecture, area:agent, area:api, area:ui, area:docs
- spec-id: `FEAT-008`
- Base revision: `ac7c0db9b9646b7c796c760d60c326b4d472e14b`
- Execution bundle: `.ddx/executions/20260413T214907-a42abbf6`

## Description
<context-digest>
Review area: server agent observability and operator controls. Evidence covers FEAT-002 server surfaces, FEAT-008 web UI, FEAT-014 token-awareness/routing metrics, the embedded-agent progress lane, and the requirement that a host+user ddx-server expose actionable provider availability and utilization state across configured harnesses/providers.
</context-digest>
Define the server-side agent dashboard that lets operators inspect configured providers and make informed execution decisions.

## Goals
- Show configured providers/harnesses, effective routing availability, auth/health state, and current quota or headroom when known
- Show utilization and performance signals already normalized by DDx: token usage, cost when available, recent latency/success, burn estimates, and freshness timestamps
- Support search/sort/filter/reporting across provider, model, profile, status, and time window
- Distinguish provider-native facts from DDx-derived estimates and from unknown values

## Required spec work
- Update FEAT-002, FEAT-008, and FEAT-014 so the dashboard and its backing read model are explicitly governed
- Define the API/read-model fields exposed to UI consumers and reporting surfaces
- Clarify how live provider availability, cached quota snapshots, and historical usage are presented without inventing certainty

## Required implementation planning
- Define sample/fixture data needed to Playwright-test the dashboard states
- Define how the dashboard relates to worker progress, execution history, and routing diagnostics without collapsing them into one page

## Acceptance Criteria
FEAT-002, FEAT-008, and FEAT-014 define an agent dashboard showing configured providers, availability/health, utilization metrics, freshness, and unknown-state semantics; the design includes a queryable read model and deterministic fixture scenarios for Playwright coverage

## Governing References
- `FEAT-008` — `docs/helix/01-frame/features/FEAT-008-web-ui.md` (Feature: DDx Server Web UI)

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

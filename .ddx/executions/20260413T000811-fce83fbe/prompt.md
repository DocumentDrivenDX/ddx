# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-8b6cd40e`
- Title: Evolve ddx server into a multi-project execution host
- Labels: helix, kind:architecture, area:api, area:agent, area:docs
- spec-id: `FEAT-002`
- Base revision: `7bd43ca917bcb55008735a6849e18ec5ab96aa6f`
- Execution bundle: `.ddx/executions/20260413T000811-fce83fbe`

## Description
<context-digest>
Review area: ddx server topology beyond the initial execute-bead release lane. Evidence covers FEAT-002, FEAT-013, SD-013, the single-project server architecture, and the requirement to defer multi-project and multi-host topology work until the single-project executable-bead flow is tested and released.
</context-digest>
Evolve `ddx server` beyond the initial single-project execute-bead release lane in staged steps.

## Scope
- Stage 2: one-machine, multi-project `ddx server` with project-scoped APIs, UI, and worker control after the single-project execute-bead loop is tested and released
- Stage 3: service-backed storage/control plane with a central web UI and multiple headless node gateways, each serving multiple projects and workers

## Boundaries
- Preserve local-first, git-authoritative project state for code/docs/beads
- Do not compete with the immediate FEAT-006 lane to ship a trustworthy single-project executable-bead loop
- Keep DDx primitives explicit so HELIX and other workflow tools can compose scheduling policy on top

## Acceptance Criteria
Child design beads define the one-machine multi-project server module, the server-managed execute-bead supervisor contract, and the future service-backed multi-node topology with clear stage boundaries

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
8. Before concluding no changes are needed, explicitly verify each criterion by quoting the exact text from the relevant file that satisfies it. If you cannot quote it directly, the criterion is not yet met — make the edit. Only stop with no commits if every criterion is provably satisfied by existing content.
9. Work in small commits. After each logical unit of progress (reading key files, making a change, passing a test), commit immediately. Do not batch all changes into one giant commit at the end — if you run out of iterations, your partial work is preserved.
10. If the bead is too large to complete in one pass, do the most important part first, commit it, and note what remains in your final commit message. DDx will re-queue the bead for another attempt if needed.
11. Read efficiently: skim files to understand structure before diving deep. Only read the files you need to make changes, not every reference listed. Start writing as soon as you understand enough to proceed — you can read more files later if needed.

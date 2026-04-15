# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-32b3008e`
- Title: Evolve execute-bead into a tracked execution-evidence workflow
- Labels: helix, kind:architecture, area:agent, area:exec, area:git, area:docs
- spec-id: `FEAT-006`
- Base revision: `d886572aa8aa41912305ebe614474282c5ea761d`
- Execution bundle: `.ddx/executions/20260413T140544-6b4034a1`

## Description
<context-digest>
Review area: execute-bead prompt rationalization, tracked execution evidence, and commit provenance. Evidence covers FEAT-006 execute-bead workflow, FEAT-012 git/landing behavior, FEAT-014 runtime metrics capture, the live prompt-construction bug ddx-e07fb546, the need for tracked execution artifacts to support autoresearch, and the requirement that any programmatic commit-message metadata be rendered from machine-readable files rather than ad hoc process state.
</context-digest>
Evolve execute-bead from a transient worktree runner into a tracked execution-evidence workflow.

## Scope
- Define an explicit execute-bead prompt template / rationalizer contract
- Define tracked execution artifacts for each attempt, separate from ignored local runtime scratch
- Define which execution evidence is committed on landed and preserved attempts
- Define how commit messages are rendered from tracked execution artifact files
- Keep provider-native transcripts external by default while tracking normalized pointers, usage, and summaries in DDx-owned artifacts

## Acceptance Criteria
The queue contains a coherent spec-and-build lane for execute-bead prompt rationalization, tracked execution artifacts, commit-message provenance rendered from files, and the split between tracked execution evidence and ignored local runtime scratch

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

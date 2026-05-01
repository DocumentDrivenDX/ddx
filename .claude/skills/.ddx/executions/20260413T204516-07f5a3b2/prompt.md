# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-cb418dcf`
- Title: Align specs for prompt templates, tracked execution artifacts, and commit provenance
- Parent: `ddx-32b3008e`
- Labels: helix, phase:planning, kind:architecture, area:agent, area:exec, area:git, area:docs
- spec-id: `FEAT-006,FEAT-012,FEAT-014,FEAT-015,API-001`
- Base revision: `8b81f9a0062c0f15aff436bac657fb1522e40147`
- Execution bundle: `.ddx/executions/20260413T204516-07f5a3b2`

## Description
<context-digest>
Review area: governing contract for execute-bead prompts and execution evidence. Evidence covers the current FEAT-006/API-001 workflow, the prompt fallback bug, the desire to keep beads authored and relatively stable, and the new requirement that execution evidence be tracked in git so autoresearch can replay, compare, and analyze runs.
</context-digest>
Align the governing docs around one execute-bead evidence contract.

## Goals
- Define the execute-bead prompt template as a DDx-owned rationalizer over bead fields plus resolved governing references, not a bead mutation and not a speculative summary of whole specs
- Define the stable tracked artifact set for each execution attempt: prompt, manifest, result, checks, normalized log, and usage/provider pointers
- Define the split between tracked execution evidence and ignored local scratch/runtime state
- Define the default commit policy for successful and preserved attempts
- Define the rule that all programmatically-added commit message metadata must be rendered from tracked machine-readable files

## Acceptance Criteria
FEAT-006, FEAT-012, FEAT-014, FEAT-015, and API-001 describe the same contract: execute-bead compiles a deterministic prompt from bead data plus resolved references; each attempt produces tracked machine-readable execution artifacts; ignored runtime scratch is clearly separated from tracked evidence; and commit-message metadata is projected from tracked files rather than ad hoc runtime state

## Governing References
- `FEAT-006` — `docs/helix/01-frame/features/FEAT-006-agent-service.md` (Feature: DDx Agent Service)
- `FEAT-012` — `docs/helix/01-frame/features/FEAT-012-git-awareness.md` (Feature: Git Awareness and Revision Control Integration)
- `FEAT-014` — `docs/helix/01-frame/features/FEAT-014-token-awareness.md` (Feature: Agent Usage Awareness and Routing Signals)
- `FEAT-015` — `docs/helix/01-frame/features/FEAT-015-installation-architecture.md` (Feature: DDx Installation Architecture)
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

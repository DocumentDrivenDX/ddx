# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-4a91da5d`
- Title: Adopt XML-tagged execute-bead prompt template
- Parent: `ddx-32b3008e`
- Labels: helix, phase:planning, kind:architecture, area:agent, area:exec, area:docs
- spec-id: `FEAT-006`
- Base revision: `9874e596376f6f2ecd1de405ee6caf69a07759ac`
- Execution bundle: `.ddx/executions/20260413T023941-b6b621ee`

## Description
<context-digest>
Review area: execute-bead prompt-template structure. Evidence covers the shipped synthesized prompt in cli/cmd/agent_execute_bead.go, the new tracked execution bundle under .ddx/executions/, and the need for a more deterministic machine-readable prompt contract for replay, diffing, and future autoresearch.
</context-digest>
Evolve the synthesized execute-bead prompt from markdown-heading sections to an explicit XML-tagged structure.

## Goals
- Replace markdown section headings in the synthesized execute-bead prompt with XML-style tags for machine-significant structure
- Preserve the current human-readable content while making the prompt easier to parse, diff, and validate deterministically
- Define which sections are required, optional, and repeatable in the prompt template
- Keep the prompt aligned with the tracked execution manifest and future commit-provenance rendering
- Update the governing specs and contracts so the prompt-template contract is documented rather than only implied by code

## Required spec work
- Update FEAT-006 and any applicable execute-bead contract docs to define the prompt-template structure
- Align the execution-evidence lane docs so prompt.md is explicitly structured, machine-readable evidence

## Required implementation work
- Update prompt synthesis in cli/cmd/agent_execute_bead.go to emit XML-tagged sections
- Update automated coverage to assert the new tagged structure and prevent markdown-header regression

## Acceptance Criteria
FEAT-006 and the execute-bead contract docs define an XML-tagged prompt template for execute-bead; cli/cmd/agent_execute_bead.go emits that tagged structure into prompt.md; and tests verify the required tags and reject regression to markdown-heading-only prompt structure

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

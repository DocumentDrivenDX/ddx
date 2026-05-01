# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-8464dc96`
- Title: Update DDx to ddx-agent v0.3.3
- Labels: helix, phase:build, kind:implementation, area:agent, area:cli
- spec-id: `FEAT-006`
- Base revision: `32481d387e2fd4e4d9f51e3b77df0e840077ac6b`
- Execution bundle: `.ddx/executions/20260413T143934-2f42fe98`

## Description
<context-digest>
Review area: embedded ddx-agent version alignment. Evidence covers the prior ddx-agent v0.2.0 upgrade lane, the published v0.2.1 release, and the need to pick up improved model-selection capabilities in DDx's embedded agent integration.
</context-digest>
Upgrade DDx to ddx-agent v0.2.1 and validate the embedded agent integration against the new model-selection behavior.

## Scope
- Bump the ddx-agent module dependency to v0.2.1
- Reconcile any embedded-agent integration changes required by the improved model-selection surface
- Verify DDx build/test coverage for the embedded agent path after the upgrade
- Update any relevant docs or release notes if the embedded agent capability surface changes materially

## Boundaries
- Keep this bead focused on the v0.2.1 upgrade and compatibility validation
- Broader routing/server/dashboard work remains in its existing beads

## Acceptance Criteria
cli/go.mod pins github.com/DocumentDrivenDX/agent v0.3.3; DDx builds and targeted embedded-agent tests pass; and the upgrade captures the improved model-selection capability without regressing current embedded-agent behavior

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

# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-91fd7e27`
- Title: Close execute-bead merge-gate gap
- Labels: helix, kind:architecture, area:agent, area:exec, area:docs
- spec-id: `FEAT-006`
- Base revision: `9e4d0c11159abc062d7b3a36939271608b3dd25d`
- Execution bundle: `.ddx/executions/20260412T232137-eb37b6a6`

## Description
<context-digest>
Review area: execute-bead merge gating. Evidence covers FEAT-006 merge-by-default landing semantics, FEAT-007 graph-authored execution documents with ddx.execution.required=true, FEAT-010 execute-bead compatibility rules, and the current implementation gap where execute-bead lands successful runs without enforcing documented required execution gates.
</context-digest>
Close the gap between the documented execute-bead merge-gate model and the shipped implementation.

## Scope
- Make the merge-default contract explicit: successful execute-bead runs land unless an explicit gate blocks landing or --no-merge is set
- Keep merge gates in the existing executable-bead shape: graph-authored execution documents linked through governing artifacts, not new bead fields
- Separate structural execution-readiness validation from post-run merge-gate evaluation
- Dry-run the intended model by authoring real execution-gate metadata for the current execute-loop/docs lane

## Acceptance Criteria
The queue contains a coherent lane covering contract alignment, required execution-gate enforcement in execute-bead, and authored execution-gate metadata that dry-runs the model against current FEAT-006 work

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
8. Before concluding no changes are needed, explicitly verify each criterion by quoting the exact text from the relevant file that satisfies it. If you cannot quote it directly, the criterion is not yet met — make the edit. Only stop with no commits if every criterion is provably satisfied by existing content.
9. Work in small commits. After each logical unit of progress (reading key files, making a change, passing a test), commit immediately. Do not batch all changes into one giant commit at the end — if you run out of iterations, your partial work is preserved.
10. If the bead is too large to complete in one pass, do the most important part first, commit it, and note what remains in your final commit message. DDx will re-queue the bead for another attempt if needed.
11. Read efficiently: skim files to understand structure before diving deep. Only read the files you need to make changes, not every reference listed. Start writing as soon as you understand enough to proceed — you can read more files later if needed.

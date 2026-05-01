# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-cb621399`
- Title: Specify ddx server launchd install contract for macOS
- Parent: `ddx-8b6cd40e`
- Labels: helix, area:server, area:ops, area:macos, kind:architecture
- spec-id: `FEAT-002,SD-019`
- Base revision: `53f3eeaac90f082d74a1e9737316e1089a232fb3`
- Execution bundle: `.ddx/executions/20260413T001902-fd674986`

## Description
Specify the macOS launchd lifecycle for ddx server so the service-manager contract is portable even though only the Linux implementation ships now.

## Acceptance Criteria
The server specs define the launchd plist path, working directory, logs/state locations, environment overrides, and lifecycle expectations for future macOS implementation

## Governing References
- `FEAT-002` — `docs/helix/01-frame/features/FEAT-002-server.md` (Feature: DDx Server)
- `SD-019` — `docs/helix/02-design/solution-designs/SD-019-multi-project-server-topology.md` (Solution Design: One-Machine Multi-Project Server Topology)

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

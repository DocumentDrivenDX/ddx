# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-d2930ee4`
- Title: Improve execute-loop no-progress handling and queue-drain behavior
- Labels: ddx, phase:build, kind:implementation, area:agent, area:bead, area:cli
- spec-id: `FEAT-006`
- Base revision: `b340d0e28b259ca56d9792a6127fc0a44b7c4455`
- Execution bundle: `.ddx/executions/20260413T145945-e33a9d4f`

## Description
<context-digest>
Review area: execute-loop queue-drain reliability after the no-change misclassification fix. Evidence covers repeated installed-binary `execute-loop --once` runs selecting the same execution-ready bead, the new `no_changes` status, the need to auto-close already-satisfied beads without reintroducing false-success closure, and the requirement to avoid infinite retry loops on unresolved no-progress work.
</context-digest>
Improve execute-loop so repeated no-progress attempts are classified and handled in a way that lets the queue make forward progress without sacrificing correctness.

## Goals
- Distinguish already-satisfied no-change beads from unresolved no-progress attempts
- Prevent immediate re-execution loops on beads that repeatedly return `no_changes`
- Keep closure semantics machine-owned and evidence-backed rather than inferred from freeform agent output
- Preserve queue ordering semantics while making the queue drain behavior operationally useful

## Acceptance Criteria
The queue contains a concrete contract-and-build lane for execute-loop no-change disposition, satisfaction-based auto-close, and retry suppression so queue burning does not stall on one unresolved no-op bead

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
12. **Never run `ddx init`** — the workspace is already initialized. Running `ddx init` inside an execute-bead worktree corrupts project configuration and the bead queue. Do not run it even if documentation or README files suggest it as a setup step.

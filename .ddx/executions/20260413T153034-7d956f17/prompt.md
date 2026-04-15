# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-b44ca394`
- Title: Prevent execute-loop from launching open epics by default
- Parent: `ddx-cf340665`
- Labels: helix, phase:build, area:agent, area:bead, area:cli
- spec-id: `FEAT-006`
- Base revision: `0b0ab1c3ab4d2c96ee817faba0cc2add5a8b7e94`
- Execution bundle: `.ddx/executions/20260413T153034-7d956f17`

## Description
<context-digest>
Alpha validation run from /Users/erik/Projects/helix using `ddx agent execute-loop --once --json` against a live HELIX queue. Evidence includes the generated execution bundle `.ddx/executions/20260411T011535-bbda19d7`, worktree `.ddx/.execute-bead-wt-helix-fef22846-20260411T011535-bbda19d7`, and the current execute-loop implementation in `cli/internal/bead/store.go` and `cli/internal/agent/execute_bead_loop.go`.
</context-digest>

`ddx agent execute-loop` selected and launched an open `epic` bead from the HELIX execution queue instead of a bounded executable bead.

## Reproduction
1. In `/Users/erik/Projects/helix`, run `ddx bead ready --json --execution`.
2. Observe the first candidate is `helix-fef22846`, `issue_type=epic`, title `Audit deleted HELIX artifact types for restoration or retirement`.
3. Run `ddx agent execute-loop --once --json`.
4. Observe DDx creates `.ddx/.execute-bead-wt-helix-fef22846-20260411T011535-bbda19d7` and `.ddx/executions/20260411T011535-bbda19d7/manifest.json` with `bead_id=helix-fef22846`.

## Observed gap
`ReadyExecution()` currently treats any open, unblocked bead as execution-ready unless `execution-eligible=false`, `superseded-by`, or cooldown metadata says otherwise. `execute-loop` then takes the first candidate without a stronger structural execution screen. That behavior is weaker than the API-001 execute-bead supervisor contract, which says the loop should run a generic execution-ready validator against ordered candidates before launch.

## Why this matters
Open epics represent umbrella planning/decomposition work, not necessarily a bounded workspace transform. Launching them directly makes queue-drain behavior unstable and can strand the loop on non-executable work even when bounded tasks are ready behind the epic.

## Acceptance Criteria
`ddx agent execute-loop` does not launch open epics or other structurally non-executable bead types by default; the generic execution-ready validation surface rejects or skips such beads before claim/launch; and deterministic tests cover a queue where an open epic sorts ahead of a ready task so the loop selects the task instead of the epic.

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

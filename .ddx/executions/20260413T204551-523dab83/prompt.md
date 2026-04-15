# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-3315dce2`
- Title: execute-loop: bead claiming is not atomic — concurrent workers race on the same bead
- Labels: bug, execute-loop, concurrency
- Base revision: `0cef4c38eda685d3315238e77e85b358af9fe0b9`
- Execution bundle: `.ddx/executions/20260413T204551-523dab83`

## Description
When two execute-loop workers scan the ready queue simultaneously, both can claim the same bead before either has committed its in_progress status, resulting in duplicated work or merge conflicts.

Observed: started two execute-loop --local --harness agent workers (vidar and bragi) against the same project. Both immediately began working on the same bead in separate worktrees. Their output files were in near-perfect lockstep (131 vs 127 lines), indicating they had independently claimed and started executing the same item. One was killed manually to stop the waste.

Root cause hypothesis: the claim operation in execute-loop reads the ready queue, selects the next open bead, then writes its status to in_progress as two separate steps. If two workers interleave between the read and the write, both see the bead as open and both proceed to execute it.

Required fix: bead claiming must be an atomic compare-and-swap. The worker should attempt to transition bead status from open → in_progress using a conditional write that fails if the status is no longer open at write time (optimistic locking). If the claim fails, the worker should re-scan for the next available bead rather than proceeding.

For local workers (beads.jsonl on disk), a file-level lock around the read-claim-write cycle is sufficient. For server-submitted workers, the DDx server should serialize claims.

Additional issue: when a worker is killed mid-execution, the bead may be left in_progress indefinitely. The loop should support a claim timeout or heartbeat so abandoned in_progress beads become reclaimable after a TTL.

## Acceptance Criteria
Two execute-loop --local workers started simultaneously against the same project never execute the same bead; the second worker to attempt a claim on an already-claimed bead skips it and picks the next open one; a bead left in_progress by a crashed worker becomes reclaimable after a configurable TTL (default: 10 minutes)

## Governing References
No governing references were pre-resolved. Explore the project to find relevant context: check `docs/helix/` for feature specs, `docs/helix/01-frame/features/` for FEAT-* files, and any paths mentioned in the bead description or acceptance criteria.

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

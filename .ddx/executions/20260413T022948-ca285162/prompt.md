# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-051b0015`
- Title: Add dedicated read-only tools (ls, find, grep) to ddx-agent for stall detection and efficiency
- Labels: ddx, phase:planning, kind:architecture, area:agent, area:exec
- Base revision: `42fd0feeb54667aa6f89b98c491943357972d42e`
- Execution bundle: `.ddx/executions/20260413T022948-ca285162`

## Description
During execute-loop runs with OpenRouter models (GLM-5.1, Qwen 3.6), agents get stuck in read loops — they call read and bash (for ls/find/grep) repeatedly without ever writing. The stall detector cannot distinguish bash ls from bash "git commit" because bash is classified as a write-capable tool, resetting the stall counter on every call.

Adding dedicated read-only tools (ls, find, grep) to ddx-agent would:
1. Let the stall detector accurately distinguish read-only exploration from write activity
2. Reduce token usage by avoiding bash prompt/response overhead for simple file queries
3. Give models clearer affordances — read-only tools for exploration, write/edit/bash for modification
4. Help models transition from reading to writing faster by making the tool boundary explicit

With these tools, the stall detector classifies read/ls/find/grep as read-only (increment stall counter) and write/edit/bash as write-capable (reset counter). Bash is only used when the agent needs commands that might modify the filesystem.

## Acceptance Criteria
dxx-agent has dedicated ls, find, and grep tools; stall detector classifies them as read-only; bash is only classified as write-capable; agents on OpenRouter models transition from reading to writing faster

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

# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-2ef94940`
- Title: execute-loop: --harness flag silently ignored when --model is a provider name
- Labels: bug, execute-loop, cli, ux
- Base revision: `ac7c0db9b9646b7c796c760d60c326b4d472e14b`
- Execution bundle: `.ddx/executions/20260413T215908-869920b7`

## Description
When execute-loop is invoked with both --harness claude and --model <provider-name> (e.g. --model claude-sonnet-4-6), the --harness flag is silently ignored and the embedded agent harness is used instead, routing to whichever LM Studio provider is configured as default.

Observed: started execute-loop --local --harness claude --model claude-sonnet-4-6. Every session in the output showed 'model: vidar' or 'model: bragi' and all LLM responses were from qwen3.5-27b. No claude traffic was generated. The claude binary is healthy (ddx agent doctor reports OK). When execute-loop is started with --harness claude and NO --model flag, the claude binary is correctly invoked (confirmed by claude --print process appearing in ps output).

Root cause hypothesis: the --model flag value is being interpreted as a provider/route key by the embedded agent harness dispatch layer. When the model string matches a known provider name or a catalog entry that maps to the embedded-openai surface, the harness selection silently switches to 'agent' regardless of what --harness specifies. The --harness flag loses the race.

Required fix: --harness must take strict precedence over any routing implied by --model. If --harness claude is specified and the model string is not a valid claude model ID, the loop should either (a) pass the model string through to the claude binary as-is and let it fail with a clear error, or (b) emit a startup warning 'model X is not a recognized claude model; ignoring --model flag' and proceed with the harness default. Silent rerouting to a different harness is never acceptable.

Additionally: when the harness override causes a routing mismatch, the error or warning must appear before any bead is claimed, not after execution has started.

## Acceptance Criteria
ddx agent execute-loop --harness claude --model claude-sonnet-4-6 invokes the claude binary, not ddx-agent; session log shows model: claude-sonnet-4-6 not model: vidar or model: bragi; if --model specifies an unrecognized claude model, the error is surfaced before any bead is claimed; ddx agent execute-loop --harness agent --model vidar continues to work as before (regression guard)

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

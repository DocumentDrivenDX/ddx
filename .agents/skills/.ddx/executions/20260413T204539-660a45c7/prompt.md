# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-9a5df084`
- Title: Fix pre-push no-debug hook regex compatibility and failure semantics
- Labels: helix, phase:build, area:tooling, area:quality
- spec-id: `FEAT-001`
- Base revision: `5341433135201ba34d614555a8e465814dfa075f`
- Execution bundle: `.ddx/executions/20260413T204539-660a45c7`

## Description
<context-digest>
Observed during `git push` in /Users/erik/Projects/ddx after landing tracker commits for execute-loop alpha findings. The DDx pre-push hook `no-debug` invoked `rg` with a look-ahead pattern and emitted a regex parse error, but the hook summary still reported success and the push continued.
</context-digest>

The DDx pre-push `no-debug` hook uses an `rg` pattern that requires PCRE2 look-around support, but it invokes plain `rg` without `--pcre2`. In the current environment that produces a parse error while the hook still exits successfully.

## Reproduction
1. In `/Users/erik/Projects/ddx`, run `git push`.
2. Observe pre-push hook output from `no-debug` containing:
   `rg: regex parse error: ... look-around ... is not supported`
3. Observe the hook summary still marks `no-debug` as passed and the push continues.

## Why this matters
The hook currently gives a false sense of enforcement. A broken regex should either be written in ripgrep-compatible syntax or use `--pcre2`, and failures in the debug scan should not be silently treated as success.

## Acceptance Criteria
The DDx pre-push `no-debug` hook runs without regex parse errors in a default ripgrep environment; the debug-scan command fails closed when the scan itself errors; and the hook still blocks actual debug statements it is meant to catch.

## Governing References
- `FEAT-001` — `docs/helix/01-frame/features/FEAT-001-cli.md` (Feature: DDx CLI)

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

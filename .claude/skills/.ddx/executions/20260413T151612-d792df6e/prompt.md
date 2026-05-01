# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-60316844`
- Title: Fix fallback secret scan false positives on tracker JSONL commits
- Labels: helix, phase:build, area:security, area:bead, area:tooling
- spec-id: `FEAT-001`
- Base revision: `189faca3c3db788469def0ee986d6222145270e2`
- Execution bundle: `.ddx/executions/20260413T151612-d792df6e`

## Description
<context-digest>
Tracker-only DDx bead commit from /Users/erik/Projects/ddx after filing execute-loop alpha findings. Environment does not have `gitleaks`, so the `lefthook` pre-commit hook used the fallback grep path in `lefthook.yml`.
</context-digest>

The fallback `secrets` hook in `lefthook.yml` produces false positives on staged `.ddx/beads.jsonl` changes because it scans the entire staged file with a broad `token|auth|password|...` regex instead of scanning added lines with tighter assignment-oriented patterns.

## Reproduction
1. Ensure `gitleaks` is not installed.
2. Stage a normal tracker-only change to `.ddx/beads.jsonl`.
3. Run `git commit`.
4. Observe the `secrets` hook fails and prints a line from `.ddx/beads.jsonl` where text like `token-awareness.md` and later quoted JSON content satisfy the fallback regex even though no secret was added.

## Observed gap
The fallback pattern is broad enough to match ordinary prose containing words like `token` or `auth`, and because it scans the whole staged file it re-trips on historical tracker content whenever `.ddx/beads.jsonl` changes.

## Why this matters
In environments without `gitleaks`, normal bead creation becomes effectively uncommittable. That blocks DDx's own tracker workflow and makes it harder to land the very bugs discovered during alpha execution.

## Acceptance Criteria
When `gitleaks` is unavailable, the `lefthook` fallback secret scan still catches obvious secret assignments but does not fail on ordinary `.ddx/beads.jsonl` tracker updates; the fallback inspects staged additions rather than whole files or otherwise avoids re-flagging historical tracker content; and a deterministic repro covers a staged tracker diff containing `token-awareness.md` text without any added secrets.

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

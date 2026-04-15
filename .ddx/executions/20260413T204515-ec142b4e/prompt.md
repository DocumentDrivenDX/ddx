# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

## Bead
- ID: `ddx-c19ae8b8`
- Title: Implement execute-loop-first help, skill, and operator guidance
- Parent: `ddx-a4195a8e`
- Labels: helix, phase:build, kind:documentation, area:cli, area:agent, area:docs, area:skills
- spec-id: `FEAT-006`
- Base revision: `4607b7bbf91e3b0a442e695711f9cb5801224240`
- Execution bundle: `.ddx/executions/20260413T204515-ec142b4e`

## Description
<context-digest>
Review area: consumer-facing execute-bead / execute-loop guidance. Evidence covers the shipped CLI help surfaces, the open docs gaps tracked by ddx-a4195a8e and ddx-7a01ba6c, and the need to teach humans and agents one coherent model: execute-bead is the primitive, execute-loop is the normal queue-driven surface.
</context-digest>
Implement the first complete consumer-facing guidance surface for DDx agent execution.

## Required changes
1. Update CLI help text and any nearby command docs so `ddx agent execute-bead` is described as the single-bead primitive and `ddx agent execute-loop` is described as the queue/supervisor command built on top of it.
2. Add or update user-facing operator docs and CLI reference surfaces so users discover `execute-loop` first for normal queue-driven work, with concrete examples for:
   - `ddx agent execute-loop`
   - `ddx agent execute-loop --once`
   - `ddx agent execute-loop --poll-interval <duration>`
   - `ddx agent execute-bead <bead-id>` for bounded/manual runs
3. Update the ddx-bead skill and any relevant internal skill/operator guidance so agents are told explicitly:
   - planning/document-only beads are valid execution targets
   - execute-loop is the default queue-driven entrypoint
   - execute-bead is the lower-level primitive for targeted/manual execution
4. Document the result/close semantics operators need in practice:
   - which outcomes/statuses close a bead
   - which outcomes preserve and leave the bead open
   - which JSON fields automation should read (`status`, `outcome`, `preserve_ref`, `result_rev`, `session_id`)
5. Make the operator guidance consistent with the current merge model:
   - merge is the default successful outcome
   - explicit required execution documents are the automatic merge gates
   - `--no-merge` is the manual preserve override

## Boundaries
- Do not invent a new bead shape or merge-gate field
- Use the existing execute-bead / execute-loop / execution-document contract
- Keep the user-facing guidance aligned with the actual shipped command behavior and documented result surface

## Acceptance Criteria
CLI help, operator docs, and ddx-bead skill guidance all teach the same model: execute-bead is the primitive, execute-loop is the normal queue-driven surface, planning/document-only beads are valid execution targets, merge is default unless explicit required execution docs block landing or --no-merge is set, and the documented JSON/result semantics tell operators and automation how close/preserve outcomes behave

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

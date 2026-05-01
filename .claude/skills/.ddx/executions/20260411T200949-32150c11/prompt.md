# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-c19ae8b8`
- Title: Implement execute-loop-first help, skill, and operator guidance
- Parent: `ddx-a4195a8e`
- Labels: helix, phase:build, kind:documentation, area:cli, area:agent, area:docs, area:skills
- spec-id: `FEAT-006`
- Base revision: `a555a4d6e5b4393c18ba2ca01df4622f9541d453`
- Execution bundle: `.ddx/executions/20260411T200949-32150c11`

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
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If the bead is missing critical context or the governing references conflict, stop and report the gap explicitly instead of improvising hidden policy.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. Before finishing, commit your changes with `git add -A && git commit -m '...'`. DDx will merge your commits back to the base branch.
8. If the work is already satisfied with no tracked changes needed, stop cleanly and let DDx record a no-change attempt.

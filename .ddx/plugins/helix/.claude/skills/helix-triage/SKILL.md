---
name: helix-triage
description: Create tracker issues with governing artifact references and deterministic acceptance and success-measurement criteria.
argument-hint: '"Title" [--type task|epic|bug|chore] [options...]'
---

# Triage: Shape Execution-Ready And Planning Issues

Execution-ready implementation beads should enter the tracker ready to
execute. When the request is still vague or lacks deterministic success
criteria, route it to planning/polish or file it explicitly as a
not-execution-ready planning/review bead instead of pretending it is ready for
`helix run`. This skill guides you through shaping both paths with the
metadata HELIX uses to select, refine, build, and close work.

## When to Use

- The user asks for work to be done, tracked, queued, or filed as an issue
- The user mentions bugs, tasks, or work items that need creating
- Creating new implementation, design, or test issues
- Decomposing an epic into child tasks
- Recording blockers or follow-on work during implementation
- Filing bugs discovered during review or testing
- Batch-creating execution issues from alignment review gaps or gap registers
- Any time work should enter the tracker rather than be listed as "next steps"

**Trigger rule**: If the user is asking for work and it is not a spec or
requirement change (use `helix evolve` for those), default to triage. When in
doubt between suggesting next steps and filing issues, file the issues.

## Required Metadata

Use these conventions when creating issues so they remain executable and
traceable:

| Field | Required For | What To Set |
|-------|-------------|-------------|
| `--labels helix,phase:*` | All issues | `helix` + one of `phase:frame`, `phase:design`, `phase:test`, `phase:build`, `phase:deploy`, `phase:iterate`, `phase:review` |
| `--spec-id` | Tasks | Nearest governing artifact (e.g., `SD-001`, `ADR-048`, `TP-SD-014`) |
| `--acceptance` | Tasks, Epics | Deterministic completion and success-measurement criteria — what must be true, and how success will be observed, when this is done |
| `--description` | Recommended | Work contract: what to do, which files, governing artifact references |
| `--design` | Recommended | Implementation approach: how to do it |

Additional labels to include when applicable:
- `kind:build`, `kind:implementation`, `kind:testing`, `kind:documentation`
- `area:*` for the affected subsystem, using the taxonomy declared in
  `docs/helix/01-frame/concerns.md`

## Steps

1. **Identify the governing artifact.** What spec, design, ADR, or test plan
   authorizes this work? Set `--spec-id` to its ID.

2. **Write deterministic acceptance and success-measurement criteria.** The
   AC should be verifiable by running a specific command or checking a
   specific condition, and it should make success observable enough for
   DDx-managed execution to close merged work with evidence. Bad:
   "system works correctly." Good: "cargo test -p niflheim-wal passes;
   AC-SD17-003 promoted to active in TP-SD-017.acceptance.toml."
   Better: "cargo test -p niflheim-wal passes; `docs/helix/.../contract.md`
   names `ddx agent execute-loop` as the queue-drain primitive; git diff
   --check passes."

   Triage must not create execution-ready implementation beads without
   measurable success criteria. If the governing artifacts do not let you
   name explicit commands, checks, files, fields, or observable end-state
   conditions, stop treating the work as queue-ready execution input.

3. **Choose type and labels.**
   - `task` — bounded work with clear completion (most issues)
   - `epic` — collection of tasks sharing a goal (needs child decomposition)
   - `bug` — something broken (no spec-id needed)
   - `chore` — maintenance (no spec-id needed)

4. **Set parent and dependencies.** If this is part of an epic, set `--parent`.
   If it depends on other issues completing first, add explicit tracker
   dependencies after creation with `ddx bead dep add`.

   For queue-drained execution beads, prefer AC that names:
   - exact commands to run
   - exact files or fields expected to change
   - exact pass/fail or observable end-state conditions
   Avoid AC that requires a human to decide whether the result "looks right"
   unless the bead is explicitly a review or planning bead rather than an
   execution-ready implementation bead.

   If you cannot derive deterministic success criteria from the governing
   artifacts, do not create a queue-ready implementation bead anyway. Route
   the work back to planning/polish, or file it as a not-execution-ready
   planning/review bead with the missing information called out explicitly.

5. **Assemble context digest.** Follow
   `.ddx/plugins/helix/workflows/references/context-digest.md` to build a compact summary of
   active principles, concerns, practices, relevant ADRs, and governing spec
   context. Prepend the `<context-digest>` XML block to the description so
   the implementing agent can work from the bead alone. If the repo ships
   `scripts/refresh_context_digests.py`, use it after bead creation instead of
   hand-writing digest XML. The `<concerns>` field must list concern names,
   never area labels.

6. **Create the issue.**

```bash
new_id="$(ddx bead create "Implement X" \
  --type task \
  --labels helix,phase:build,kind:implementation,area:wal \
  --spec-id SD-017 \
  --description "<context-digest>
<principles>Design for simplicity · Tests first · Fake data</principles>
<concerns>typescript-bun | postgres</concerns>
<practices>ESLint strict · Biome · bun:test</practices>
<adrs>ADR-050 event sourcing for audit</adrs>
<governing>SD-017 §3.2 — Y component must handle partial writes</governing>
</context-digest>

Implement the Y component per SD-017 Section 3.2. Governing: SD-017, TP-SD-017, ADR-050." \
  --acceptance "cargo test -p niflheim-wal test_y passes; AC-SD17-005 promoted to active" \
  --parent hx-epic-id)"

ddx bead dep add "$new_id" hx-dependency-id
```

## Decomposition Pattern

When an epic or large task needs breakdown:

1. Read the parent's acceptance criteria
2. Create child tasks that collectively cover every AC
3. Set `--parent` on each child
4. Add explicit `ddx bead dep add` edges when child order matters
5. Each child should be completable in one `helix build` or `execute-bead` cycle
6. If the parent is a task with 2+ children, consider promoting it to an epic

## References

- Tracker conventions: `ddx bead --help` (DDx FEAT-004)
- Implementation action: `.ddx/plugins/helix/workflows/actions/implementation.md`
- CLI: `ddx bead create --help`

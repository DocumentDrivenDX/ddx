---
name: helix-triage
description: Create tracker issues with governing artifact references and deterministic acceptance criteria.
argument-hint: '"Title" [--type task|epic|bug|chore] [options...]'
---

# Triage: Create Execution-Ready Issues

Every issue should enter the tracker ready to execute. This skill guides you
through shaping issues with the metadata that `helix run` uses to select,
build, and close them.

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
| `--acceptance` | Tasks, Epics | Deterministic completion criteria — what must be true when this is done |
| `--description` | Recommended | Work contract: what to do, which files, governing artifact references |
| `--design` | Recommended | Implementation approach: how to do it |

Additional labels to include when applicable:
- `kind:build`, `kind:implementation`, `kind:testing`, `kind:documentation`
- `area:*` for the affected subsystem (e.g., `area:wal`, `area:query`, `area:cli`)

## Steps

1. **Identify the governing artifact.** What spec, design, ADR, or test plan
   authorizes this work? Set `--spec-id` to its ID.

2. **Write deterministic acceptance criteria.** The AC should be verifiable
   by running a specific command or checking a specific condition. Bad:
   "system works correctly." Good: "cargo test -p niflheim-wal passes;
   AC-SD17-003 promoted to active in TP-SD-017.acceptance.toml."

3. **Choose type and labels.**
   - `task` — bounded work with clear completion (most issues)
   - `epic` — collection of tasks sharing a goal (needs child decomposition)
   - `bug` — something broken (no spec-id needed)
   - `chore` — maintenance (no spec-id needed)

4. **Set parent and dependencies.** If this is part of an epic, set `--parent`.
   If it depends on other issues completing first, use `--deps`.

5. **Assemble context digest.** Follow
   `.ddx/plugins/helix/workflows/references/context-digest.md` to build a compact summary of
   active principles, concerns, practices, relevant ADRs, and governing spec
   context. Prepend the `<context-digest>` XML block to the description so
   the implementing agent can work from the bead alone.

6. **Create the issue.**

```bash
ddx bead create "Implement X" \
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
  --parent hx-epic-id \
  --deps hx-dependency-id
```

## Decomposition Pattern

When an epic or large task needs breakdown:

1. Read the parent's acceptance criteria
2. Create child tasks that collectively cover every AC
3. Set `--parent` on each child
4. Each child should be completable in one `helix build` cycle
5. If the parent is a task with 2+ children, consider promoting it to an epic

## References

- Tracker conventions: `ddx bead --help` (DDx FEAT-004)
- Implementation action: `.ddx/plugins/helix/workflows/actions/implementation.md`
- CLI: `ddx bead create --help`

---
name: ddx-bead
description: Create and manage DDx beads (work items) with proper metadata, labels, and dependency wiring.
argument-hint: '"Title" [--type task|epic|bug|chore] [options...]'
---

# DDx Bead: Create and Manage Work Items

Beads are the DDx work item primitive. Every piece of traceable work — tasks,
epics, bugs, chores — lives as a bead. This skill guides you through creating
beads with the metadata needed for execution and traceability.

## Tracker Policy

- Use `ddx bead` subcommands for all bead maintenance: create, update, close,
  list, show, status, ready, blocked, dep, import, export, init, and evidence.
- Do not edit `.ddx/beads.jsonl` directly.
- Do not invent bead IDs or prefixes such as `hx-*` or `ddx-*`; use IDs from
  the CLI.

## When to Use

- Creating new work items of any type
- Decomposing an epic into child tasks
- Recording dependencies between work items
- Updating status, metadata, or relationships on existing beads
- Querying the tracker for work item state

## Required Fields

| Field | Flag | Required For | What To Set |
|-------|------|-------------|-------------|
| Title | positional | All | Short, imperative description of the work |
| Type | `--type` | All | `task`, `epic`, `bug`, or `chore` |
| Labels | `--labels` | All | `helix` required; add phase and kind labels (see below) |
| Acceptance | `--acceptance` | Tasks, Epics | Deterministic criteria verifiable by command or condition |

## Label Conventions

All beads must carry at minimum:
- `helix` — marks the bead as part of the HELIX workflow

Add one phase label:
- `phase:frame`, `phase:design`, `phase:test`, `phase:build`, `phase:deploy`, `phase:iterate`, `phase:review`

Add a kind label when applicable:
- `kind:implementation`, `kind:testing`, `kind:documentation`, `kind:research`

Add an area label for the affected subsystem:
- `area:cli`, `area:server`, `area:bead`, `area:agent`, `area:mcp`, `area:config`

## Finding the Governing Artifact for spec-id

The `--spec-id` links a bead to the document that authorizes it. To find the
nearest governing artifact:

1. Check `docs/helix/02-design/solution-designs/` for solution design docs (SD-*)
2. Check `docs/helix/02-design/technical-designs/` for technical designs (TD-*)
3. Check `docs/helix/02-design/adr/` for architectural decisions (ADR-*)
4. Check `docs/helix/01-frame/features/` for feature specs (FEAT-*)
5. Use the most specific document that authorizes the work

## Steps

1. **Identify governing artifact.** What spec, design, or feature document
   authorizes this work? Record the reference in the bead description or a
   custom field if needed.

2. **Write deterministic acceptance criteria.** AC must be verifiable — a
   command that passes, a file that exists, a behavior that can be observed.
   Bad: "works correctly." Good: "`go test ./internal/bead/... passes`; bead
   collection round-trips through JSONL export/import."

3. **Choose type and labels.**
   - `task` — bounded work with clear completion (most beads)
   - `epic` — collection of tasks sharing a goal
   - `bug` — something broken (spec-id optional)
   - `chore` — maintenance work (spec-id optional)

4. **Set parent and dependencies** if applicable.

5. **Create the bead.**

```bash
ddx bead create "Implement X" \
  --type task \
  --labels helix,phase:build,kind:implementation,area:bead \
  --description "Implement Y component per SD-004 Section 2." \
  --acceptance "go test ./internal/bead/... passes; bead persists across restart" \
  --parent ddx-epic-id
```

## Update Workflow

To update fields on an existing bead:

```bash
ddx bead update <id> --labels helix,phase:iterate,kind:implementation
ddx bead update <id> --acceptance "updated acceptance criteria"
ddx bead update <id> --status closed
ddx bead close <id>
```

## Dependency Management

```bash
# Add a dependency (bead B depends on bead A completing first)
ddx bead dep add <bead-b-id> <bead-a-id>

# Remove a dependency
ddx bead dep remove <bead-b-id> <bead-a-id>

# View full dependency tree
ddx bead dep tree <id>
```

## Query and Filter

```bash
ddx bead list --status open
ddx bead ready             # Beads with no unmet dependencies
ddx bead blocked           # Beads blocked by dependencies
ddx bead show <id>         # Full detail on one bead
ddx bead status            # Summary counts by state
```

## Execution Guidance

When using DDx agent primitives against bead work:
- Prefer `ddx agent execute-loop` for normal queue-driven execution.
- Use `ddx agent execute-bead <id>` for a specific manual single-bead run.
- For direct `ddx agent run` usage, prefer `--profile smart` or another profile.
- Treat `--model` and `--effort` as explicit overrides; do not add them on top
  of `--profile` unless you are intentionally testing or pinning behavior.

## References

- Full flag list: `ddx bead --help`, `ddx bead create --help`
- Tracker feature spec: `docs/helix/01-frame/features/FEAT-004-beads.md`

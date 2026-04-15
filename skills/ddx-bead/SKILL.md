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
- Treat tracker mutations as durable work, not disposable local state.
- After `ddx bead create`, `update`, `dep add/remove`, or `close`, commit the
  resulting tracker change by default.
- If the bead edit is the only repo change, make a tracker-only commit
  immediately. If related implementation/docs changes are already in progress,
  fold the tracker change into that same commit instead of leaving `.ddx/beads.jsonl`
  dirty.

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

## What Makes a Bead Execution-Ready

A bead that passes required-field validation is not necessarily execution-ready.
An automated `execute-bead` run reads the bead description cold — no prior
conversation, no open tabs, no access to the history that led to the bead. The
description must be self-sufficient.

**Execution-ready means the description answers all of these:**

1. **Which files to touch.** Exact repo-relative paths, not module names or
   component names. `crates/axon-server/src/auth_pipeline.rs`, not "the auth
   pipeline."

2. **Which files NOT to touch.** Scope negation prevents scope creep. If the
   bead is only about auth_pipeline, say "do not touch auth_schema.rs."

3. **What cross-bead context is needed.** If this bead depends on a prior bead,
   describe what that prior bead landed: file paths, type names, key APIs. Do
   not assume the agent will read the prior bead's history. Each bead is read
   cold.

4. **Spec content inline, not by reference.** If the bead implements a table
   from ADR-018 §4, paste the table into the description. Do not write "see
   ADR-018 §4." The agent can only act on content that is present.

5. **Named verification commands in AC.** `go test ./internal/bead/... passes`
   is verifiable. "unit tests green" is not. Every AC item must name the exact
   command or condition.

**The description is the contract the agent follows.** If the description
contains a typo (e.g., `subject` instead of `external_id`), the agent will
implement the typo faithfully. Authors are responsible for accuracy.

## Description Template

Use this template as a starting point. Omit sections that are genuinely not
applicable, but err toward inclusion — extra context costs nothing, missing
context costs a re-run.

```
## Scope

One or two sentences: what this bead does and why it exists.

## Files to Touch

- `path/to/file.go` — what changes and why
- `path/to/other.rs` — what changes and why

## Files NOT to Touch

- `path/to/out_of_scope.go` — belongs to bead <id> or phase <x>

## Cross-Bead Context

*Fill this section when this bead has dependencies.*

Bead <id> (<title>) landed:
- `path/to/file.rs` — contains `TypeName { field_a, field_b }` (use field_b, not field_a)
- `path/to/schema.sql` — adds table `credential_revocations(user_id, revoked_at)`

## Spec Excerpts

*Paste the relevant sections inline. Do not link to docs; the agent reads the bead, not the docs.*

[paste table, struct definition, API shape, or decision text here]

## Verification Commands

- `<exact command>` — what it checks
- `<exact command>` — what it checks
```

## Bead Sizing

A bead should be one unit of testable work that a single agent pass can
complete. Use this heuristic:

- **Too large:** AC has more than ~12 checkboxes, or the description names more
  than ~5 files to touch. Split into sibling beads.
- **Too small:** The work is a one-line change or a trivial rename with no
  independent acceptance criteria. Fold it into a related bead.
- **Right size:** The agent can read the description, touch the named files, run
  the named commands, and be done — in a single pass with no ambiguity.

When splitting an epic into tasks, prefer splits along test boundaries: each
task should have its own named verification command that can pass independently.

## Review-Driven Reopen

When a bead is reopened after a `REQUEST_CHANGES` review:

1. **Update the description in place.** Add a `## Review Findings` subsection
   (or fold corrections into the relevant sections). The next execute-bead run
   will see the corrected description.
2. Do not rely on the review comment history being visible to the next agent
   run. The description is the only guaranteed source of context.
3. If the review identified a spec ambiguity (e.g., `subject` vs
   `external_id`), fix the spec excerpt in the description before re-queuing.

```bash
ddx bead update <id> --description "$(cat updated_description.md)"
```

## Common Anti-Patterns

These mistakes were observed during execute-loop runs and consistently produce
bad agent outputs:

- **Doc ID without inline content.** Writing "per ADR-018 §4" instead of
  pasting §4's content. The agent cannot open ADR-018; it acts on what is in
  the description.
- **No file paths.** Writing "update the auth module" instead of
  `crates/axon-server/src/auth_pipeline.rs`. The agent will guess and may touch
  the wrong file.
- **Vague AC items.** Writing "tests pass" instead of
  `cargo test -p axon-server`. Vague items cannot be verified and produce
  inconsistent results.
- **Missing cross-bead context.** Writing "depends on A1" without describing
  what A1 landed. The agent starts each bead cold and will not read A1's
  history.
- **Scope described in the parent epic only.** Subtask beads must be
  self-contained. The agent does not read the parent epic unless the bead
  description explicitly quotes it.
- **Typos in type names or field names.** The description is the contract. A
  wrong field name in the description produces wrong code.

## Update Workflow

To update fields on an existing bead:

```bash
ddx bead update <id> --labels helix,phase:iterate,kind:implementation
ddx bead update <id> --acceptance "updated acceptance criteria"
ddx bead update <id> --status closed
ddx bead close <id>
```

After these commands, check the tracker diff and commit it unless you are
intentionally folding it into the current task's implementation commit.

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
- `ddx agent execute-loop` is the **normal queue-driven surface**. It drains
  the execution-ready queue, claiming and running each bead through
  `execute-bead`, and closes beads only on `success`/`already_satisfied`.
  Reach for it by default.
- `ddx agent execute-bead <id>` is the **primitive** — one agent on one bead
  in an isolated worktree. Use it to debug or re-run a specific bead; the
  loop calls it internally.
- Planning and document-only beads are valid execution targets. Any bead
  with unmet acceptance criteria and no blocking deps is eligible; acceptance
  can produce docs, specs, code, or any other artifact.
- The loop closes the bead with session + commit evidence only on `success`
  (and `already_satisfied` after repeated `no_changes`). Every other status
  — `no_changes`, `land_conflict`, `post_run_check_failed`, `execution_failed`,
  `structural_validation_failed` — leaves the bead open and unclaimed for a
  later attempt.
- For direct `ddx agent run` usage, prefer `--profile smart` or another profile.
- Treat `--model` and `--effort` as explicit overrides; do not add them on top
  of `--profile` unless you are intentionally testing or pinning behavior.
- See `docs/agent-execute.md` for the operator reference on close semantics
  and result statuses.

## Branch Merge Policy

execute-bead and execute-loop runs emit commits that carry durable per-attempt
execution metrics. These commits are **not noise** — they are the audit trail.

Commit shapes produced by the loop:

- `chore: update tracker (execute-bead <TIMESTAMP>)` — one per attempt tracker
  heartbeat. The TIMESTAMP is the attempt identifier. Emitted for every
  attempt: success, `no_changes`, `land_conflict`, `post_run_check_failed`,
  `execution_failed`, `structural_validation_failed`.
- `Merge bead <bead-id> attempt <TIMESTAMP>- into <branch>` — the merge commit
  that lands a successful attempt from its isolated worktree back onto the
  working branch. Carries the bead ID and attempt timestamp in the subject.
- Substantive `feat:` / `fix:` / `refactor:` commits tagged with `[ddx-<id>]`
  — the actual code change for a bead.

Together these let you reconstruct which beads were attempted, when, how many
retries they took, and what the outcomes were. Cost, latency, retry-count, and
tier-escalation reports read this trail directly from git history.

**Never squash-merge** a branch containing execute-bead commits. Squashing
collapses every attempt timestamp into a single commit message and destroys
the per-attempt metric trail. `gh pr merge --squash` is forbidden for these
branches.

**Never rebase-drop or fixup** tracker-heartbeat commits even if they look
like "chore noise". The timestamp in the subject line is load-bearing data.

Correct merge strategies:

- **Fast-forward merge** — when `main` is a strict ancestor of the branch,
  `git push origin main` (or `gh pr merge --rebase`) advances `main` to the
  branch HEAD, preserving every commit. This is the default choice for
  execute-bead branches.
- **Merge commit** (`gh pr merge --merge`) — creates a 2-parent merge commit.
  Acceptable if you want an explicit boundary between the branch's work and
  main; still preserves all per-attempt commits in the branch's history.

Avoid:
- `gh pr merge --squash` — destroys per-attempt audit trail.
- `git rebase -i` with `fixup`/`drop` on `chore: update tracker` or
  `Merge bead` commits — same.
- `git filter-branch` / `git filter-repo` stripping tracker commits — same.

## References

- Full flag list: `ddx bead --help`, `ddx bead create --help`
- Tracker feature spec: `docs/helix/01-frame/features/FEAT-004-beads.md`

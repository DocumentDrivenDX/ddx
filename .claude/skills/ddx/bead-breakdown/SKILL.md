---
name: bead-breakdown
description: Break a spec, plan, epic, or free-form task into execution-ready beads by filing `ddx bead create` calls with inline context, acceptance criteria, and dependency wiring. Use when a governing artifact, approved plan, or epic needs to be translated into a queue of actionable beads.
---

# Bead Breakdown

A workflow skill for decomposing a spec, plan, epic, or free-form task into
execution-ready beads. Produces a set of `ddx bead create` calls with inline
context, acceptance criteria, and dependency edges — so the resulting beads can
be picked up cold by `ddx work` with no additional human narration.

## When to use

- **After plan approval** — translate an approved plan into a bead queue before
  implementation begins.
- **Epic decomposition** — break a large feature (FEAT-\*, milestone, user story)
  into child beads when the work exceeds one session or touches multiple
  subsystems.
- **Backlog creation** — file a batch of beads from a spec or roadmap document.
- **Effort-estimate follow-up** — when `effort-estimate` returns
  `NEEDS_BREAKDOWN`, use this skill to produce the child beads.

Single-session, single-subsystem work does not need breakdown — file one bead
directly. Reserve this skill for work that genuinely spans multiple files,
agents, or sessions.

## Workflow

### 1. Read the governing artifact

Before filing any beads, read the source material in full:

- The plan, spec, or epic (FEAT-\*, plan-\*.md, or free-form description).
- Any CONTRACT-\*, ADR-\*, or FEAT-\* artifacts it references.
- Existing beads that may already cover part of the scope
  (`ddx bead list --label area:<subsystem>`).

If a `spec-id` exists (FEAT-\* or similar), every child bead should carry it.

### 2. Identify the decomposition units

Each bead should:

- Touch at most ~3 files (strongly prefer fewer).
- Stay within one subsystem (CLI, storage, server, frontend, library, etc.).
- Have a single, testable outcome.
- Be completable in one agent session.

If the source spans multiple subsystems, create an **epic bead** first, then
file child beads that depend on it. The epic provides a stable `spec-id` anchor
for its children.

```
Epic: "Add structured bead review evidence"
├── Task: Render per-criterion verdict rows
├── Task: Attach review evidence to a bead
├── Task: Reject malformed verdicts without evidence
└── Task: Add integration test using script reviewer output
```

Name each unit with an imperative title that states the outcome, not the
activity. "Render per-criterion verdict rows" beats "Review work".

### 3. Draft each bead

For each unit, draft:

**Title** — imperative, specific, names the outcome.

**Description** — must stand alone. An isolated agent executing the bead has
no prior context. Include:

- What to change and why (inline the relevant facts; no links to ephemeral
  sources like chat or PR discussions).
- Root cause or motivation if it's a bug.
- File scope: explicit in-scope files and an out-of-scope list to prevent drift.
- Any implementation hint that narrows the search space (function name, line
  number, data structure, protocol detail).

**Acceptance criteria** — every criterion is a command that passes:

```
1. `cd cli && go test ./internal/bead/... -run TestReviewEvidence` passes.
2. `ddx bead review <id>` exits 0 and prints a structured findings table.
3. `ddx bead evidence add <id> --type review --body review.md` stores the
   review evidence.
```

"Works correctly" is not an AC. "Looks good" is not an AC. Each line must be
mechanically verifiable.

**Labels** — `area:<subsystem>` + `kind:<category>` at minimum. Add
`plan-<date>` when beads implement a specific plan.

**`spec-id`** — the governing FEAT-\* or plan identifier, when one exists.

**Type** — `task` for implementation, `bug` for defects, `epic` for
multi-bead containers, `chore` for non-functional work.

### 4. Wire dependencies

After filing all beads, wire the dependency DAG. A bead B that depends on A
must not enter the ready queue until A closes.

```bash
ddx bead dep add <B-id> <A-id>   # B depends on A
```

Common dependency patterns:

- **Sequential:** each bead depends on the previous (strict ordering).
- **Fan-out from epic:** all child tasks depend on the epic bead (or on each
  other where implementation order matters).
- **Shared precondition:** multiple tasks depend on a single setup task (schema
  migration, API contract, shared fixture).

Do not wire dependencies that do not exist. An unnecessary dep blocks work
without benefit.

### 5. Verify the queue shape

After filing, check the queue reflects the expected shape:

```bash
ddx bead list --label plan-<date>   # all beads for this plan
ddx bead ready                      # beads with satisfied deps
ddx bead status                     # derived dependency-waiting counts
ddx bead dep tree <epic-id>         # full dependency tree
```

Confirm that the beads with no predecessors appear in `ddx bead ready` and that
downstream beads appear as dependency-waiting in `ddx bead status` or the
dependency tree.

### 6. Commit the tracker state

Bead creation writes to `.ddx/beads.jsonl`. Commit after all beads and deps are
filed:

```bash
git add .ddx/beads.jsonl
git commit -m "chore: file beads for <plan or epic title>"
```

## Sizing heuristics

| Signal | Action |
|---|---|
| Bead touches > 3 files | Split into smaller beads |
| Bead spans 2+ subsystems | Split at the subsystem boundary |
| AC requires > 3 verifiable commands | Consider splitting |
| Estimated time > 4 hours (from `effort-estimate`) | Decompose before filing |
| Description needs > ~200 words to be self-contained | Usually too large |
| Bead has no testable AC | Under-specified; resolve before filing |

## Dispatch-based breakdown

For large specs where the breakdown itself is complex, dispatch the breakdown
to an agent:

```bash
# Write the breakdown prompt
cat > breakdown-target.md << 'EOF'
## Source

<paste plan excerpt or FEAT-* section>

## Governing artifacts

- FEAT-010: docs/helix/01-frame/features/FEAT-010-task-execution.md

## Breakdown question

Decompose the above into execution-ready beads. For each bead, produce:
- Imperative title
- Description (inline context, file scope, out-of-scope list)
- Acceptance criteria (commands that pass)
- Labels (area:*, kind:*)
- Dependencies on other beads in the set

Output the breakdown as a numbered list. Then output the `ddx bead create`
commands I should run to file them, in dependency order.
EOF

ddx run --harness claude --min-power 10 \
  --prompt breakdown-target.md \
  > breakdown.md
```

Review the agent output before running the `ddx bead create` commands. The
agent produces a draft; you own the final bead descriptions.

## Anti-patterns

- **Vague titles.** "Auth work" or "Fix stuff" — an agent executing cold cannot
  orient. Use the outcome as the title.
- **Description is a link.** "See the plan at docs/..." — the agent in an
  isolated worktree can read that file, but the bead must still be self-contained
  for the case where the file moves or is renamed. Inline the facts.
- **AC is a sentence.** "The feature should work." Gives the agent wiggle room
  to claim success without evidence. Every AC line is a command that exits 0 or
  a named assertion.
- **One bead per epic.** An epic bead is a container, not an implementation
  unit. Always file child task beads beneath it.
- **Missing out-of-scope list.** Without explicit scope negation, agents drift
  into adjacent cleanup. List what the bead does NOT touch.
- **Implicit dependencies.** "After the auth PR lands" — wire it with
  `ddx bead dep add`. The queue cannot enforce implicit ordering.
- **No tests in AC.** Implementation without a test command in AC means the
  bead can close without verification. Every implementation bead needs at least
  one test command.

## CLI reference

```bash
# Create a bead
ddx bead create "Title" --type task \
  --labels area:cli,kind:task,plan-2026-04-29 \
  --description "..." \
  --acceptance "1. go test ./... passes\n2. ddx bead list shows ..." \
  --set spec-id=FEAT-010

# Create an epic
ddx bead create "Epic: <title>" --type epic \
  --labels area:<subsystem>,kind:epic \
  --description "..."

# Wire dependencies (B depends on A)
ddx bead dep add <B-id> <A-id>

# Inspect the queue after filing
ddx bead list --label plan-2026-04-29
ddx bead ready
ddx bead status
ddx bead dep tree <epic-id>

# Commit the tracker state
git add .ddx/beads.jsonl
git commit -m "chore: file beads for <plan title>"

# Dispatch breakdown to an agent
ddx run --harness claude --min-power 10 \
  --prompt breakdown-target.md > breakdown.md
```

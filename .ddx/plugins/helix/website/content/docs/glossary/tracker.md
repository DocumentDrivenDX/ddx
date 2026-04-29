---
title: Tracker
weight: 6
prev: /docs/glossary/concepts
---

# Tracker

The built-in tracker is HELIX's execution layer. Issues are called **beads** and are stored in `.ddx/beads.jsonl`, managed via `ddx bead` subcommands.

## Beads

A bead is a work item — the atomic unit of tracked work in HELIX.

| Field | Purpose |
|-------|---------|
| **id** | Unique identifier (e.g., `hx-407ed8b8`) |
| **title** | Short description of the work |
| **type** | `task`, `chore`, `epic`, `decision` |
| **status** | `open`, `in_progress`, `closed`, `deferred` |
| **priority** | Numeric priority (lower = higher priority) |
| **labels** | Organizational tags for triage and traceability |
| **spec-id** | Reference to the governing artifact (e.g., `FEAT-001`, `TD-003`) |
| **parent** | Parent epic or issue ID |
| **deps** | Dependency list — issues that must close before this one is ready |
| **acceptance** | Deterministic criteria that define "done" |
| **description** | Full description, often prefixed with a [context digest](/docs/glossary/concepts#context-digest) |
| **notes** | Execution notes appended during work |

## Labels

Labels are organizational conventions for triage and traceability.

### Phase Labels

| Label | Meaning |
|-------|---------|
| `phase:frame` | Requirements and scoping work |
| `phase:design` | Architecture and design work |
| `phase:test` | Test creation work |
| `phase:build` | Implementation work |
| `phase:deploy` | Deployment and release work |
| `phase:iterate` | Review, improvement, and follow-up work |
| `phase:review` | Reconciliation or audit work |

### Kind Labels

| Label | Meaning |
|-------|---------|
| `kind:build` | Standard execution work |
| `kind:deploy` | Rollout work |
| `kind:backlog` | Prioritized follow-up |
| `kind:review` | Review or reconciliation |

### Area Labels

| Label | Meaning |
|-------|---------|
| `area:ui` | Frontend work |
| `area:api` | Backend/server work |
| `area:data` | Database/storage work |
| `area:infra` | Infrastructure/deployment work |
| `area:cli` | CLI tool work |

Area labels are required for [concern](/docs/glossary/concerns) filtering to work. `helix polish` assigns them to unlabeled beads.

### Traceability Labels

| Label | Meaning |
|-------|---------|
| `story:US-NNN` | Links to a user story |
| `feature:FEAT-NNN` | Links to a feature spec |
| `source:metrics` | Discovered from metric analysis |
| `source:learnings` | Discovered from operational learnings |
| `review-finding` | Created by a fresh-eyes review |

## Common Commands

```bash
# Initialize the tracker
ddx bead init

# Create an issue
ddx bead create "Title" --type task --labels helix,phase:build \
  --spec-id FEAT-001 --acceptance "tests pass"

# View queue health
ddx bead status

# List ready issues
ddx bead ready --json

# Claim and work an issue
ddx bead update <id> --claim

# View an issue
ddx bead show <id> --json

# View dependency tree
ddx bead dep tree <id>

# Add a dependency
ddx bead dep add <id> --blocked-by <other-id>

# Close an issue
ddx bead close <id>

# View blocked issues
ddx bead blocked --json
```

## Queue Control

The tracker queue drives the [supervisory loop](/docs/workflow):

1. `helix check` inspects the ready queue and recommends the next action
2. `helix build` claims and executes one ready issue
3. `helix run` chains check and build until the queue drains

### Ready Queue

An issue is **ready** when:
- Status is `open` (not `in_progress`, `closed`, or `deferred`)
- All dependencies are `closed`
- It is an execution issue (not `phase:review`)

### Queue Drain

When the ready queue is empty:
- `helix check` determines whether this is true exhaustion or a planning gap
- `ALIGN` means planning exists but the next work set is unclear
- `DESIGN` means design authority is missing
- `POLISH` means issues need refinement
- `STOP` means there is genuinely no more work

## spec-id

The `spec-id` field links a bead to its governing artifact. This is how HELIX maintains traceability from execution back to requirements.

Examples:
- `spec-id: FEAT-001` — governed by feature spec 001
- `spec-id: TD-003` — governed by technical design 003
- `spec-id: US-042` — governed by user story 042

The build action loads the governing artifact referenced by `spec-id` to understand what the issue should accomplish and verify the work against acceptance criteria.

## Dependency Management

Beads can declare dependencies on other beads:

```bash
# B cannot start until A is closed
ddx bead dep add B --blocked-by A

# View the full dependency tree
ddx bead dep tree <id>
```

Dependencies affect the ready queue — a bead with unresolved dependencies is not ready. The `helix check` action considers dependency chains when recommending which issue to work next.

# Feature: Beads (Work Item Tracker)

**ID:** FEAT-004
**Status:** Not Started
**Priority:** P0
**Owner:** DDx Team

## Overview

Beads are portable, ephemeral work items with metadata. The `ddx bead` command provides CRUD, dependency tracking, and JSONL interchange — giving AI agents and workflows a structured work queue without coupling to any specific methodology.

The name follows the `bd` (Dolt-backed) and `br` (SQLite-backed) convention: short, lowercase tool names for the same conceptual unit. DDx beads use JSONL-backed local storage; `bd` and `br` provide database-backed alternatives. All three interchange via JSONL.

## Problem Statement

**Current situation:** AI agents need a work queue to know what to do next. HELIX built its own tracker (`scripts/tracker.sh`) tightly coupled to HELIX workflow semantics (phase labels, spec-id enforcement). Other workflows have no reusable work-tracking primitive.

**Pain points:**
- No generic work-item tool — every workflow reinvents issue tracking
- HELIX tracker can't be used without HELIX's full label/validation requirements
- No portable format for carrying work items between projects and tools
- Agents can't discover ready work without workflow-specific logic

**Desired outcome:** A lightweight, workflow-agnostic bead tracker that any methodology can layer semantics on top of. HELIX adds phase labels; a kanban workflow adds columns; a plain project just tracks tasks.

## Requirements

### Functional

1. **Bead CRUD** (`ddx bead create/show/update/close`) — create, read, update, and close work items
2. **Listing and filtering** (`ddx bead list`) — filter by status, label, or custom predicates
3. **Dependency DAG** (`ddx bead dep add/remove/tree`) — declare ordering constraints between beads
4. **Ready queue** (`ddx bead ready`) — list open beads with all dependencies satisfied
5. **Blocked query** (`ddx bead blocked`) — list open beads with unsatisfied dependencies
6. **Status summary** (`ddx bead status`) — counts of open, closed, blocked, ready beads
7. **Import** (`ddx bead import`) — ingest beads from `bd`, `br`, or raw JSONL files
8. **Export** (`ddx bead export`) — write beads as JSONL for interchange with other tools
9. **Initialization** (`ddx bead init`) — create storage file and directory
10. **Validation hooks** — workflows register custom validators for create/update operations
11. **Unknown field preservation** — round-trip fields DDx doesn't know about (enables workflow-specific extensions)

### Non-Functional

- **Performance:** All local operations complete in <100ms for up to 10,000 beads
- **Portability:** No external dependencies beyond the `ddx` binary
- **Concurrency:** File-level locking prevents corruption from parallel writes
- **Interchange:** JSONL format compatible with `bd`, `br`, and HELIX tracker

## Bead Schema

| Field | Type | Required | Default | Notes |
|-------|------|----------|---------|-------|
| id | string | auto-generated | `bx-` + 8 hex chars | Prefix configurable |
| title | string | yes | — | Non-empty |
| type | string | no | `task` | Free-form (task, epic, bug, chore, etc.) |
| status | string | no | `open` | Enum: open, in_progress, closed |
| priority | int | no | 2 | Range 0-4 (0 = highest) |
| labels | []string | no | [] | Free-form, no enforcement |
| parent | string | no | "" | ID of parent bead |
| description | string | no | "" | |
| acceptance | string | no | "" | |
| deps | []string | no | [] | IDs of dependency beads |
| assignee | string | no | "" | |
| notes | string | no | "" | |
| created | datetime | auto | — | ISO-8601 UTC |
| updated | datetime | auto | — | ISO-8601 UTC |

Unknown fields in imported or existing beads are preserved on read/write. This allows HELIX to store `spec-id`, `execution-eligible`, `claimed-at`, `claimed-pid`, `superseded-by`, and `replaces` without DDx needing to understand them.

## Storage

- **Default path:** `.ddx/beads.jsonl` (one JSON object per line, sorted by id)
- **Configuration:** `DDX_BEAD_DIR` env var or `bead.dir` in `.ddx/config.yaml`
- **ID prefix:** `DDX_BEAD_PREFIX` env var or `bead.id_prefix` in config (default: `bx`)
- **Locking:** Directory-based lock at `.ddx/beads.lock/` with PID file and acquisition timestamp. Configurable timeout (default: 10s).

## Validation

**DDx base validation** (always applied):
- Title is non-empty
- Dependencies reference existing bead IDs
- No self-referential dependencies
- Priority is in valid range (0-4)
- Status is a recognized value

**Workflow validation hooks:** An executable at `.ddx/hooks/validate-bead-create` (and `validate-bead-update`) receives the bead JSON on stdin. Exit codes: 0 = ok, 1 = hard error (stderr = message, creation blocked), 2 = warning (stderr = message, creation proceeds).

## CLI Surface

```
ddx bead init
ddx bead create "Title" [--type T] [--priority N] [--labels L,L] [--acceptance A] [--parent ID] [--description D]
ddx bead show <id> [--json]
ddx bead update <id> [--title T] [--status S] [--priority N] [--labels L,L] [--acceptance A] [--assignee A] [--claim]
ddx bead close <id>
ddx bead list [--status S] [--label L] [--json]
ddx bead ready [--json]
ddx bead blocked [--json]
ddx bead status
ddx bead dep add <id> <dep-id>
ddx bead dep remove <id> <dep-id>
ddx bead dep tree [<id>]
ddx bead import [--from auto|bd|br|jsonl] [file]
ddx bead export [--stdout] [file]
```

## Server Endpoints (FEAT-002 integration)

| MCP Tool | HTTP Endpoint | Description |
|----------|--------------|-------------|
| `ddx_list_beads` | `GET /api/beads` | List beads with optional filters |
| `ddx_show_bead` | `GET /api/beads/:id` | Show one bead |
| `ddx_bead_ready` | `GET /api/beads/ready` | List ready beads |
| `ddx_bead_status` | `GET /api/beads/status` | Summary counts |

## User Stories

### US-020: Create and Manage Beads
**As a** developer tracking work items
**I want** to create, update, and close beads from the CLI
**So that** I have a lightweight work queue without external tools

**Acceptance Criteria:**
- Given I'm in a DDx project, when I run `ddx bead create "Fix auth bug" --type bug --priority 1`, then a bead is created with a unique ID and status `open`
- Given a bead exists, when I run `ddx bead update <id> --status in_progress --assignee me`, then the bead is updated
- Given a bead exists, when I run `ddx bead close <id>`, then its status is set to `closed`
- Given beads exist, when I run `ddx bead list --status open`, then only open beads are shown

### US-021: Track Dependencies Between Beads
**As a** developer with ordered work
**I want** to declare that bead A must complete before bead B
**So that** the ready queue reflects actual execution order

**Acceptance Criteria:**
- Given beads A and B exist, when I run `ddx bead dep add B A`, then B depends on A
- Given B depends on open A, when I run `ddx bead ready`, then B is not listed
- Given B depends on A and A is closed, when I run `ddx bead ready`, then B is listed
- Given I run `ddx bead dep tree`, then I see the full dependency graph

### US-022: Import Beads from External Sources
**As a** developer migrating from another tracker
**I want** to import beads from `bd`, `br`, or JSONL files
**So that** I can consolidate work items into DDx

**Acceptance Criteria:**
- Given a `.beads/issues.jsonl` file exists, when I run `ddx bead import --from jsonl`, then beads are imported preserving all fields
- Given `bd` is available and has data, when I run `ddx bead import --from bd`, then beads are imported
- Given I run `ddx bead export --stdout`, then JSONL output is compatible with `ddx bead import --from jsonl`

### US-023: Agent Queries Ready Beads via MCP
**As an** AI agent with MCP tool access
**I want** to query the ready queue programmatically
**So that** I can pick up the next actionable work item

**Acceptance Criteria:**
- Given ddx-server is running with beads, when an agent calls `ddx_bead_ready`, then it receives ready beads as structured JSON
- Given an agent calls `ddx_show_bead` with an ID, then it receives the full bead including all fields (known and unknown)

## Edge Cases

- Create when storage doesn't exist — auto-initialize on first write
- Import with duplicate IDs — skip duplicates, report count
- Dep add with nonexistent target — error with clear message
- Concurrent writes — locking prevents corruption, second writer waits or times out
- Export when no beads exist — produce empty JSONL file
- Unknown fields in imported data — preserve on round-trip without validation

## Dependencies

- `.ddx/` directory (created by `ddx init` or `ddx bead init`)
- `bd` / `br` only required for `--from bd` / `--from br` import

## Out of Scope

- Workflow-specific validation (phase labels, spec-id) — that's HELIX or other workflows via hooks
- Supervisory execution loops — that's workflow-level orchestration
- Real-time sync between bead stores — use import/export for interchange
- Web UI for bead management — use CLI or server API

---
ddx:
  id: FEAT-004
  depends_on:
    - helix.prd
---
# Feature: Beads (Work Item Tracker)

**ID:** FEAT-004
**Status:** Complete
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
11. **Claim ownership** (`ddx bead update <id> --claim [--assignee A]`) — claim a bead with explicit assignee control and stable claim metadata
12. **Execution evidence** (`ddx bead evidence add/list`) — append-only history for close summaries, agent outputs, and experiment outcomes
13. **Unknown field preservation** — round-trip fields DDx doesn't know about (enables workflow-specific extensions)

### Non-Functional

- **Performance:** All local operations complete in <100ms for up to 10,000 beads. Read-heavy queue commands (`list`, `ready`, `blocked`, `status`) operate on one parsed snapshot and remain deterministic under repeated calls.
- **Portability:** No external dependencies beyond the `ddx` binary
- **Concurrency:** File-level locking prevents corruption from parallel writes. Writers use temp-file + atomic rename semantics, and repair flows keep a backup before swapping in a cleaned file.
- **Recovery:** Malformed or partially written JSONL records are handled best-effort. Valid records are preserved, malformed lines are reported with line numbers, and a `.bak` backup is kept when the store auto-repairs a file.
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

DDx beads abstracts over multiple storage backends. Consumers (HELIX, dun, other workflows) **must only interact via `ddx bead` commands or the server API** — never read/write the storage file directly.

### Backend Selection

| Backend | Config Value | When To Use |
|---------|-------------|-------------|
| `jsonl` (default) | `bead.backend: jsonl` | Simple projects, single-agent, no external tooling |
| `bd` | `bead.backend: bd` | Dolt-backed bead store (collaborative, branching) |
| `br` | `bead.backend: br` | SQLite-backed bead store (local performance) |

Backend is configured in `.ddx/config.yaml` or via `DDX_BEAD_BACKEND` env var. Default: `jsonl`.

For `bd` and `br` backends, DDx shells out to the respective binary. For `jsonl`, DDx manages the file directly.

### JSONL Backend Details

- **Default path:** `.ddx/beads.jsonl` (one JSON object per line, sorted by id)
- **Configuration:** `DDX_BEAD_DIR` env var or `bead.dir` in `.ddx/config.yaml` (default: `.ddx`)
- **Locking:** Directory-based lock at `.ddx/beads.lock/` with PID file and acquisition timestamp. Configurable timeout (default: 10s).
- **Write algorithm:** Mutating operations take the lock, build a complete bead snapshot, write to `beads.jsonl.tmp`, and atomically rename the temp file into place. This avoids partially written tracker state.
- **Read algorithm:** Queue commands scan `beads.jsonl` line-by-line. Valid JSON objects are loaded into a snapshot; malformed lines are skipped with line-numbered warnings so one bad record does not take down the entire queue.
- **Repair algorithm:** If a read finds malformed lines but at least one valid bead, DDx takes the store lock, copies the current file to `.ddx/beads.jsonl.bak`, and rewrites the repaired snapshot atomically.
- **Failure mode:** If every line is malformed, the command returns a contextual error that names the file and malformed-record count.
- **Git integration:** `beads.jsonl` is project state and must be committed to git. Auto-commit behavior after bead mutations is defined in FEAT-012 (Git Awareness), requirements S7 and US-124. `ddx init` generates agent guidance that includes `beads.jsonl` as a committable file (FEAT-012 S8, US-125).
- **Design reference:** See [`SD-004-beads-tracker.md`](../../02-design/solution-designs/SD-004-beads-tracker.md), [`TD-004-beads-claims-evidence.md`](../../02-design/technical-designs/TD-004-beads-claims-evidence.md), and [`TP-004-beads-claims-evidence.md`](../../03-test/test-plans/TP-004-beads-claims-evidence.md) for the concrete algorithms, repair flow, claim semantics, evidence trail, and validation matrix.

### Storage Boundary

The bead backend is the reusable DDx storage engine for bead-schema records.
The primary work queue is one collection, not the only possible collection.

- The default active-work collection maps to `beads.jsonl` in the JSONL backend.
- Other DDx services may use separate bead-backed collections for archived
  work, execution history, agent session indexes, or other record families.
- Large payloads such as prompt bodies or raw logs live in separate attachment
  files referenced from the bead-schema record rather than inline in the
  primary collection row.
- Backend selection is a store-level concern, not a single-file concern.

### ID Prefix

Following the bd convention, the prefix is **derived from the repository name** by default:

- **Auto-detected:** git repo root directory name (e.g., repo `my-project` → prefix `my-project` → IDs like `my-project-a3f2dd`)
- **Override:** `bead.id_prefix` in `.ddx/config.yaml` or `DDX_BEAD_PREFIX` env var
- **Fallback:** `bx` if repo name detection fails
- **Format:** `<prefix>-` + 8 hex chars

This matches how bd derives its prefix, ensuring beads created by DDx and bd in the same repo use the same naming convention.

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
ddx bead evidence add <id> [--kind K] [--body B] [--summary S] [--source SRC] [--actor A]
ddx bead evidence list <id> [--json]
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

### US-024: Operator Recovers From Partial JSONL Corruption
**As a** repo operator
**I want** bead queue commands to survive a partially written or externally edited `beads.jsonl`
**So that** one bad record does not block the whole queue

**Acceptance Criteria:**
- Given `beads.jsonl` contains one malformed record and one valid record, when I run `ddx bead ready` or `ddx bead status`, then DDx reports the malformed line number, preserves the valid record, and rewrites the cleaned file atomically
- Given DDx auto-repairs a partially corrupted file, then it keeps a `.bak` backup of the original contents
- Given `beads.jsonl` contains only malformed records, then queue reads fail with a contextual error that includes the file path and malformed-record count

### US-025: Operator Claims a Bead With a Specific Assignee
**As a** developer or agent coordinating work
**I want** to claim a bead and assign it to a known actor
**So that** ownership is explicit and not hardcoded to DDx

**Acceptance Criteria:**
- Given a bead exists, when I run `ddx bead update <id> --claim --assignee alice`, then the bead becomes `in_progress`, `assignee` is `alice`, and `claimed-at` / `claimed-pid` are recorded
- Given a bead exists, when I run `ddx bead update <id> --claim` without `--assignee`, then DDx uses the runtime caller identity fallback before defaulting to `ddx`
- Given a claimed bead exists, when I run `ddx bead update <id> --unclaim`, then claim metadata is cleared and the bead returns to `open`

### US-026: Operator Appends Execution Evidence
**As a** developer or agent
**I want** to append evidence to a bead without rewriting prior history
**So that** close summaries and experiment outcomes remain auditable

**Acceptance Criteria:**
- Given a bead exists, when I run `ddx bead evidence add <id> --kind summary --body "done"`, then a new immutable event is appended with a timestamp and actor
- Given evidence already exists, when I append another event, then prior entries remain in order and are not rewritten
- Given I run `ddx bead show <id> --json`, then the full evidence history is returned
- Given I run `ddx bead list`, `ready`, `blocked`, or `status`, then evidence entries do not change queue semantics

## Claim Semantics

Beads support advisory ownership claims for agent/workflow coordination:

- `ddx bead update <id> --claim [--assignee NAME]` sets `status=in_progress`, resolves `assignee` from the explicit flag or runtime caller identity, and records `claimed-at` (ISO-8601 UTC) plus `claimed-pid` (current PID)
- `ddx bead update <id> --unclaim` sets `status=open`, clears `assignee`, `claimed-at`, `claimed-pid`
- Claims are advisory — they prevent double-claiming, not hard locks
- `claimed-at` and `claimed-pid` are standard fields (not unknown-field extensions)

Claim resolution order is:

1. Explicit `--assignee` flag.
2. Runtime caller identity from the execution context.
3. `ddx` as the final fallback.

This makes claim ownership explicit for both human operators and agents without hardcoding the caller to the CLI binary name.

## Execution Evidence

Beads carry append-only execution evidence in workflow-specific metadata at `Extra["events"]`.

- Each event records `kind`, `summary`, `body`, `actor`, `created_at`, and `source`.
- `kind` covers common workflow events such as `claim`, `unclaim`, `close`, `summary`, and `experiment`.
- Events are appended in order and never rewritten or removed by normal CLI operations.
- Evidence is visible in `show --json` and server/API responses via the `events` metadata field, but queue derivation ignores it.
- Workflows may use the evidence trail for operator notes, experiment results, or close summaries without changing the bead's core status semantics.

## Custom Fields

Workflows need to store fields DDx doesn't know about (e.g., HELIX stores `spec-id`, `execution-eligible`, `superseded-by`).

- `ddx bead create --set key=value` — set arbitrary field on create
- `ddx bead update <id> --set key=value` — set arbitrary field on update
- `ddx bead update <id> --set closing_commit_sha=<sha>` — repair closing provenance on a closed bead without editing storage directly
- `ddx bead list --where key=value` — filter by custom field
- Unknown fields are preserved on all read/write operations
- Custom fields appear in JSON output and import/export
- The evidence trail is stored as workflow-specific metadata in `Extra["events"]`, preserving bd/br compatibility while still round-tripping the full history.

This keeps DDx agnostic while giving workflows a typed pass-through mechanism.

## Validation Hooks

Workflows register custom validators as executables:

- `.ddx/hooks/validate-bead-create` — called on create, receives bead JSON on stdin
- `.ddx/hooks/validate-bead-update` — called on update, receives bead JSON on stdin
- Exit 0 = ok, Exit 1 = hard error (creation blocked, stderr = message), Exit 2 = warning (proceeds, stderr = message)

Example: HELIX installs a hook requiring `spec-id` on task-type beads and `acceptance` on task/epic beads.

## Edge Cases

- Create when storage doesn't exist — auto-initialize on first write
- Import with duplicate IDs — skip duplicates, report count
- Dep add with nonexistent target — error with clear message
- Concurrent writes — locking prevents corruption, second writer waits or times out
- Partially corrupted JSONL — valid records are preserved, malformed lines are skipped with contextual warnings, and auto-repair keeps a `.bak` backup before atomic replacement
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

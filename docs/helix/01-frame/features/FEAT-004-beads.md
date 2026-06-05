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
4. **Ready queue** (`ddx bead ready`) — list `status=open` beads with all dependencies satisfied and no execution-suppressing metadata
5. **Dependency-waiting query** (`ddx bead blocked`) — list `status=open` beads with unsatisfied dependencies; this is derived waiting, not `status=blocked`
6. **Status summary** (`ddx bead status`) — counts persisted-status values and derived buckets such as ready and dependency-waiting
7. **Import** (`ddx bead import`) — ingest beads from `bd`, `br`, or raw JSONL files
8. **Export** (`ddx bead export`) — write beads as JSONL for interchange with other tools
9. **Initialization** (`ddx bead init`) — create storage file and directory
10. **Validation hooks** — workflows register custom validators for create/update operations
11. **Claim ownership** (`ddx bead update <id> --claim [--assignee A]`) — claim a bead with explicit assignee control and stable claim metadata
12. **Execution evidence** (`ddx bead evidence add/list`) — append-only history for close summaries, agent outputs, and experiment outcomes
13. **Unknown field preservation** — round-trip fields DDx doesn't know about (enables workflow-specific extensions)
14. **In-priority queue ordering override** (`ddx bead queue ...`) — let operators move a bead within its existing priority bucket without changing priority or extending the core bead schema

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
| status | string | no | `open` | Enum: open, in_progress, closed, blocked, proposed, cancelled (bd/br canonical set; see TD-027 §1) |
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

Unknown fields in imported or existing beads are preserved on read/write. This allows HELIX to store `spec-id`, `execution-eligible`, `claimed-at`, `claimed-pid`, `superseded-by`, `replaces`, and DDx-specific queue metadata such as `queue-rank` without extending the core bd/br-compatible schema table.

Operator-attention work uses `status=proposed` when a bead needs a human
decision before autonomous execution. Legacy/backcompat `needs_human` labels
and legacy/backcompat `needs-human-*` preserved `Extra` fields may be read
during migration, but they are metadata only; they do not define a queue lane or
suppress routing after the status-owned lifecycle contract in TD-027 is applied.

TD-027 §3 (Category Taxonomy) defines how lifecycle actions use the existing carriers: persisted
bd/br statuses, labels, dependency edges, append-only events, and preserved
`Extra` fields. FEAT-004 does not introduce additional status vocabulary for
no_changes, cooldown, superseded, or execution-readiness cases.

### Lifecycle Migration Gate

The status-owned lifecycle migration is one-way. Legacy/backcompat
`needs_human`, `triage:needs-investigation`, and pseudo-status lifecycle names
are input to `ddx bead migrate --lifecycle`; normal runtime does not preserve
them as compatibility queue lanes.

Before queue commands derive readiness or mutate beads, DDx performs the TD-027
lifecycle startup preflight. If the active store still contains unmigrated
legacy lifecycle labels or pseudo-statuses, `ddx bead ready`, `ddx bead
blocked`, `ddx bead status`, `ddx work`, and worker/API queue-readiness surfaces
must refuse normal operation. The permitted bead commands during this state are
the migration path and diagnostics:

```bash
ddx bead migrate --lifecycle --dry-run
ddx bead migrate --lifecycle --apply
```

The refusal output must include counts by legacy label/pseudo-status, a sample
of affected bead IDs, and the exact migration command to run. Because beads are
git-tracked, recovery from an incorrect one-way migration is git rollback of
the tracker commit rather than maintaining dual lifecycle semantics.

Bead readiness assessment uses those existing metadata carriers. Lint/rubric
scoring happens inside readiness, and post-attempt triage happens after
execution. None of them add dedicated readiness fields or new schema fields;
readiness is derived from the bead record's existing title, description,
acceptance, labels, parent, dependencies, claim metadata, and preserved
extras.

### Queue Semantics For Epics

Epics remain first-class beads, but they are not consumed by the same
queue-drain contract as ordinary executable task/bug/chore beads.

- A normal execution-ready queue is **single-ticket-first**. Ready non-epic
  beads are ordered ahead of ready epic beads at the same priority.
- Open epics are not launched by the ordinary `ddx work` single-ticket worker.
  Their disposition is decided by the **idle-path auto-remediator** (see FEAT-010
  Layer 3 — Idle-Path Diagnosis and Auto-Remediation), which classifies each
  ready epic by its open/closed child counts, parent/dep state, and operator
  override labels, then fires exactly one of these auto-actions per loop pass:
  - **Closure cascade** when all children are **terminal** — `closed` or
    `cancelled` (`nonTerminalChildCount == 0 && totalChildCount > 0`): the
    epic is auto-closed via the closure-evaluation path. `cancelled` counts
    as terminal because a cancelled child is deliberately abandoned and "will
    not run" (status semantics, TD-027 §1) — it must not hold its epic open
    forever. If **every** child is `cancelled` (none `closed`), the epic is
    itself set to `cancelled` rather than `closed`, since no work actually
    completed. The cascade fires from **both** the child-close path
    (immediately when the last child closes — via `Close()` *and*
    `CloseWithEvidence`, so autonomous worker drains settle the epic without
    waiting for an idle pass) **and** the idle-path remediator (for children
    closed outside the worker — imports, manual edits, federation sync). It
    **recurses the full parent chain**: closing the last leaf of a nested
    epic-of-epics settles every ancestor epic in one operation, bounded by a
    visited-set cycle guard.
  - **Supersession cascade** when an open child carries `superseded-by:<Y>`
    and Y is closed, with no other reason for the child to stay open: the
    child auto-closes via a one-hop cascade inside `Close()`.
  - **Auto-decomposition** when the epic has `totalChildCount == 0`, has a
    valid decomposition source (description with PROBLEM/PROPOSED FIX/AC
    sections, `spec:*` or `area:*` label), and has not exhausted its
    per-bead attempt cap or `--max-recovery-cost` budget: the in-loop
    pre-claim decomposer is dispatched against the epic.
  - **Operator surface** when none of the above applies (real dependency
    block on open work, parent/child state conflict, exhausted attempts,
    operator-set `manual-hold` / `no-auto-decompose` / `container` label,
    or unknown state): the epic is listed in `ddx work focus` Section A
    (operator-required). Beads with an auto-action queued for the next loop
    pass are surfaced in `ddx work focus` Section B (count with `--verbose`
    detail) rather than silently hidden.
- Child beads of an epic remain individually executable units and may be
  closed one-by-one.
- Epic queue entries remain visible in the tracker and UI. The diagnostic
  surface `ddx work plan --explain` prints the per-epic reason code so
  operators can see what the loop will do on its next iteration.

The "skip and exit" disposition (`No execution-eligible beads in the queue`
with only ready epics present) is forbidden by the FEAT-010 reliability
contract; every ready epic must classify into one of the four auto-actions
above. Concurrency, cycle guards, idempotency, and audit-event requirements
are owned by FEAT-010 Layer 3 and are common to every auto-remediation in the
idle path.

**Deferred** (out of scope for this contract): an epic-scoped worker mode
with a dedicated epic branch/worktree that sequentially executes child beads
on a shared branch. That capability is a separate, larger initiative and is
not required for the queue-drain contract to hold.

### Queue Ordering Overrides

The canonical ready-queue order is:

1. `priority` ascending (`0` first, `4` last)
2. explicit `queue-rank` ascending, with missing `queue-rank` sorted after
   explicit ranks inside the same priority bucket
3. `created_at` ascending
4. `id` ascending

`queue-rank` is preserved extension metadata stored in the bead's unknown-field
map, not a core bead schema field. It is an operator override for ordering
within one priority bucket only. A ranked `P1` bead never sorts ahead of an
unranked `P0` bead, and a ranked bead that is blocked, on retry cooldown,
superseded, `execution-eligible=false`, or epic-only/container work remains
excluded from `ddx work`'s execution-ready picker.

The CLI exposes queue movement as a first-class surface rather than requiring
operators to manage raw metadata:

- `ddx bead queue top <id>` assigns a rank that places the bead first among
  ready beads with the same priority.
- `ddx bead queue move <id> --before <other-id>` places the bead before another
  bead in the same priority bucket.
- `ddx bead queue move <id> --after <other-id>` places the bead after another
  bead in the same priority bucket.
- `ddx bead queue clear <id>` removes the explicit rank and restores the
  default tie-break ordering for that bead.

`queue move --before/--after` fails when the two beads have different
priorities. Operators that want to change urgency must use `ddx bead update
<id> --priority N` explicitly. Queue-rank values are canonicalized as integers;
read paths may accept numeric strings for compatibility, but writes persist a
number. Rank calculation should use sparse integer values and renormalize only
the affected priority bucket when no midpoint exists.

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

### Bead Readiness And Authoring Quality

ADR-023 adds lifecycle-quality hooks on top of the existing validation-hook
surface. The canonical product concept is bead readiness assessment: the
pre-claim decision about whether a bead is tractable and actionable. The same
policy surface also includes lint/rubric scoring inside readiness, which
measures prompt quality, and post-attempt triage, which classifies evidence
after execution. The older "pre-claim intake" wording survives only as legacy
compatibility language, not normative terminology. Base validation still
protects the reusable bead schema; authoring quality checks protect the
"bead as prompt" contract needed by `ddx try`, `ddx work`, and autonomous
sub-agent execution. Readiness uses the existing bead metadata carriers and
does not add schema fields.

The lint rubric is the 8-criterion template in
`docs/helix/06-iterate/bead-authoring-template.md`:

- title is scoped and imperative
- description contains problem, root cause with file:line, proposed fix, and
  non-scope
- acceptance criteria are numbered, verifiable, and name concrete tests or
  observable artifacts when tests apply
- new code paths include wired-in assertions
- acceptance criteria name the applicable `go test` command and
  `lefthook run pre-commit`
- labels include phase, area, kind, and cross-references
- parent and dependencies are explicit
- the bead body is sufficient for a competent sub-agent to execute without
  asking for operator context

Bead readiness assessment, lint/rubric scoring, and post-attempt triage all
invoke the nested bead-lifecycle skill under the `ddx` skill tree. DDx owns
passing bead JSON, mode, waiver labels, and evidence paths into the hook; the
skill owns producing human-readable criterion findings. Hook output is
ephemeral execution evidence, not durable bead schema. For `ddx try` and
`ddx work`, the readiness and lint reports are stored under the attempt
evidence directory alongside the prompt, result, checks, and triage records.
`beads.jsonl` is not extended with lint-score fields.

Waiver storage uses existing labels. The durable form is
`lint-waiver:<criterion>`, such as `lint-waiver:c` for the concrete-test-name
criterion. Built-in rubric skips come from the authoring template: doc-only
beads may skip test-name and wired-in criteria, epic beads may satisfy those
criteria through children, and deletion/rename beads may skip wired-in checks
when behavior preservation is asserted.

Manual override is explicit and audited. When an operator dispatches with
`--force --reason <text>`, DDx appends an evidence event recording the actor,
reason, mode, waived criteria, and lint summary. The override does not mutate
the bead schema and does not suppress future lint runs.

WARN-ONLY is the default mode: lint findings are reported but create/update and
dispatch proceed. BLOCK mode is opt-in and may block dispatch only after valid
lint output, rubric skips, and label waivers have been applied. Hook
infrastructure failures follow ADR-023's fail-open rule and are reported as
warnings, not schema validation failures.

## CLI Surface

```
ddx bead init
ddx bead create "Title" [--type T] [--priority N] [--labels L,L] [--acceptance A] [--parent ID] [--description D]
ddx bead show <id> [--json]
ddx bead update <id> [--title T] [--status S] [--priority N] [--labels L,L] [--acceptance A] [--assignee A] [--claim]
ddx bead queue top <id>
ddx bead queue move <id> [--before OTHER | --after OTHER]
ddx bead queue clear <id>
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

### US-023a: Operator Reorders Work Within Priority
**As an** operator managing the ready queue
**I want** to move a bead ahead of other beads with the same priority
**So that** I can express immediate sequence without changing urgency or rewriting the core bead schema

**Acceptance Criteria:**
- Given two ready `P0` beads exist, when I run `ddx bead queue top <later-id>`, then `ddx bead ready --execution` lists `<later-id>` before the other `P0` bead
- Given a `P1` bead has `queue-rank=0` and a `P0` bead has no `queue-rank`, when I run `ddx bead ready --execution`, then the `P0` bead remains before the `P1` bead
- Given two beads with different priorities, when I run `ddx bead queue move <id> --before <other-id>`, then DDx returns an error explaining that queue moves are limited to one priority bucket
- Given a ranked bead is blocked, superseded, on retry cooldown, `execution-eligible=false`, or an epic-only/container bead, then `ddx bead ready --execution` still excludes it
- Given a ranked bead exists, when I run `ddx bead queue clear <id>`, then DDx removes `queue-rank` and the bead returns to the default priority/created/id ordering

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

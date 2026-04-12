---
ddx:
  id: AR-2026-04-04
  depends_on:
    - helix.prd
    - FEAT-001
    - FEAT-002
    - FEAT-003
    - FEAT-004
    - FEAT-005
    - FEAT-006
    - FEAT-007
    - FEAT-008
    - FEAT-009
    - FEAT-010
    - FEAT-011
    - FEAT-012
    - FEAT-013
    - FEAT-014
    - FEAT-015
    - FEAT-016
    - FEAT-018
    - FEAT-019
---
# DDx Architecture

DDx is a local-first development platform with three cooperating layers:

1. **Command layer** (`ddx` CLI)
2. **Execution layer** (agent, execution, registry, tracker, and document engines)
3. **Service layer** (`ddx server`) with API, MCP, and embedded UI surfaces

All data is file-backed with optional local cache and generated artifacts.

## Design Goals

- Keep runtime behavior deterministic and testable from filesystem fixtures.
- Minimize coordination overhead: one binary on one machine with project-scoped adapters and registry-backed server routing.
- Preserve feature boundaries from the existing spec set (docs, agents, beads, registry, server, web).
- Make governance explicit through `.ddx/config.yaml`, docs frontmatter, and tracker records.

## Logical Architecture

```text
                     +----------------------------+
                     |        CLI Entrypoint       |
                     |   (cobra command graph)     |
                     +--------------+-------------+
                                    |
                     +--------------v-------------+
                     |  Command Service Adapters  |
                     |   - doc                 |
                     |   - bead                 |
                     |   - agent                |
                     |   - exec                 |
                     |   - update/install/verify |
                     +--------------+-------------+
                                    |
        +---------------------------+---------------------------+
        |                           |                           |
        v                           v                           v
 +-------------+           +------------------+           +------------------+
 |  Doc Engine |           |  Tracker Engine   |          | Registry Engine  |
 | docgraph    |           | beads (JSONL)     |          | install/search   |
+-------------+           +------------------+           +------------------+
        |                           |                           |
        +-------------+-------------+-------------+-------------+
                      |                           |
                      v                           v
               +------------------+         +------------------+
               |   `.ddx/`        |         | `.ddx/lock.yaml` |
               | project artifacts |         | integrity state  |
               +------------------+         +------------------+
                      |
                      +------------------------> Server Adapter --> HTTP/MCP/UI
                                                (`cli/cmd/server.go`,
                                                 `cli/internal/server/*`)

```

## Command Plane

The CLI is the primary orchestration surface.

- `cli/cmd` defines command trees and option semantics.
- Each command delegates to internal services instead of doing heavy logic inline.
- `.ddx/config.yaml` is read once at run time by command adapters where relevant
  (for example, agent defaults and server binding).
- Server-related command (`ddx server`) reuses the same data adapters used by
  REST/MCP handlers.

This keeps CLI and server behavior aligned and reduces duplicated contract code.

## Execution Engines

### 1) Document graph engine (`internal/docgraph`)

`docgraph` computes deterministic document dependencies from frontmatter and
frontmatter hashes. It is the shared read model for:

- CLI doc commands (`ddx doc *`)
- Server document APIs (`/api/docs/*`)
- future web visualizations
- future migration and validation tooling

Engine duties:
- parse frontmatter and resolve `depends_on`
- compute full graph + transitive descendants
- compute staleness and stale frontier
- write/update doc stamps for repeatable incremental validation

### 2) Bead tracker engine (`internal/beads`)

Beads are append-only JSONL records with explicit dependency links and status.
The engine supports:
- ready/blocked derivation for automation ordering
- dependency closure queries
- tracker health summaries and status transitions
- persistence compatible with existing CLI workflows and server read APIs

### 3) Agent engine (`internal/agent`)

Agent execution remains CLI-owned:
- agent command constructs a runner config from CLI args, environment, and `.ddx/config.yaml`
- routing is intent-first for normal use: profile/model/effort selectors resolve
  to a candidate harness plan before invocation
- DDx consumes the shared `ddx-agent` model catalog for aliases, profiles,
  canonical targets, and deprecation metadata across harness surfaces
- DDx owns cross-harness candidate planning, ranking, and rejection
  decisions; the embedded `ddx-agent` runtime owns provider/backend selection
  after DDx chooses the embedded harness
- capability introspection and doctor surfaces expose routing-relevant harness
  capability and state, not only static model defaults
- session evidence is written through a dedicated bead-backed
  `agent-sessions` collection with separate prompt/response/log attachments,
  and is surfaced read-only by server

### 4) Execution engine (`internal/exec`)

Execution remains artifact-generic:
- execution definitions and runs live in dedicated bead-backed collections
  (`exec-definitions` and `exec-runs`).
- for the JSONL backend, those collections map to `.ddx/exec-definitions.jsonl`
  and `.ddx/exec-runs.jsonl`; large result or log payloads live in
  `.ddx/exec-runs.d/<run-id>/`.
- other backends preserve the same logical collections while owning their
  physical storage layout.
- command handlers own validation, execution, result inspection, and history
  queries.
- artifact-linked specializations, such as metrics, project filtered read
  models over the shared execution store.

The execution engine does not interpret HELIX stage progression. It resolves
linked artifacts, loads the execution definition, invokes the configured
executor, and records the run with explicit references back to governing
artifact and definition IDs.

Both the agent and execution engines use the same storage pattern for durable
evidence: a bead-backed metadata collection plus named attachment files for
large captured bodies. This avoids forcing prompt, response, and log payloads
into one shared row while preserving a uniform inspection model and backend
family.

## Service Plane

`ddx server` is a one-process, three-surface, multi-project host:

- `/` serves embedded frontend assets (FEAT-008 runtime target).
- `/api/` exposes JSON REST endpoints backed by the same internal services as CLI.
- `/mcp/` exposes Streamable HTTP JSON-RPC tools for the same domain operations.
- `server.projects` or an implicit singleton cwd project determines which project context each request resolves against.

Server policy:
- JSON contracts are centralized so CLI and server share response/field expectations.
- Request handling is stateless per project context and filesystem-backed.
- Mutating actions are blocked by default unless explicitly intended by design.

## Deployment Topology

Default deployment is single-host, local-first:

- Local users run the binary directly.
- `ddx` and `ddx server` operate on the repository or registry of repositories being served.
- No central database is required for v1.
- Shared state lives in project-local `.ddx/` paths and local cache directories.
- One server process can serve multiple project roots on the same host, but a request is always resolved against exactly one project context.

```text
Developer Laptop
├─ repo-a/
│  ├─ .ddx/
│  └─ docs/
├─ repo-b/
│  ├─ .ddx/
│  └─ docs/
└─ ~/.cache/ddx/
   └─ server/
      ├─ repo-a/
      └─ repo-b/
```

## Data Governance and Integrity

- `ddx` docs and beads rely on explicit frontmatter and JSONL structure.
- Doc changes are validated against deterministic hash signatures to support drift detection.
- Registry operations record source, commit/tree references, and file hashes in
  `.ddx/lock.yaml`.
- Agent sessions, when enabled, remain append-only logs for auditability.
- Server registry state is derived from config plus explicit project roots and never inferred from sibling project caches.

## Security and Operability Boundaries

- CLI commands and server are expected to run under user context; high-impact actions are local-file scoped.
- Remote server access is opt-in and configurable (address + optional auth policy in config).
- Sensitive data (secrets, tokens) should stay outside committed project files; config supports environment override patterns.

## Failure Modes

- **Missing docgraph node dependency**: fail fast with actionable path and expected dependency id.
- **Graph/ tracker drift**: surface stale reasons with enough context to repair upstream docs first.
- **Registry mismatch**: fail install/search operations with explicit source and expected hash information.
- **Config schema mismatch**: reject unknown keys in known sections and suggest supported keys.
- **Project registry entry mismatch**: fail the affected project entry, keep sibling project contexts available, and report the invalid or missing root.
- **Project registry shape error**: duplicate project ids are a startup/configuration error and fail fast before serving partial context.

## Migration Path

- Current behavior uses existing CLI-first implementation and existing command surfaces.
- The first multi-project server step adds project-scoped routing on top of the current singleton model without changing repository-local data formats.
- The architecture incrementally supports moving from docgraph implementation parity to full FEAT-002 and FEAT-008 delivery without rewriting service internals.
- FEAT-007 migration tooling (`ddx doc migrate`) can operate against historical docs to convert `dun:` metadata toward `ddx:` without changing runtime semantics.

---
ddx:
  id: FEAT-010
  depends_on:
    - helix.prd
    - FEAT-005
    - FEAT-006
---
# Feature: Executions (Definitions and Runs)

**ID:** FEAT-010
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx executions provide a workflow-agnostic way to run repository-local
operations, capture their raw logs, normalize their structured outputs, and
retain immutable run history over time.

Artifacts define what matters and how it relates to the rest of the document
graph. Execution definitions define how an artifact or group of artifacts is
evaluated. Execution runs record what happened when that definition was
invoked.

This gives DDx a reusable runtime substrate for metrics, acceptance checks,
test-case evaluation, and other evidence-producing operations without teaching
DDx about HELIX phases or workflow-specific actions.

## Problem Statement

**Current situation:** DDx already knows artifacts and their relationships, and
it already has specialized runtime surfaces such as `ddx agent`. But there is
no generic execution primitive for non-agent operations or for linking runtime
evidence back to artifacts in a uniform way.

**Pain points:**
- Artifact documents get overloaded with runtime details that do not belong in
  the authority graph
- Metrics, acceptance checks, and similar operations fall back to ad hoc files,
  one-off scripts, or tool-specific logs
- There is no shared way to capture raw logs and structured result data for one
  execution
- There is no durable, queryable run history linked to artifact IDs
- Agent-backed and command-backed evidence do not share a common execution
  record model

**Desired outcome:** A generic `ddx exec` surface that stores machine-readable
execution definitions, runs them reproducibly, captures logs and structured
results, and persists immutable history linked to artifacts by ID.

## Execution Model

DDx separates three concerns:

1. **Artifact** — a declarative node in the document graph (`MET-*`, `FEAT-*`,
   `ADR-*`, or any other project convention)
2. **Execution definition** — a machine-readable runtime contract linked to one
   or more artifact IDs
3. **Execution run** — an immutable record of one invocation of one execution
   definition

### Projections and Convenience Surfaces

DDx may expose domain-specific projections over execution definitions and runs
when a project benefits from a narrower vocabulary, but those projections do
not own separate storage or a separate runtime substrate.

Examples:
- metrics project numeric observations, comparison results, and trends from
  execution runs linked to `MET-*` artifacts
- acceptance or test-case projections may present pass/fail evidence for
  artifacts such as `AC-*` or `TC-*`
- convenience commands such as `ddx metric` are optional wrappers over
  `ddx exec`; they must not introduce independent `.ddx/<domain>/` stores or
  bypass the generic execution record model

### Execution Definition

An execution definition describes:
- definition ID
- linked artifact ID or IDs
- executor kind (`command`, `agent`, future extension points)
- inputs such as command, prompt reference, working directory, environment, and
  timeout
- log capture policy
- structured result schema or parser contract
- optional evaluation rules such as pass/fail interpretation or scalar-field
  extraction

Execution definitions may be authored as git-tracked documents (graph-authored
definitions, discovered via FEAT-007) or managed as DDx runtime records
(runtime-managed definitions stored in the `exec-definitions` collection).
Graph-authored definitions participate in ordinary DDx document indexing and
are the preferred source when using `ddx agent execute-bead`. Runtime-managed
definitions in the `exec-definitions` collection remain valid for `ddx exec`
operations that do not require graph discovery. In either case, execution
runs are always immutable runtime records in the exec-runs substrate.

### Execution Run

An execution run records:
- run ID
- definition ID
- linked artifact ID or IDs
- executor kind
- started/finished timestamps
- terminal status
- raw logs
- structured result payload
- attachment references for large captured bodies when needed
- provenance such as actor, host, git revision, and DDx version

Runs are append-only evidence. A new run never rewrites a previous one.
The authoritative run metadata may be stored as a bead-backed record in a
dedicated execution collection, with named attachment files for large bodies,
as long as DDx preserves one coherent run identity and inspection model.

## Requirements

### Functional

1. **Execution definition storage** — DDx stores machine-readable execution definitions with stable IDs in repo-local storage
2. **Artifact linkage** — each definition links explicitly to one or more artifact IDs from the DDx document graph
3. **Executor kinds** — DDx supports built-in `command` and `agent` executor kinds, with room for future extension
4. **Definition validation** — `ddx exec validate` checks schema, linked artifact IDs, executor configuration, and result contract
5. **Execution invocation** — `ddx exec run <id>` executes one definition and creates one immutable run record
6. **Raw log capture** — each run captures stdout/stderr or equivalent raw execution logs
7. **Structured result capture** — each run stores machine-readable result data separate from raw logs
8. **Normalized terminal status** — each run records whether it succeeded, failed, timed out, or errored before producing a result
9. **Append-only history** — DDx retains ordered execution history without mutating prior runs
10. **History inspection** — users and tools can query runs by artifact ID, definition ID, status, or recency
11. **Run detail inspection** — users and tools can inspect one run's logs, structured result, and provenance
12. **Agent executor integration** — agent-backed definitions can delegate to `ddx agent` and retain the underlying session identity and logs
13. **Configuration** — storage roots, retention settings, and executor defaults are configurable in `.ddx/config.yaml`
14. **Projection support** — DDx supports domain-specific read models over shared execution definitions and runs without introducing separate runtime stores
15. **Metric specialization** — metrics may define numeric-result conventions, comparison behavior, and trend summaries as projections over execution runs linked to `MET-*` artifacts
16. **Compatibility and migration** — when DDx replaces a specialized runtime surface with `ddx exec`, the feature must define either a migration path or an explicit backward-compatible read/write policy
17. **Attachment-backed evidence** — DDx may store large run bodies such as logs or structured payloads in immutable attachment files referenced by run metadata rather than forcing all evidence inline
18. **Collection-backed storage** — DDx may store execution definitions and runs as bead-schema records in dedicated runtime collections rather than inventing a separate metadata schema for each execution family

### Non-Functional

- **Determinism:** DDx must persist the exact logs and structured result associated with one invocation
- **Durability:** definition and run writes must be atomic or serialized so concurrent writers cannot leave partial records behind
- **Observability:** stored runs must be human-readable enough for debugging and machine-parseable enough for automation
- **Portability:** definitions and runs remain repo-local and file-backed; no hosted service or database is required for v1
- **Low overhead:** execution bookkeeping adds minimal overhead beyond the underlying command or agent invocation
- **Payload resilience:** large prompt, response, and log bodies must not require rewriting a shared history file to persist one run safely

## CLI Commands

```bash
ddx exec list [--artifact ID]                          # list execution definitions
ddx exec show <definition-id>                          # show one definition
ddx exec validate <definition-id>                      # validate definition and links
ddx exec run <definition-id>                           # execute and persist a run
ddx exec log <run-id>                                  # show raw logs for one run
ddx exec result <run-id> [--json]                      # show structured result
ddx exec history [--artifact ID] [--definition ID]     # inspect historical runs
```

## Server Endpoints (FEAT-002 integration)

| MCP Tool | HTTP Endpoint | Description |
|----------|--------------|-------------|
| `ddx_exec_definitions` | `GET /api/exec/definitions` | List execution definitions |
| `ddx_exec_show` | `GET /api/exec/definitions/:id` | Show one execution definition |
| `ddx_exec_history` | `GET /api/exec/runs` | List historical execution runs |
| `ddx_exec_run` | `GET /api/exec/runs/:id` | Show one run's structured result and metadata |

The HTTP/MCP surface is read-only for v1. Execution invocation remains CLI-only.

## User Stories

### US-090: Developer Runs a Metric-Backed Execution
**As a** developer tracking a metric artifact
**I want** to execute its linked DDx definition and store the observed result
**So that** I can review the logs and structured numeric output later

**Acceptance Criteria:**
- Given an execution definition linked to `MET-001`, when I run `ddx exec run <definition-id>`, then DDx records a new immutable run with raw logs and structured result data
- Given the run completes, when I inspect its history, then I can find it by `MET-001` and by the definition ID

### US-091: Developer Reviews Acceptance Evidence Over Time
**As a** developer evaluating an acceptance artifact
**I want** to inspect historical pass/fail runs and their details
**So that** I can see whether the criterion has been stable or regressed

**Acceptance Criteria:**
- Given an execution definition linked to an acceptance artifact, when repeated runs occur, then DDx preserves ordered history rather than overwriting the last result
- Given I inspect one run, then I can see its status, logs, structured result, and provenance

### US-092: Workflow Tool Queries Reusable Execution Evidence
**As** HELIX, dun, or another tool
**I want** to query execution history by artifact ID
**So that** I can build higher-level decisions without inventing my own log format

**Acceptance Criteria:**
- Given execution history exists for an artifact, when a tool queries DDx, then it receives structured run metadata and result payloads
- Given the execution was agent-backed, then the returned run data retains a link to the underlying agent session

### US-093: Developer Uses a Metric Convenience Surface Without Forking Storage
**As a** developer working with metrics frequently
**I want** any optional metric-focused command surface to resolve through `ddx exec`
**So that** metrics stay aligned with the generic execution model

**Acceptance Criteria:**
- Given a metric-linked execution definition exists, when I use an optional metric convenience command, then it resolves through the same execution definitions and run history used by `ddx exec`
- Given metric history exists, then there is no separate authoritative `.ddx/metrics/` runtime store that can drift from `.ddx/exec/`

### US-094: Operator Migrates From a Specialized Runtime Store
**As a** repo operator adopting the execution model
**I want** a defined migration or compatibility policy for older specialized runtime data
**So that** adopting `ddx exec` does not strand existing metric evidence

**Acceptance Criteria:**
- Given a repository contains prior metric runtime data, when DDx adopts `ddx exec`, then new execution and metric writes land in bead-backed collections and legacy `.ddx/exec/` or `.ddx/metrics/` data remains readable as a fallback during migration
- Given the chosen policy, then the behavior is documented and testable rather than implicit

### US-095: Execute-bead Compatibility and Definition Source Priority
**As** a workflow tool invoking execute-bead
**I want** DDx to resolve graph-authored execution definitions as authoritative
**So that** runtime behavior matches the documents in git

**Acceptance Criteria:**
- Given a graph-authored execution definition exists for an artifact, when `ddx exec validate` or `ddx exec run` resolves that artifact's definition, then the graph-authored document takes precedence over any runtime-managed definition in the `exec-definitions` collection.
- Given only a runtime-managed definition exists for an artifact, when `ddx exec run` is invoked, then it proceeds using the runtime-managed definition without error.
- Given an execution definition is marked `required: true`, when its execution run terminates with a non-success status, then the result is classified as merge-blocking and any execute-bead workflow consuming it preserves rather than lands.
- Given a metric-producing execution run completes, when `ddx metric` queries for that metric, then the result is served from the `exec-runs` collection — no `.ddx/metrics/` directory is created or required.
- Given an agent-backed execution run completes, when the run record is inspected, then it retains a stable link to the underlying agent session ID resolvable via `ddx agent log`.
- Given `ddx agent execute-bead` triggers execution documents, when the resulting runs are queried, then they appear in `ddx exec history` and `ddx metric` output through the standard inspection surfaces.

## Dependencies

- FEAT-005 (Artifacts) — execution definitions link runtime behavior to artifact IDs
- FEAT-006 (Agent Service) — provides the `agent` executor kind and canonical agent session logs
- DDx CLI infrastructure (config loading, command factory)

## Out of Scope

- Workflow-specific action semantics such as HELIX phase routing or issue closing rules
- **Autonomy semantics and escalation policy** — DDx does not define what autonomy levels mean or when to escalate; those are HELIX methodology-layer decisions
- **When to invoke execution and what to do with results** — DDx provides the execution substrate; HELIX decides when to run executions and how to act on outcomes
- Automatic generation of execution definitions from artifact prose
- Server-side execution invocation over HTTP/MCP
- Centralized hosted execution history storage
- Visual dashboards or methodology-specific scoring rules
- Separate domain-specific runtime stores that duplicate the authority of `ddx exec`

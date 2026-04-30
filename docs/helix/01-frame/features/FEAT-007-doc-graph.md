---
ddx:
  id: FEAT-007
  depends_on:
    - helix.prd
---
# Feature: Artifact Dependency Graph

**ID:** FEAT-007
**Status:** In Progress
**Priority:** P0
**Owner:** DDx Team

## Overview

DDx tracks dependencies between artifacts using identity metadata. Markdown
artifacts declare identity in YAML frontmatter under `ddx:`. Non-markdown
artifacts (diagrams, images, prompts, PDFs, binaries, and other media) declare
the same identity in a sidecar `<filename>.ddx.yaml` file. When an upstream
artifact changes, DDx detects which downstream artifacts are stale and need
review. This is the "keep artifacts honest" capability — the infrastructure
that prevents document and generated-media drift.

Workflow tools and check runners consume the graph to enforce document quality. DDx owns the graph model, staleness detection, hashing, and stamping. Check runners delegate graph operations to DDx rather than implementing their own.

## Problem Statement

**Current situation:** Dun implements document dependency tracking internally (`doc_dag.go`, `frontmatter.go`, `hash.go`, `stamp.go`). This is document infrastructure, not check logic — it belongs in DDx alongside the document library it operates on.

**Pain points:**
- Document staleness detection is locked inside the check runner, inaccessible to other tools
- No CLI command to inspect the document graph, check staleness, or stamp documents
- No MCP endpoint for agents to query document relationships
- The `dun:` frontmatter prefix ties a generic concept to a specific tool

**Desired outcome:** DDx owns the artifact graph as a first-class service. Any tool can query it. The `ddx:` frontmatter convention and `.ddx.yaml` sidecars are the standard ways to declare artifact identity and dependencies.

## The `ddx:` Frontmatter Convention

Documents declare their identity and dependencies using YAML frontmatter with the `ddx:` namespace. GitHub hides YAML frontmatter in its markdown viewer, so this is invisible to casual readers.

### Format

```yaml
---
ddx:
  id: helix.architecture
  depends_on:
    - helix.prd
    - helix.feature-registry
  inputs:
    - node:helix.prd
    - refs:helix.prd
  review:
    self_hash: "a3f2dd..."
    deps:
      helix.prd: "7c6f43..."
    reviewed_at: "2026-04-03T12:00:00Z"
---
# Architecture Document

Content here...
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique document identifier (e.g., `helix.prd`, `SD-001`) |
| `media_type` | string | no | IANA-style media type. Markdown frontmatter defaults to `text/markdown`; sidecars should set this explicitly. |
| `depends_on` | []string | no | IDs of documents this one depends on |
| `inputs` | []string | no | Input selectors for prompt context resolution |
| `generated_by` | object | no | Provenance for generated artifacts: `{ run, prompt, source_hash }`. This creates a provenance edge, not an authority edge. |
| `review` | object | no | Staleness tracking metadata (managed by `ddx doc stamp`) |
| `review.self_hash` | string | auto | Content hash of this document (excluding review block) |
| `review.deps` | map[string]string | auto | Map of dependency ID → hash at last review |
| `review.reviewed_at` | string | auto | ISO-8601 timestamp of last stamp |
| `prompt` | string | no | Custom prompt for updating this document |
| `parking_lot` | bool | no | Skip this document in staleness checks |

### Compatibility

- The `ddx:` prefix replaces the existing `dun:` prefix. DDx should read both `ddx:` and `dun:` frontmatter for backward compatibility, but write `ddx:`.
- Documents without `ddx:` frontmatter are ignored by the graph.
- Unknown fields within the `ddx:` block are preserved on round-trip.

## Sidecar Identity for Non-Markdown Artifacts

Non-markdown artifacts use a sidecar whose filename is the full paired
artifact filename plus `.ddx.yaml`:

```text
diagram.excalidraw       -> diagram.excalidraw.ddx.yaml
hero.png                 -> hero.png.ddx.yaml
system-prompt.txt        -> system-prompt.txt.ddx.yaml
```

The sidecar is plain YAML, not markdown frontmatter:

```yaml
id: DIAG-007
media_type: application/vnd.excalidraw+json
depends_on:
  - SD-003
generated_by:
  run: run-20260429T120000Z
  prompt: docs/prompts/diagram.prompt.md
  source_hash: sha256:abc123
review:
  self_hash: "a3f2dd..."
  deps:
    SD-003: "7c6f43..."
  reviewed_at: "2026-04-29T12:00:00Z"
```

Sidecar rules:

- The sidecar and paired artifact must be in the same directory.
- A sidecar with no paired artifact is a dangling-sidecar error.
- Markdown files must use embedded `ddx:` frontmatter, not sidecars.
- `media_type` is required for sidecar identity and may be inferred only as a
  fallback for display.
- The scanner reads sidecar YAML and hashes the paired artifact bytes for
  `review.self_hash`.

## Generated-By Provenance

`generated_by` records how an artifact was produced. It is a provenance edge,
not an authority edge:

- It does not participate in dependency staleness cascade the way
  `depends_on` does.
- A `generated_by.source_hash` mismatch marks the generated artifact itself as
  regeneration-eligible.
- It is still indexed so graph consumers can answer "which run produced this?"
  and "which artifacts came from this run?"

## Requirements

### Functional

1. **Identity parsing** — read `ddx:` (and legacy `dun:`) YAML frontmatter from markdown files and sidecar `*.ddx.yaml` identity from non-markdown artifacts
2. **Graph construction** — scan a directory tree, parse identity, build a directed acyclic graph of artifact dependencies
3. **Content hashing** — deterministic hash of document content excluding the `review` block. Same content = same hash always.
4. **Staleness detection** — a document is stale when any dependency's current hash differs from the hash recorded in `review.deps`
5. **Cascade propagation** — if a document is stale, all its dependents are transitively stale
6. **Stamp command** (`ddx doc stamp`) — update `review.self_hash` and `review.deps` to mark a document as reviewed against its current dependencies
7. **Graph query** (`ddx doc graph`) — show the document dependency graph as text or JSON
8. **Stale query** (`ddx doc stale`) — list stale documents
9. **Input resolution** — resolve `inputs` selectors (`node:`, `refs:`, `code_refs:`, `paths:`) to actual content for prompt assembly
10. **Graph configuration** — optional `.ddx/graphs/*.yaml` files defining required roots, ID-to-path mappings, and cascade rules
11. **Body-link indexing** — scan markdown document bodies for `[[ID]]` reference syntax and index those as graph edges alongside frontmatter-declared `depends_on` edges. Support plain IDs (`[[FEAT-001]]`), slugged IDs (`[[US-036-list-mcp-servers]]`), and dotted IDs (`[[helix.workflow.artifact-hierarchy]]`). Return the union of body links and frontmatter edges without duplicate edges. Body-link indexing is markdown-only; binary/media artifacts are indexed only through sidecars.
12. **Media metadata** — expose `media_type` and `generated_by` metadata on graph nodes, HTTP responses, and MCP responses.

### Non-Functional

- **Performance:** Graph construction <500ms for repos with 100+ documents
- **Determinism:** Same document content always produces the same hash
- **Portability:** Works on any repo with markdown files containing `ddx:` frontmatter
- **Backward compatibility:** Reads `dun:` frontmatter, writes `ddx:`

## CLI Commands

```bash
ddx doc graph [--json]              # Show document dependency graph
ddx doc stale [--json]              # List stale documents
ddx doc stamp [paths...] [--all]    # Update review stamps
ddx doc show <id>                   # Show document metadata and status
ddx doc deps <id>                   # Show what a document depends on
ddx doc dependents <id>             # Show what depends on a document
```

## Server Endpoints (FEAT-002 integration)

| MCP Tool | HTTP Endpoint | Description |
|----------|--------------|-------------|
| `ddx_doc_graph` | `GET /api/docs/graph` | Full dependency graph |
| `ddx_doc_stale` | `GET /api/docs/stale` | List stale documents |
| `ddx_doc_show` | `GET /api/docs/:id` | Document metadata and status |
| `ddx_doc_deps` | `GET /api/docs/:id/deps` | Dependencies of a document |
| `ddx_doc_sidecar` | `GET /api/docs/:id/sidecar` | Sidecar identity payload for non-markdown artifacts |
| `ddx_doc_generated_by` | `GET /api/docs/:id/generated-by` | Generated-artifact provenance |

## User Stories

### US-070: Developer Checks Document Freshness
**As a** developer maintaining project docs
**I want** to see which documents are stale after upstream changes
**So that** I know what needs reviewing

**Acceptance Criteria:**
- Given the PRD has changed since the architecture doc was last stamped, when I run `ddx doc stale`, then the architecture doc is listed as stale
- Given I update the architecture doc and run `ddx doc stamp docs/architecture.md`, then it's no longer reported as stale

### US-071: Agent Discovers Document Relationships
**As an** AI agent working on a feature
**I want** to query which documents govern this area of the codebase
**So that** I can load the right context

**Acceptance Criteria:**
- Given ddx-server is running, when an agent calls `ddx_doc_deps` with a design doc ID, then it receives the list of upstream governing documents
- Given an agent calls `ddx_doc_stale`, then it knows which documents need attention before proceeding

### US-073: Graph Indexes Body Links and Supports Reverse Traversal
**As** DDx (and tools consuming the graph)
**I want** document body `[[ID]]` references indexed as graph edges
**So that** dependency relationships declared in prose are machine-queryable without grep

**Acceptance Criteria:**
- Given a markdown document containing `[[FEAT-011]]`, `[[US-036-list-mcp-servers]]`, and `[[helix.workflow.artifact-hierarchy]]` in its body, when DDx constructs the graph, then those references are indexed as resolvable graph edges.
- Given both `ddx.depends_on: [A]` frontmatter and a `[[A]]` body reference in the same document, when the graph is queried, then a single edge to A is returned — not two.
- Given documents B and C each contain `[[A]]` in their bodies, when dependents of A are queried, then B and C are returned from the graph index without requiring an external file scan.
- Given an execution document declares `ddx.depends_on: [FEAT-001]`, when the graph is queried, then the execution document is returned as a discoverable graph artifact linked to FEAT-001.
- Given malformed `[[...]]` syntax (e.g., `[[]]`, `[[has spaces]]`) or an `[[UnknownID]]` reference, when the graph is constructed, then the reference is skipped silently and graph construction completes without error.

### US-072: Check Runner Delegates Graph Operations
**As** dun (check runner)
**I want** to call `ddx doc stale` instead of implementing my own graph logic
**So that** document staleness is a shared service

**Acceptance Criteria:**
- Given dun's doc-dag check is configured, when it runs, then it calls `ddx doc stale --json` and reports the results
- Given dun's change-cascade check detects upstream changes, then it calls `ddx doc stale` to identify affected downstream documents

## Implementation Notes

### Porting from Dun

The following dun source files contain the logic to port:

| Dun File | What It Does | DDx Destination |
|----------|-------------|----------------|
| `internal/dun/frontmatter.go` | Parse/write `dun:` frontmatter | `internal/docgraph/frontmatter.go` |
| `internal/dun/doc_dag.go` | Build graph, detect staleness, cascade | `internal/docgraph/graph.go` |
| `internal/dun/hash.go` | Deterministic content hashing | `internal/docgraph/hash.go` |
| `internal/dun/stamp.go` | Update review stamps in frontmatter | `internal/docgraph/stamp.go` |

### Migration Strategy

**Phase 1:** DDx ships doc graph commands (`ddx doc graph/stale/stamp`). Reads both `ddx:` and `dun:` frontmatter, writes `ddx:`.

**Phase 2:** Dun adds `ddx doc stale --json` as alternative to internal graph logic, gated by `DUN_USE_DDX_DOC=1`.

**Phase 3:** Once proven, dun removes internal doc_dag/frontmatter/hash/stamp code and delegates fully to DDx.

**Phase 4:** A one-time migration tool converts `dun:` frontmatter to `ddx:` in existing repos.

## Edge Cases

- Document with `ddx:` frontmatter but no `id` — skip, warn
- Circular dependencies — detect and report, don't infinite loop
- Missing dependency (ID referenced but document not found) — report as warning, don't fail
- Mixed `dun:` and `ddx:` in same repo — read both, prefer `ddx:` if both present on same file
- Binary files or non-markdown without sidecars — skip silently
- Sidecar file without paired artifact — report dangling-sidecar error
- Sidecar beside markdown file — report validation error; markdown uses frontmatter
- `generated_by` target missing — warn and keep the artifact indexed
- Very large repos (1000+ markdown files) — incremental scanning with caching

## Dependencies

- DDx CLI infrastructure (config, command factory)
- Markdown files with `ddx:` frontmatter
- For server endpoints: FEAT-002

## Execution Documents

Execution documents are a category of graph-discovered artifact that declare
what validations, checks, and measurements should run for a bead or set of
governing artifacts. They live in the worktree as ordinary markdown files with
`ddx:` frontmatter and participate in DDx document indexing alongside other
graph artifacts.

### Frontmatter convention

```yaml
---
ddx:
  id: exec.FEAT-001.acceptance-smoke
  depends_on:
    - FEAT-001
  execution:
    kind: command             # command | agent
    required: true            # true = merge-blocking in execute-bead
    command: ["make", "test"]
    cwd: cli
    timeout_ms: 120000
---
# Acceptance Smoke Test for FEAT-001
...
```

### Key fields

| Field | Description |
|-------|-------------|
| `ddx.id` | Unique document ID; by convention prefixed `exec.` |
| `ddx.depends_on` | Governing artifacts this execution is linked to |
| `ddx.execution.kind` | Executor kind: `command` or `agent` |
| `ddx.execution.required` | `true` means this execution is merge-blocking in `execute-bead` |

### Discovery

`ddx try` resolves applicable execution documents from the graph inside the
attempt worktree by following dependency links from the target bead and its
governing artifacts. DDx indexes execution documents like other graph artifacts;
no separate registry is required.

### Relationship to FEAT-010

Execution documents are the git-backed, authored source of truth for what
`ddx exec` runs. FEAT-010's exec substrate stores immutable run history.
FEAT-007's graph owns the discovery and indexing of execution document
definitions.

## Git-Aware History (FEAT-012 Integration)

The following commands extend the doc graph with revision-control-aware
queries. They are defined in FEAT-012 but operate on the document graph:

```bash
ddx doc history <id> [--since <ref>]   # Commit log for an artifact
ddx doc diff <id> [<ref1>] [<ref2>]    # Content diff between refs
ddx doc changed [--since <ref>]        # Artifacts changed since a ref
```

These require the graph's artifact-ID-to-file-path mapping and are
surfaced alongside the existing graph/stale/stamp commands.

## Out of Scope

- Code-to-document tracing (which code implements which spec) — that's dun's `spec-binding` check
- Automated document updating — DDx detects staleness, agents do the updating
- Document content validation (correct sections, completeness) — that's workflow-level
- Branch management, remote operations, merge conflict resolution (see FEAT-012 Out of Scope)
- **Graph traversal policy** — authority ordering, impact-flow strategy, and search fallback policy are delegated to workflow tools; DDx provides the graph primitives (edges, dependents, staleness), not the traversal strategy
- **Artifact decomposition policy** — how to break a change across an artifact stack is a plugin concern; DDx indexes the artifacts and their relationships

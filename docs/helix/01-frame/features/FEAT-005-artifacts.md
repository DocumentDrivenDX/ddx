---
ddx:
  id: FEAT-005
  depends_on:
    - helix.prd
    - FEAT-007
---
# Feature: Artifact Convention

**ID:** FEAT-005
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

An **artifact** is any markdown document with `ddx:` YAML frontmatter. The `ddx.id` field identifies it; the ID prefix conventionally indicates the type (ADR-001, SD-003, MET-007, FEAT-012, etc.). DDx does not hardcode artifact types — it manages the document graph generically. Types are conventions that workflows define.

This replaces the previous approach of dedicated `ddx adr` and `ddx sd` commands. Those are removed in favor of the generic `ddx doc` commands (FEAT-007).

## Definition

An artifact is a markdown file containing:

```yaml
---
ddx:
  id: <PREFIX>-<NNN>
  depends_on: [<other-ids>]
---
# Content
```

That's it. Any file with a `ddx:` block and an `id` field is an artifact. DDx discovers them by scanning for frontmatter, not by looking in specific directories.

### Common Artifact Types (by convention)

| Prefix | Type | Typical Location |
|--------|------|-----------------|
| `FEAT` | Feature specification | `docs/helix/01-frame/features/` |
| `ADR` | Architecture Decision Record | `docs/adr/` or `docs/helix/02-design/adr/` |
| `SD` | Solution Design | `docs/designs/` or `docs/helix/02-design/solution-designs/` |
| `TD` | Technical Design | `docs/helix/02-design/technical-designs/` |
| `TP` | Test Plan | `docs/helix/03-test/` |
| `MET` | Metric definition | `docs/metrics/` |
| `US` | User Story | `docs/helix/01-frame/user-stories/` |

These prefixes are conventions — DDx treats them all the same. Workflows
(HELIX) may enforce that certain types exist or follow specific templates.

Projects may introduce additional conventions such as `AC-*` for acceptance
criteria or `TC-*` for test cases. DDx does not privilege those types; it only
tracks their IDs and relationships.

### Frontmatter Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique identifier with type prefix (e.g., `ADR-001`) |
| `depends_on` | []string | no | IDs of artifacts this one depends on |
| `inputs` | []string | no | Input selectors for prompt context resolution |
| `review` | object | no | Staleness tracking (managed by `ddx doc stamp`) |
| `prompt` | string | no | Custom prompt for updating this document |
| `parking_lot` | bool | no | Skip in staleness checks |

Unknown fields are preserved on round-trip.

## The Artifact Graph

All artifacts in a project form a **single directed acyclic graph** via `depends_on` edges. This graph is the project's authority structure — it encodes which documents govern which.

```
Product Vision
  └─ PRD
       ├─ FEAT-001
       │    ├─ SD-001
       │    │    └─ TD-001
       │    │         └─ TP-001
       │    └─ US-001
       ├─ FEAT-002
       │    └─ ADR-001
       └─ MET-001
```

Every artifact should be reachable from a root (typically the vision or PRD). Orphaned artifacts — those with no incoming or outgoing edges — are a smell that `ddx doc validate` flags.

### Graph Properties

- **Single graph per project.** All artifacts with `ddx:` frontmatter participate in one graph. There are no separate "ADR graph" or "feature graph" partitions.
- **Edges are explicit.** Only `depends_on` creates edges. No implicit edges from directory structure or naming.
- **Direction is authority.** An edge from B to A means "A governs B" — if A changes, B may be stale.
- **Staleness cascades.** If a node is stale, all its descendants are transitively stale.
- **Types are orthogonal.** The graph doesn't care about artifact types. An ADR can depend on a feature spec; a metric can depend on a PRD; a test plan can depend on a solution design. The graph is type-agnostic.

## What DDx Does With Artifacts

All via `ddx doc` commands (FEAT-007):

1. **Discover** — scan for `ddx:` frontmatter in markdown files
2. **Graph** — build the single project-wide dependency DAG
3. **Stale** — detect when upstream dependencies changed since last stamp
4. **Stamp** — record review hashes after updating a document
5. **List** — list artifacts with optional type filtering (`ddx doc list --type ADR`)
6. **Show** — display artifact content and metadata
7. **Validate** — check frontmatter structure, dependency references exist, no circular deps, no orphans

## What DDx Does NOT Do

- **Scaffold from templates** — agents and workflows create documents. DDx doesn't need `create` commands for each type.
- **Enforce type-specific structure** — "ADRs must have a Decision section" is a workflow concern. DDx validates the graph; dun validates content structure.
- **Hardcode artifact types** — no switch statements on type prefixes. The graph treats all artifacts equally.
- **Partition the graph** — all artifacts are in one graph regardless of type or directory.
- **Execute artifact documents directly** — runtime execution belongs to DDx executions (FEAT-010), not the artifact graph itself.

## Templates and Prompts

Each artifact type has associated resources in the document library:

```
.ddx/library/artifacts/<type>/
├── template.md       # Structural template (frontmatter + sections)
├── create.md         # Prompt for creating a new instance
├── evolve.md         # Prompt for updating when dependencies change
└── check.md          # Prompt for reviewing/validating the artifact
```

For example, `.ddx/library/artifacts/adr/`:
- `template.md` — ADR skeleton with Context/Decision/Alternatives sections
- `create.md` — prompt instructing an agent how to write a good ADR
- `evolve.md` — prompt for updating an ADR when upstream requirements change
- `check.md` — prompt for reviewing an ADR against its governing artifacts

These are **library resources**, not CLI features. Agents and workflows read them from the library when operating on artifacts. DDx ships defaults; projects can override with project-specific versions.

The doc graph (FEAT-007) can resolve these prompts via input selectors:
- `artifact-prompt:ADR:create` → the create prompt for ADR artifacts
- `artifact-template:ADR` → the template for ADR artifacts

## Artifacts and Executions

Artifacts stay declarative. They define meaning, authority, and relationships
in the document graph. DDx does **not** execute artifact files directly.

Runtime evaluation belongs to DDx execution definitions and execution runs
(FEAT-010), which are file-backed runtime records linked to artifacts by ID.

### Boundary

- Artifacts declare what matters, who governs it, and how it relates to other
  artifacts in the graph.
- Execution definitions describe how to evaluate, verify, or measure one or
  more artifacts.
- Execution runs capture one immutable invocation: raw logs, structured result
  data, status, and provenance.
- Execution definitions and runs are not artifacts, and they do not
  participate in document staleness or graph validation.
- DDx does not infer execution semantics from an artifact type prefix.

### Examples

- A `MET-*` artifact may link to an execution definition that emits numeric
  observations over time.
- Metric comparison and trend views are projections over those execution runs,
  not evidence stored in a separate metric-owned runtime backend.
- A project may model acceptance criteria or test cases as standalone artifacts
  and link them to execution definitions that emit pass/fail or richer
  structured results.
- An execution definition may evaluate multiple governing artifacts together as
  long as those links are explicit in the runtime record.

The shared rule is simple: artifact IDs establish meaning and authority; DDx
executions establish repeatable runtime behavior and evidence.

## Migration

The dedicated `ddx adr` and `ddx sd` commands are removed. Their functionality is subsumed by:

| Old Command | Replacement |
|-------------|-------------|
| `ddx adr create` | Agent creates file from template |
| `ddx adr list` | `ddx doc list --type ADR` |
| `ddx adr show ADR-001` | `ddx doc show ADR-001` |
| `ddx adr validate` | `ddx doc validate` |
| `ddx sd create` | Agent creates file from template |
| `ddx sd list` | `ddx doc list --type SD` |
| `ddx sd show SD-001` | `ddx doc show SD-001` |
| `ddx sd validate` | `ddx doc validate` |

## Dependencies

- FEAT-007 (Doc Graph) — provides the `ddx doc` commands
- Document library with templates

## Out of Scope

- Type-specific CLI commands (use `ddx doc` generically)
- Content structure validation (that's dun/workflow-level)
- Template application commands (agents do this)

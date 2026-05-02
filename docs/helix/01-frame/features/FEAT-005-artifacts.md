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

An **artifact** is any file carrying DDx **identity** — a structured `id` plus
optional graph and provenance metadata. Identity may be embedded (in a
markdown file's YAML frontmatter under a `ddx:` block) or supplied via a
sidecar `<filename>.ddx.yaml` for non-markdown media (diagrams, wireframes,
images, prompts, audio, PDFs, binaries, etc.).

The `id` field identifies the artifact; the ID prefix conventionally
indicates the type (ADR-001, SD-003, MET-007, FEAT-012, etc.). DDx does
not hardcode artifact types — it manages the document graph generically.
Types are conventions that workflows define.

This replaces the previous approach of dedicated `ddx adr` and `ddx sd` commands. Those are removed in favor of the generic `ddx doc` commands (FEAT-007).

## Definition

### Authority rule

**Identity present → artifact.** A file is an artifact if and only if DDx can
resolve identity for it, by either of two mechanisms:

1. **Embedded identity** — markdown files with a `ddx:` block in YAML
   frontmatter:

   ```yaml
   ---
   ddx:
     id: <PREFIX>-<NNN>
     depends_on: [<other-ids>]
   ---
   # Content
   ```

2. **Sidecar identity** — any file `<path>/<name>.<ext>` accompanied by
   `<path>/<name>.<ext>.ddx.yaml` containing the identity block:

   ```yaml
   # diagram.excalidraw.ddx.yaml
   id: DIAG-007
   media_type: application/vnd.excalidraw+json
   depends_on: [SD-003]
   generated_by:
     run: <run-id>
     prompt: <input-selector>
     source_hash: <sha256>
   ```

There is no second-class "managed asset" concept. Either a file has identity
(and is an artifact), or it does not (and DDx ignores it for graph purposes).

DDx discovers artifacts by scanning for embedded frontmatter and sidecar
`*.ddx.yaml` files, not by looking in specific directories.

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
| `DIAG` | Diagram (mermaid, excalidraw, svg) | `docs/diagrams/` |
| `WIRE` | Wireframe / mockup | `docs/wireframes/` |
| `IMG` | Image asset (hero, illustration) | `docs/images/` |
| `PRMT` | Prompt artifact | `docs/prompts/` |
| `RSCH` | Research note (IDs `RSCH-NNN`) | `docs/helix/00-discover/research/` |
| `REF` | Reference, external source capture (IDs `REF-NNN`) | `docs/helix/00-discover/references/` |

These prefixes are conventions — DDx treats them all the same. Workflows
(HELIX) may enforce that certain types exist or follow specific templates.

Projects may introduce additional conventions such as `AC-*` for acceptance
criteria or `TC-*` for test cases. DDx does not privilege those types; it only
tracks their IDs and relationships.

### Identity Schema

Identity carries the same field set whether embedded under a markdown
`ddx:` block or stored in a sidecar `.ddx.yaml`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique identifier with type prefix (e.g., `ADR-001`) |
| `media_type` | string | conditional | IANA-style media type. Required for sidecar identity; inferred from extension for embedded markdown (`text/markdown`). |
| `depends_on` | []string | no | IDs of artifacts this one depends on |
| `inputs` | []string | no | Input selectors for prompt context resolution |
| `generated_by` | object | no | Provenance for generated artifacts: `{ run, prompt, source_hash }`. Records the layer-1/layer-2 run that produced this artifact and the hash of its source inputs. |
| `review` | object | no | Staleness tracking (managed by `ddx doc stamp`) |
| `prompt` | string | no | Custom prompt for updating this document |
| `parking_lot` | bool | no | Skip in staleness checks |

Unknown fields are preserved on round-trip.

#### Type-specific metadata (outside `ddx:`)

Type conventions may carry additional metadata fields at the top level of
frontmatter, alongside (not inside) the `ddx:` identity block. The parser
treats these as inert pass-through fields. For example, a `REF-NNN`
reference artifact captures provenance about an external source:

```yaml
---
ddx:
  id: REF-007
  depends_on: [RSCH-003]
source_url: https://example.com/article
source_author: Jane Doe
accessed: 2026-05-01
---
# Reference: ...
```

Identity (`id`, `depends_on`, etc.) stays under `ddx:`; type-specific fields
like `source_url`, `source_author`, and `accessed` live at the top level.

#### `media_type`

Identifies how an artifact's bytes should be interpreted. Examples:

- `text/markdown` (default for embedded identity)
- `image/png`, `image/svg+xml`
- `application/vnd.excalidraw+json`
- `application/pdf`
- `text/mermaid`
- `application/vnd.ddx.prompt+yaml`

Web UI rendering, regenerate flows, and tooling key off `media_type` rather
than file extension.

#### `generated_by`

Marks an artifact as the output of a DDx run. Carries the run id, the prompt
selector, and the `source_hash` used at generation time. Regeneration is
source-hash-driven: when inputs change, the source hash diverges and tooling
can offer to regenerate. `generated_by` creates a separate provenance edge in
the graph (FEAT-007) — it tracks origin, not authority, and does **not**
cascade staleness like `depends_on`.

## The Artifact Graph

All artifacts in a project — markdown and non-markdown alike — form a
**single directed acyclic graph** via `depends_on` edges. This graph is the
project's authority structure: it encodes which artifacts govern which.

```
Product Vision
  └─ PRD
       ├─ FEAT-001
       │    ├─ SD-001
       │    │    ├─ TD-001
       │    │    │    └─ TP-001
       │    │    └─ DIAG-001  (excalidraw, sidecar identity)
       │    └─ US-001
       ├─ FEAT-002
       │    └─ ADR-001
       └─ MET-001
```

Every artifact should be reachable from a root (typically the vision or PRD). Orphaned artifacts — those with no incoming or outgoing edges — are a smell that `ddx doc validate` flags.

### Graph Properties

- **Single graph per project.** All artifacts with identity participate in one graph regardless of media type. There are no separate "ADR graph," "diagram graph," or "feature graph" partitions.
- **Edges are explicit.** Only `depends_on` creates authority edges. `generated_by` creates a parallel provenance edge that is not authority. No implicit edges from directory structure or naming.
- **Direction is authority.** A `depends_on` edge from B to A means "A governs B" — if A changes, B may be stale.
- **Authority staleness cascades.** If a node is stale, all its `depends_on` descendants are transitively stale.
- **Provenance staleness does not cascade.** A `generated_by` source-hash mismatch flags only the generated artifact itself, offering regeneration; it does not propagate.
- **Types and media are orthogonal.** The graph doesn't care about artifact types or media. An ADR can depend on a feature spec; a metric can depend on a PRD; a wireframe can depend on a solution design. The graph is type- and media-agnostic.

## What DDx Does With Artifacts

All via `ddx doc` commands (FEAT-007):

1. **Discover** — scan for embedded `ddx:` frontmatter in markdown and for sidecar `*.ddx.yaml` files
2. **Graph** — build the single project-wide dependency DAG plus provenance edges
3. **Stale** — detect when upstream `depends_on` artifacts changed since last stamp; detect generated artifacts whose `source_hash` no longer matches
4. **Stamp** — record review hashes after updating a document
5. **List** — list artifacts with optional type or media filtering (`ddx doc list --type ADR`, `ddx artifact list --media-type image/png`)
6. **Show** — display artifact content/metadata; for non-markdown media, show the sidecar plus a media-typed pointer
7. **Validate** — check identity structure, dependency references exist, no circular `depends_on`, no orphans, sidecars match an existing target file

## What DDx Does NOT Do

- **Scaffold from templates** — agents and workflows create artifacts. DDx doesn't need `create` commands for each type.
- **Enforce type-specific structure** — "ADRs must have a Decision section" is a workflow concern. DDx validates the graph; dun validates content structure.
- **Hardcode artifact types or media** — no switch statements on type prefixes or media types. The graph treats all artifacts equally.
- **Partition the graph** — all artifacts are in one graph regardless of type, media, or directory.
- **Execute artifact files directly** — runtime execution belongs to DDx runs (FEAT-010), not the artifact graph itself.
- **Recognize a "managed asset" tier** — files without identity are not artifacts, period. There is no halfway state.

## Templates and Prompts

Each artifact type has associated resources in the document library:

```
.ddx/library/artifacts/<type>/
├── template.md       # Structural template (frontmatter + sections), or sidecar template for binary types
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

## Artifacts and Runs

Artifacts stay declarative. They define meaning, authority, provenance, and
relationships in the graph. DDx does **not** execute artifact files directly.

Runtime invocation belongs to the three-layer run architecture (FEAT-010):
`ddx run` (layer-1 single agent invocation), `ddx try <bead>` (layer-2 bead
attempt), `ddx work` (layer-3 queue drain). Run records are file-backed and
linked to artifacts by ID via `generated_by`.

### Boundary

- Artifacts declare what matters, who governs it, where it came from, and how
  it relates to other artifacts in the graph.
- Runs describe one invocation that may produce or consume artifacts.
- Run records capture one immutable invocation: raw logs, structured result
  data, status, tokens/duration/exit, and provenance.
- Runs and run records are not artifacts and do not participate in document
  staleness or graph validation.
- DDx does not infer run semantics from an artifact type prefix.

### Examples

- A `MET-*` artifact may link to a layer-1 run definition that emits numeric
  observations over time.
- A `DIAG-*` excalidraw artifact may carry `generated_by` pointing at the
  layer-1 run that produced it; regenerating updates the file and refreshes
  the source hash.
- Metric comparison and trend views are projections over runs, not evidence
  stored in a separate metric-owned runtime backend.
- A project may model acceptance criteria or test cases as standalone
  artifacts and link them to runs that emit pass/fail or richer structured
  results.
- A run may evaluate multiple governing artifacts together as long as those
  links are explicit in the run record.

The shared rule is simple: artifact identity establishes meaning, authority,
and provenance; DDx runs establish repeatable runtime behavior and evidence.

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

Existing markdown artifacts are unaffected by the multi-media broadening:
their embedded `ddx:` identity continues to work and `media_type` defaults
to `text/markdown` when absent. Adoption of sidecar identity is opt-in,
file by file.

## Dependencies

- FEAT-007 (Doc Graph) — provides the `ddx doc` commands, sidecar scanner,
  `media_type` handling, and `generated_by` provenance edges
- FEAT-010 (Runs) — provides the run records that `generated_by` references
- Document library with templates

## Out of Scope

- Type-specific CLI commands (use `ddx doc` generically)
- Content structure validation (that's dun/workflow-level)
- Template application commands (agents do this)
- A "managed asset" or any other second-class artifact tier

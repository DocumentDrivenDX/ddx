---
ddx:
  id: TD-026
  depends_on:
    - FEAT-007
    - FEAT-010
    - TD-025
---
# Technical Design: Artifact Graph Schema

## Purpose

This design defines the graph record shape used after artifact identity expands
beyond markdown documents. It complements TD-025, which defines how artifact
identity is discovered from embedded frontmatter and sidecar `.ddx.yaml` files.

The graph must expose `media_type`, represent `generated_by` as a provenance
edge, and keep generated-artifact staleness separate from ordinary dependency
staleness.

## Graph Nodes

Each discovered artifact produces one graph node:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Stable artifact id from `ddx.id`. |
| `path` | string | Repository-relative path to the artifact bytes or markdown file. |
| `identity_source` | enum | `frontmatter` for markdown, `sidecar` for non-markdown media. |
| `sidecar_path` | string? | Repository-relative sidecar path when `identity_source=sidecar`. |
| `media_type` | string | IANA-style media type. Markdown defaults to `text/markdown`; sidecars should set it explicitly and may be inferred only as a compatibility fallback. |
| `hash` | string | Current content hash. Markdown excludes the managed `review` block; sidecar artifacts hash paired artifact bytes. |
| `review` | object? | Last reviewed hash/dependency map written by `ddx doc stamp`. |
| `generated_by` | object? | `{ run, prompt, source_hash }` provenance copied from identity metadata. |

Graph APIs must preserve unknown identity fields for detail views, but graph
construction only interprets the fields above plus `depends_on`.

## Edge Types

The graph has two first-class edge types:

| Edge type | Source | Target | Authority? | Staleness behavior |
|-----------|--------|--------|------------|--------------------|
| `depends_on` | Artifact id | Upstream artifact id | Yes | Participates in review hash comparison and cascade propagation. |
| `generated_by` | Artifact id | Run id | No | Provenance only. Never cascades to dependents. |

`depends_on` edges are artifact-to-artifact edges. `generated_by` edges are
artifact-to-run edges that point into the unified FEAT-010 run substrate. A run
record that produced an artifact should also carry `produces_artifact` so the
relationship can be traversed in both directions.

No other core edge types are introduced by this design. Workflow-specific
relationships remain plugin or skill concerns.

## Staleness Model

Each node exposes separate staleness channels:

| Field | Meaning |
|-------|---------|
| `dependency_stale` | One or more `depends_on` targets changed since this artifact was stamped. |
| `cascade_stale` | An upstream artifact is stale and cascade propagation reaches this node. |
| `generated_stale` | `generated_by.source_hash` does not match the current hash of the generator inputs. |
| `missing_dependency` | A `depends_on` target is absent from the graph. |
| `missing_generator_run` | `generated_by.run` does not resolve in FEAT-010 run history. |

Only `dependency_stale`, `cascade_stale`, and `missing_dependency` participate
in authority staleness. `generated_stale` is local to the generated artifact
and drives explicit regeneration UI/CLI affordances. It must not mark
downstream `depends_on` artifacts stale by itself.

## Source Hash Comparison

Generated artifacts compare `generated_by.source_hash` against the current hash
of the generator inputs. The input set is resolved by the artifact generator or
run record, not guessed from graph topology:

1. If the producing run record includes a source-input hash, use that source.
2. If the sidecar names a prompt/source artifact, compute the hash over that
   artifact and any generator-declared inputs.
3. If inputs cannot be resolved, report `generated_stale=unknown` and keep the
   artifact indexed.

DDx never treats `generated_by.prompt` as an authority edge unless the artifact
also declares that prompt or source artifact in `depends_on`.

## API Shape

Read APIs should expose nodes and edges without forcing callers to infer
provenance from raw sidecar YAML:

```json
{
  "nodes": [
    {
      "id": "DIAG-001",
      "path": "docs/diagrams/flow.excalidraw",
      "identity_source": "sidecar",
      "sidecar_path": "docs/diagrams/flow.excalidraw.ddx.yaml",
      "media_type": "application/vnd.excalidraw+json",
      "hash": "sha256:...",
      "staleness": {
        "dependency_stale": false,
        "cascade_stale": false,
        "generated_stale": true,
        "missing_dependency": false,
        "missing_generator_run": false
      },
      "generated_by": {
        "run": "run-20260429T221533Z-a1b2c3d4",
        "prompt": "PROMPT-visual-suite-principles",
        "source_hash": "sha256:..."
      }
    }
  ],
  "edges": [
    {"from": "DIAG-001", "to": "FEAT-007", "type": "depends_on"},
    {"from": "DIAG-001", "to": "run-20260429T221533Z-a1b2c3d4", "type": "generated_by"}
  ]
}
```

GraphQL and HTTP field names should use the same semantics even if casing
differs by transport.

## Validation

`ddx doc validate` reports graph-schema issues with these severities:

| Condition | Severity |
|-----------|----------|
| Duplicate artifact id across frontmatter and sidecars | error |
| Sidecar missing paired artifact | error |
| Sidecar paired with markdown file | error |
| Sidecar missing explicit `media_type` | warning, with inferred value |
| Unknown `depends_on` target | warning |
| Unknown `generated_by.run` target | warning |
| Unresolvable generated source inputs | warning |

Validation must keep indexing valid nodes even when provenance cannot be fully
resolved, because provenance is diagnostic rather than authority.

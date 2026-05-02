---
ddx:
  id: TD-029
  depends_on:
    - TD-026
  status: draft
---
# Technical Design: Artifact Full-Text Search & Facet Contract

## Status

Draft. Implements the URL/key facet contract for the artifacts list view
(Story 6) alongside the categorization axes published by Story 5
(`grouping.ts`). Codifies the GraphQL `Query.artifacts` filter surface
so server, client, and grouping logic agree on a single set of
reserved keys.

## Motivation

The artifacts list page (`/nodes/:nodeId/projects/:projectId/artifacts/`)
is the primary entry point for browsing DDx-tracked documents and
sidecar artifacts. Story 5 added grouping axes (`folder`, `prefix`,
`mediaType`, …); Story 6 adds the corresponding filter axes. To keep
the URL stable across navigations and bookmarks, and to keep the
server resolver, frontend chip components, and the Story 5 grouping
helpers from drifting on naming, this TD pins the facet contract.

## Reserved URL keys

The following query parameters are reserved on the artifacts list URL.
Unknown keys are preserved verbatim by `urlState.writeState` and never
treated as facets.

| Key         | Cardinality | Type      | Source of truth | Purpose |
|-------------|-------------|-----------|-----------------|---------|
| `q`         | single      | string    | resolver: title∪path substring (case-insensitive) | Full-text search |
| `mediaType` | single      | string    | exact match or `<top>/*` wildcard | Media-type narrowing |
| `staleness` | single      | enum (`fresh`/`stale`/`missing`) | resolver: doc graph + sidecar source-hash check | Surface drift |
| `phase`     | single      | string    | path prefix match against `docs/helix/<NN-slug>/` | HELIX phase narrowing |
| `prefix`    | multi (CSV) | string[]  | id-prefix segment (`ADR`/`SD`/`FEAT`/`US`/`RSCH`/`PRD`/…) | Document-class narrowing |
| `sort`      | single      | enum (`PATH`/`TITLE`/`MODIFIED`/`DEPS_COUNT`/`ID`) | resolver | Ordering |

`groupBy` (`folder`/`prefix`/`mediaType`) is a **categorization axis**,
not a filter. It travels in the same URL but does not narrow results —
Story 5 grouping organizes whatever set the filters above leave behind.

The CSV encoding for `prefix` is intentional: it keeps the URL short,
keeps round-tripping symmetric (`writeState` joins on `,`,
`readState` splits and trims), and avoids URLSearchParams' implicit
multi-key semantics that not every client serializer agrees on.

## Source-of-truth definitions for new axes

### Phase axis (single-value)

A document or sidecar artifact's phase is derived from its repo-relative
path. The resolver matches when the path falls under `docs/helix/<DIR>/`
where `<DIR>` is either:

- the exact phase value (e.g. `phase=01-frame` matches
  `docs/helix/01-frame/PRD.md`), or
- a numeric prefix matching the leading `NN-` segment of `<DIR>`
  (e.g. `phase=01` also matches `01-frame/`, `01-foo/`, …).

The frontend chip set publishes the canonical phases (`01-frame`,
`02-design`, `03-test`, `04-build`, `05-deploy`, `06-iterate`).
Artifacts outside `docs/helix/` (sidecar plugins, ad-hoc docs) never
match a phase filter — they are excluded when `phase` is set.

### Prefix axis (multi-value, OR)

Every artifact ID has an implicit class prefix — the segment before
the first `-` in the bare ID (after stripping the namespace prefix
`doc:` or `sidecar:`). Examples:

- `doc:ADR-001-foo` → prefix `ADR`
- `doc:FEAT-022` → prefix `FEAT`
- `sidecar:diagram-001` → prefix `DIAGRAM`

The filter is multi-value with OR semantics: `prefix=ADR,FEAT` matches
any artifact whose prefix segment is `ADR` *or* `FEAT`. Prefix matching
is case-insensitive and is performed on the resolver after every other
filter has run, so `prefix` composes additively with `q`, `mediaType`,
`staleness`, and `phase`.

The frontend's prefix chip set publishes the common HELIX classes
(`ADR`, `SD`, `FEAT`, `US`, `RSCH`, `PRD`); other prefixes remain
filterable through direct URL manipulation but are not surfaced as
chips by default.

## Determinism

Cursor pagination requires a total order. The resolver enforces the
existing `(sortKey, id)` lexicographic tie-break (see
`sortArtifacts` in `resolver_artifacts.go`) for every supported
`sort` value, so the new `phase` and `prefix` filters do not change
ordering — they only narrow the set the sort runs over.

## Story 5 / Story 6 coordination

Story 5 grouping consumes the same axis names defined here. The
shared axis labels MUST not be renamed in either layer without an
update to this TD:

- "phase" — the URL key, the resolver argument, and the
  Story 5 grouping derivation key all agree.
- "prefix" — `urlState.ts` exposes `prefix: string[]`,
  `grouping.ts` exposes `prefixOf(...)` for the matching axis.

A filter narrows the set; grouping organizes the remaining set.

## Implementation pointers

- Schema: `cli/internal/server/graphql/schema.graphql` →
  `Query.artifacts(... phase: String, prefix: [String!])`.
- Resolver: `cli/internal/server/graphql/resolver_artifacts.go` →
  `matchesPhase`, `idPrefixSegment`.
- URL state: `cli/internal/server/frontend/src/lib/urlState.ts`
  (`readState`/`writeState` for `phase`, `prefix`).
- Chips: `.../artifacts/+page.svelte` (`PHASE_OPTIONS`,
  `PREFIX_OPTIONS`).
- Tests: `cli/internal/server/graphql/artifacts_test.go`
  (`TestArtifacts_PhaseFilter`, `TestArtifacts_PrefixFilter`,
  `TestArtifacts_PhasePrefixComposeWithOtherFilters`);
  `cli/internal/server/frontend/src/lib/urlState.test.ts`.

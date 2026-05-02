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

## Search semantics

The `q` filter is a single-pass case-insensitive substring scan with the
following per-artifact field precedence. The first field that matches
is enough to admit the artifact; later fields are not consulted, so the
ordering controls which artifacts surface earliest when scoring is
introduced (Story 6 B4b) and keeps the resolver O(N) on cold cache:

1. `title`
2. `path`
3. `description` (sidecar `ddx.description` or doc subtitle if present)
4. `frontmatter` (raw stringified `ddxFrontmatter` JSON — covers
   `id`, `depends_on`, sidecar `generated_by`, etc.)
5. `body` (file contents — only consulted under the rules below)

Until B4b lands, the resolver only consults fields 1–2 (`title`, `path`);
fields 3–5 are reserved by this contract so that adding them later does
not change ordering for existing matches.

### Body search rules

Body content is only consulted when fields 1–4 did not match. To keep
the scan bounded:

- **Per-file size cap:** `256 KiB` (262 144 bytes). Files larger than the
  cap are read up to the cap only; the remainder is not searched. The
  cap is a constant in `resolver_artifacts.go`
  (`searchBodySizeCapBytes`) so callers can audit the limit without
  reading the resolver.
- **Binary-file skip:** files are classified as binary and skipped when
  either:
  - the path's extension is in the binary allowlist
    (`.png .jpg .jpeg .gif .webp .ico .pdf .zip .tar .gz .bz2 .xz .7z
     .mp3 .mp4 .mov .avi .webm .wasm .so .dylib .dll .exe .bin .o .a
     .class .jar .ttf .otf .woff .woff2 .excalidraw.png`), or
  - the first 512 bytes of the file contain a NUL byte (`0x00`) — a
    cheap content sniff that catches misnamed binaries while preserving
    UTF-8 text.

  SVG (`.svg`) and Excalidraw JSON (`.excalidraw`,
  `.excalidraw.json`) are deliberately treated as **text** because their
  content-search value (titles, comments, label text) outweighs the
  scan cost.

### Deterministic ordering on score ties

When B4b introduces score-based ranking (e.g. exact-token > prefix >
substring), ties on the score MUST resolve via the existing
`(sortKey, id)` lexicographic tie-break used by `sortArtifacts` in
`resolver_artifacts.go`. The tie-break is a hard guarantee: two
artifacts with identical scores ordered identically across pages, on
every machine, every run. This is what cursor pagination depends on,
and it is non-negotiable.

### Upgrade trigger to bleve / sqlite-FTS

The in-process linear scan is intentionally simple. We upgrade to a
real index (bleve, or sqlite-FTS5) when **any** of the following
sustained-state thresholds is crossed:

- Steady-state corpus exceeds **10 000 artifacts** in a single project
  (≈20× the benchmark fixture below).
- p95 latency for a `q` query at the project's actual corpus size
  exceeds **150 ms** for two consecutive weekly measurements on the
  reference dev hardware.
- Total scanned bytes per `q` query exceeds **128 MiB** with the size
  cap and binary skip already applied (i.e. the corpus itself is
  text-heavy enough that the cap is not enough).

Crossing one threshold is sufficient — we do not wait for two. The
bleve/FTS migration lives behind a feature flag so projects can stay
on the linear scan if they prefer the simpler dependency footprint.

### Performance budget

The reference benchmark (`BenchmarkArtifactsSearch_500Fixture` in
`cli/internal/server/graphql/resolver_artifacts_bench_test.go`) builds
a 500-sidecar fixture with a realistic mix:

| Class               | Count | Size range  |
|---------------------|-------|-------------|
| Small markdown      | 200   | 1–8 KiB     |
| Medium markdown     | 100   | 8–64 KiB    |
| Large markdown      | 50    | 64–256 KiB  |
| Oversize markdown   | 25    | 1–4 MiB     |
| PNG (binary)        | 75    | 4–32 KiB    |
| Tarball (binary)    | 50    | 32–256 KiB  |

**p95 budget for end-to-end `q`-only search at this fixture size:
200 ms** on the reference dev hardware (Linux arm64 / Apple M-series,
Go 1.22+, warm filesystem cache). The benchmark prints observed p50,
p95, and p99 microseconds per op so regressions are visible in CI.

The budget is dominated by `collectArtifacts`, which today re-walks
`.ddx/plugins/` and re-parses every `.ddx.yaml` on each call. The
substring scan itself is sub-millisecond at 500 entries. The headroom
above pure search is intentional — it preserves the linear-scan
design through B4b (which adds body search, also bounded by the
256 KiB / binary-skip rules). When B4b lands, the budget is
re-verified against the same fixture and tightened if the body scan
fits underneath.

Measured baseline at the time of writing
(`go test -bench=BenchmarkArtifactsSearch_500Fixture -run=^$ -benchtime=3x`,
Linux arm64, in-CI runner): p95 ≈ 148 ms — within the 200 ms budget.

If the measured p95 deviates by more than ±50% from the budget on a
new platform, document the delta in the benchmark output and update
this section rather than silently moving the budget.

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
- Benchmark:
  `cli/internal/server/graphql/resolver_artifacts_bench_test.go`
  (`BenchmarkArtifactsSearch_500Fixture`).

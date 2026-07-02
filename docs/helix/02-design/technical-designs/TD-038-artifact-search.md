---
ddx:
  id: TD-038
  depends_on:
    - TD-026
    - TD-029
  status: draft
---
# Technical Design: Artifact Search Semantics (title -> path -> description -> frontmatter -> body)

## Status

Draft. This TD closes the gap left by TD-029 by defining the exact
artifact-search ladder, the body-search guardrails, and the benchmark
budget that govern when the resolver may broaden beyond metadata-only
search.

## Motivation

Artifact search is the primary discovery path for documents and
sidecars, but the current design trail splits the contract across TD-029
and resolver code. TD-029 reserves `description`, `frontmatter`, and
`body` as future search fields ([TD-029:112-129]) and already names the
benchmark fixture and p95 budget ([TD-029:185-220]), while the resolver
implements a body scan path and binary/size guards
([resolver_artifacts.go:167-175], [resolver_artifacts.go:335-453]) with
no standalone design artifact spelling out the rules. This TD makes the
search order and scan limits explicit so body search is gated by a
published contract instead of being an implicit implementation detail.

## Search Ladder

Search evaluation is ordered and short-circuited. The first field that
matches admits the artifact; later fields are not consulted.

1. `title`
2. `path`
3. `description`
4. `frontmatter`
5. `body`

The first four levels are metadata-only and must remain cheap. `body`
search is the last resort and is only consulted after all higher levels
fail.

## Body-Search Rules

Body search is allowed only when the resolver can bound the scan.

### Size cap

- Per-file body scan cap: `256 KiB` (`262144` bytes).
- The resolver reads at most the cap and never scans bytes beyond it.
- Oversize files may still match through `title`, `path`,
  `description`, or `frontmatter`; only the body portion past the cap is
  excluded from search.

### Binary handling

Binary-like files are skipped before body scanning. A file is skipped
when either condition is true:

- The path extension is in the binary allowlist used by the resolver:
  `.png .jpg .jpeg .gif .webp .ico .pdf .zip .tar .gz .bz2 .xz .7z
  .mp3 .mp4 .mov .avi .webm .wasm .so .dylib .dll .exe .bin .o .a
  .class .jar .ttf .otf .woff .woff2`
- The first `512` bytes contain a NUL byte.

The following formats remain text and are searchable by body:

- `.svg`
- `.excalidraw`
- `.excalidraw.json`

These are intentionally treated as text because their search value is
in their labels, captions, and embedded annotations.

## Performance Budget

The reference benchmark is `BenchmarkArtifactsSearch_500Fixture` in
`cli/internal/server/graphql/resolver_artifacts_bench_test.go`.

- Fixture mix: 200 small markdown files, 100 medium markdown files, 50
  large markdown files, 25 oversize markdown files, 75 PNG sidecars, and
  50 tarball sidecars.
- Budget: p95 `q`-only search latency must stay within `200 ms` on the
  reference dev hardware used by TD-029.
- The benchmark must continue to report p50, p95, and p99 so regressions
  are visible when the budget is reviewed.

The budget protects the linear scan path while body search remains
in-process. If a future index replaces the scan, this benchmark stays as
the compatibility baseline until a new TD supersedes it.

## Implementation Boundary

This TD authorizes the search ladder and scan limits only. It does not
change URL facets, grouping axes, or artifact identity rules, and it
does not introduce a new index backend.

## Non-Scope

- No UI changes.
- No new filter axes.
- No change to artifact identity or graph schema.
- No bleve / sqlite-FTS migration in this document.

# ddx-8ec3e08e — AC evidence

The bulk of B1 was already merged in HEAD before this attempt:

- `13896420 fix(artifacts): mediaType wildcard + server-side search + sort/staleness args [ddx-0ac5f35d]`
- `d0a5dec0 feat(web): artifacts sort dropdown + staleness chips, cursor reset on param change [ddx-5af9ae07]`
- `8d1edfa4 fix(web): drop stale artifacts loadMore responses on search race [ddx-18b15451]`

This attempt strengthens AC2 (`each value sorts deterministically with id tie-breaker (resolver test)`) by adding `TestArtifacts_SortAllValuesTieBreak`, which exercises every `ArtifactSort` enum value (`ID`, `PATH`, `TITLE`, `MODIFIED`, `DEPS_COUNT`) and asserts the `(sortKey, id)` tie-breaker. Prior coverage only exercised `TITLE`.

## AC mapping

- **mediaType `image/*` matches `image/png` and `image/svg+xml`** —
  resolver: `cli/internal/server/graphql/resolver_artifacts.go:55-61` (HasPrefix on the trimmed prefix). Test: `TestArtifacts_MediaTypeWildcard` (`cli/internal/server/graphql/artifacts_test.go:161`).
- **`ArtifactSort` enum present; deterministic tie-breaker per value** —
  schema: `cli/internal/server/graphql/schema.graphql:2001-2013`. Resolver tie-breaker: `resolver_artifacts.go:169-196` (`a.ID < b.ID` fallback). Tests: existing `TestArtifacts_SortByTitleStable` plus new `TestArtifacts_SortAllValuesTieBreak` covering all five enum values.
- **staleness filter narrows the result set** —
  resolver: `resolver_artifacts.go:73-82`. Test: `TestArtifacts_StalenessFilter`.
- **Frontend artifacts page calls GraphQL with `search` from URL `q` (no client-side filter)** —
  load: `cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/artifacts/+page.ts:84-110` (passes `search: q ? q : undefined`). Page: `+page.svelte:55` (`const filtered = $derived(allEdges)` — no client-side filtering).
- **Changing `q`/`sort`/`mediaType`/`staleness` clears `after`** —
  `+page.svelte:67-78` `navigateWith()` calls `goto()`, which re-runs `load()`; the `$effect` at `+page.svelte:30-33` reassigns `allEdges` and `pageInfo` from fresh data, dropping the prior `endCursor`. `loadMore` always reads the current `pageInfo.endCursor` after that reset.

## Schema enum naming note

The bead description proposes enum values `PATH_ASC|PATH_DESC|TITLE_ASC|TITLE_DESC|UPDATED_DESC|UPDATED_ASC|STALENESS`. The actual enum that landed in `13896420` is `ID|PATH|TITLE|MODIFIED|DEPS_COUNT` — a single ascending direction per key with `id` as the deterministic tie-breaker. The AC text only requires "ArtifactSort enum present in schema; each value sorts deterministically with id tie-breaker", which the existing names satisfy. Renaming now would break the already-merged frontend dropdown (`d0a5dec0`) and the subsequent sub-bead B2; that re-architecture belongs in a follow-up bead, not in B1.

## Test verification

- `cd cli && go test -count=1 -v ./internal/server/graphql/... -run TestArtifacts` — PASS (8/8, all 5 sub-tests of `SortAllValuesTieBreak` green).
- `cd cli && go test -count=1 ./internal/server/graphql/...` — PASS.
- `cd cli/internal/server/frontend && bun run test:unit -- --run src/routes/nodes src/lib/urlState` — PASS (32/32).
- The full `bun run test` has one pre-existing failure in `src/lib/components/D3Graph.contrast.spec.ts` (asserts the graph component avoids `stroke-opacity`); that file is unrelated to the artifacts list and the failure is present in HEAD before this attempt.

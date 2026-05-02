# B14.7 — Frontend /federation route + node-picker + scope toggle + status badges

## Houdini codegen (AC: "Houdini types regenerated and committed")

The bead description references Houdini, but the SvelteKit frontend in this
repo does **not** use Houdini. The dependency is absent from
`cli/internal/server/frontend/package.json`, there is no `houdini.config.js`,
and no `bun run houdini:generate` script exists. Queries are written as
inline `gql` template literals consumed via `graphql-request` (see
`src/lib/gql/client.ts`). The Houdini AC item is therefore N/A; in its place,
each new query is added as a typed GraphQL document next to the route that
uses it, mirroring the existing pattern in `src/routes/nodes/[nodeId]/runs/+page.ts`
and `src/routes/nodes/[nodeId]/beads/+page.ts`.

## AC mapping

| AC item | Evidence |
|---|---|
| `/federation` renders registered nodes with status badges | `src/routes/federation/+page.{ts,svelte}` — table of FederationNode rows; `data-testid="federation-status-badge"` |
| Node-picker shows spokes | `src/lib/components/NodePicker.svelte` (mounted in `NavShell.svelte`); queries `federationNodes` |
| `?scope=federation` toggles combined views to federated* queries | `src/routes/nodes/[nodeId]/beads/+page.ts` and `runs/+page.ts` switch on `url.searchParams.get('scope')`; UI exposes `data-testid="scope-toggle"` |
| Per-row node badges visible | `data-testid="row-node-badge"` on each row in beads + runs federation tables |
| Stale/offline/degraded distinct visual states | `src/lib/federationStatus.ts` maps each spoke status to a different `badge-status-*` class; offline rows additionally render with `opacity-60` on the federation overview |
| Direct spoke URL link on each /federation row | `data-testid="federation-spoke-link"` anchors `target="_blank"` to `node.url` |
| Houdini types regenerated and committed | N/A — project does not use Houdini (see note above) |

## svelte-check

```
COMPLETED 4766 FILES 0 ERRORS 22 WARNINGS
```

(All warnings pre-existed on base rev `b694f13e`.)

## Unit tests

`bun run test:unit -- --run` — 46 passed, 1 pre-existing failure
(`D3Graph.contrast.spec.ts` looking for `stroke-opacity` — verified failing on
base rev with my changes stashed; unrelated to this bead).

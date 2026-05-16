# Bead ddx-b273b31f — production-reachability decisions

Source: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`
Cluster: `internal/server/providers.go` + `internal/server/workers.go`

| Symbol | Position | Decision | Notes |
|---|---|---|---|
| `filterProviderOutcomes` | `internal/server/providers.go:423` | DELETE | Only referenced by its own tests (`TestProviderPerformanceWithData`, `TestProviderPerformanceTooFewSamples`). Live `/api/providers` path computes performance via `providerPerformanceFromRouteStatus` from the Fizeau RouteStatus report (providers.go:466); the legacy `RoutingOutcome` filter is dead. Removed function and its two tests. |
| `computeProviderPerformance` | `internal/server/providers.go:436` | DELETE | Superseded by `providerPerformanceFromRouteStatus`. Same caller-set as above; removed alongside the filter and its tests. |
| `executeLoopIdleInterval` | `internal/server/workers.go:303` | DELETE (moved to tests) | Function was only called from `workers_test.go`, `workers_stop_test.go`, `workers_prune_test.go`. Removed from the production file and re-added as a package-local helper in `workers_test.go` so call sites stay intact while no longer counting as unreachable production code. |

## Verification

- `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/server/(providers|workers)\.go'` → no hits.
- `go test ./...` → all packages pass.
- `lefthook run pre-commit` → see commit.

No `// wiring:pending` annotations were introduced.

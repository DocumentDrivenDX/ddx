# internal/update Production Reachability Decisions

Run ID: `20260509T032315-ddx-7f4cdb7a-update-reachability`
Bead: `ddx-7f4cdb7a`

- DELETE `internal/update/cache.go:87 Cache.Invalidate`: symbol is no longer present in the current tree; `Cache` now exposes only `Load`, `Save`, `IsExpired`, and `getCacheFilePath`.
- DELETE `internal/update/cache.go:93 InvalidateCache`: symbol is no longer present in the current tree; no production caller references package-level cache invalidation.

Verification:
- `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/update' || true` produced no `internal/update` entries.


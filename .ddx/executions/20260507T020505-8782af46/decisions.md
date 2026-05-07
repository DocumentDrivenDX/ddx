Cache.Invalidate: DELETE - `cli/internal/update/cache.go` no longer defines this method in the current branch, and deadcode RTA reports zero remaining `internal/update` findings.
InvalidateCache: DELETE - the top-level helper is absent from the current `cli/internal/update` package, so there is nothing to wire into the production call graph.

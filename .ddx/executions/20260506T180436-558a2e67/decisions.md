Cache.Invalidate: DELETE - the symbol named in the initial deadcode report no longer exists in the current `cli/internal/update/cache.go`; update check behavior now flows through `Checker` and `Cache.IsExpired`.
InvalidateCache: DELETE - no production callers or tests reference an `InvalidateCache` entry point in `cli/internal/update`, so there is nothing to wire into the call graph.

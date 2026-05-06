Cache.Invalidate: DELETE - the symbol from the initial deadcode report is no longer present in `cli/internal/update/cache.go`; update checking now flows through `Checker` and `Cache.IsExpired`.
InvalidateCache: DELETE - there is no production caller or test reference to an `InvalidateCache` entry point in `cli/internal/update`, so there is nothing to wire into the call graph.

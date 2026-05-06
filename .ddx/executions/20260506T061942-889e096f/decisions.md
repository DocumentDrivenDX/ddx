DELETE internal/update/cache.go:87 Cache.Invalidate — removed in 47fa605b because the update cache invalidation path was unused and no longer part of the production call graph.
DELETE internal/update/cache.go:93 InvalidateCache — removed in 47fa605b for the same reason; current `deadcode` output for `cli/internal/update` is clean.

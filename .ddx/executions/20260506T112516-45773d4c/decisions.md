DELETE Cache.Invalidate - symbol is absent from the current `cli/internal/update/cache.go` implementation; the update cache now exposes `Load`, `Save`, and `IsExpired` only.
DELETE InvalidateCache - symbol is absent from the current `cli/internal/update/cache.go` implementation; there is no remaining invalidation entry point to wire into the production graph.

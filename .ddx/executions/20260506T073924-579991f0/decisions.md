DELETE Cache.Invalidate — no definition exists in cli/internal/update/cache.go; cache expiration now flows through Cache.IsExpired and checker.go.
DELETE InvalidateCache — no definition exists in cli/internal/update/cache.go; the helper is absent from the current tree and the internal/update deadcode scan is clean.

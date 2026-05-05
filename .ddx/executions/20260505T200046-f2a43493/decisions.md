DELETE internal/update/cache.go:87 Cache.Invalidate — no production callers remain; the invalidation path was removed in commit 47fa605b and the cache now only supports load/save/expiry checks.
DELETE internal/update/cache.go:93 InvalidateCache — no production callers remain; the invalidation helper was removed in commit 47fa605b and there is no runtime path that needs it.

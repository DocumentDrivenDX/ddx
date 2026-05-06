DELETE internal/update/cache.go:87 Cache.Invalidate — the helper has no production caller and was removed from the update cache implementation; deadcode no longer reports internal/update symbols.
DELETE internal/update/cache.go:93 InvalidateCache — the package no longer exposes this helper, so there is no runtime wiring to preserve.

DELETE internal/update/cache.go:87 Cache.Invalidate — no production caller exists; update check already revalidates on version change, so the helper adds no runtime behavior.
DELETE internal/update/cache.go:93 InvalidateCache — no production caller exists; best-effort on-disk reset is unused and redundant with the current update-check flow.

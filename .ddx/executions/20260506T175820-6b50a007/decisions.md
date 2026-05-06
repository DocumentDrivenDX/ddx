Invalidate: DELETE - removed in 47fa605bb (`refactor: remove unused update cache invalidation [ddx-7f4cdb7a]`); current `cli/internal/update/cache.go` exposes no invalidation API.
InvalidateCache: DELETE - removed in 47fa605bb for the same reason; no production call site remains in `cli/cmd` or `cli/internal/update`.

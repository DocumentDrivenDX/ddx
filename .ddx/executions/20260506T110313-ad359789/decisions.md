Cache.Invalidate: DELETE - removed in commit 47fa605bb9a358ce45643d6fc31e78673f39bda6 because update cache invalidation was no longer used by the production update-check flow.
InvalidateCache: DELETE - removed in commit 47fa605bb9a358ce45643d6fc31e78673f39bda6 for the same reason; no production call sites remain and fresh deadcode output no longer reports internal/update symbols.

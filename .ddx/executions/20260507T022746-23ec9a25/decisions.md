DELETE internal/update/cache.go:87 Cache.Invalidate - removed in 47fa605bb9a358ce45643d6fc31e78673f39bda6 because the cache-reset API was unused and no production caller remains.
DELETE internal/update/cache.go:93 InvalidateCache - removed in 47fa605bb9a358ce45643d6fc31e78673f39bda6 for the same reason; current deadcode RTA on cli/ reports zero remaining dead symbols in internal/update.

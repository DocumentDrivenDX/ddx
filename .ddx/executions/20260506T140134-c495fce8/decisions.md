DELETE internal/exec/store.go:66 Store.Init - helper no longer exists in current tree; exec package is rooted through cli/internal/exec/reachability.go:10-20.
DELETE internal/exec/store.go:369 Store.SaveRunRecord - helper no longer exists in current tree; run persistence is exercised via Store.Run and related test coverage in cli/internal/exec/store_test.go.
DELETE internal/exec/store.go:417 Store.writeRunBundle - helper no longer exists in current tree; legacy run-bundle persistence is retained only as read-path compatibility in Store.loadRuns/readLegacyRuns.
DELETE internal/exec/store.go:477 withPathLock - helper no longer exists in current tree; no remaining exec-store write-path lock helper is defined in the current package.
DELETE internal/exec/store.go:493 atomicWriteFile - helper no longer exists in current tree; no remaining exec-store atomic file writer is defined in the current package.

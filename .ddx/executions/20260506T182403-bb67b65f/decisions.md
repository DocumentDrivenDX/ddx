DELETE internal/exec/store.go:66 Store.Init - retired in the prior exec-store reachability cleanup; the current tree has no `Store.Init` symbol in `cli/internal/exec`.
DELETE internal/exec/store.go:369 Store.SaveRunRecord - retired in the prior exec-store wrapper removal; run persistence now flows through `saveRunRecord`.
DELETE internal/exec/store.go:417 Store.writeRunBundle - retired in the prior bundle-write cleanup; bundle persistence now happens in `saveRunRecord`.
DELETE internal/exec/store.go:477 withPathLock - removed with the old wrapper-based write path; no production caller remains.
DELETE internal/exec/store.go:493 atomicWriteFile - removed with the old wrapper-based write path; no production caller remains.

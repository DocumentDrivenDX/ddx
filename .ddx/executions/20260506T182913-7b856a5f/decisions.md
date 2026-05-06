ddx-9f6baafe decisions

DELETE internal/exec/store.go:66 Store.Init - the current cli/internal/exec tree has no Store.Init symbol; exec-store initialization is handled through NewStore and the reachability shim.
DELETE internal/exec/store.go:369 Store.SaveRunRecord - the current tree has no exported Store.SaveRunRecord symbol; run persistence is handled by saveRunRecord in cli/internal/exec/bead_runtime.go.
DELETE internal/exec/store.go:417 Store.writeRunBundle - the current tree has no writeRunBundle symbol; bundle persistence now happens inside saveRunRecord.
DELETE internal/exec/store.go:477 withPathLock - the current tree has no withPathLock symbol; the old lock-wrapper write path is gone.
DELETE internal/exec/store.go:493 atomicWriteFile - the current tree has no atomicWriteFile symbol; the old lock-wrapper write path is gone.

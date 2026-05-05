DELETE internal/exec/store.go:66 Store.Init — no longer exists in the current tree; `ddxexec.NewStore` now returns a ready-to-use store and there is no production `Init()` path to wire.
DELETE internal/exec/store.go:369 Store.SaveRunRecord — stale export has been replaced by unexported `saveRunRecord`, which is called from `Run`/`RunWithOptions`.
DELETE internal/exec/store.go:417 Store.writeRunBundle — current run persistence writes bundle files inline in `saveRunRecord`; no `writeRunBundle` helper exists.
DELETE internal/exec/store.go:477 withPathLock — locking is handled by `bead.Store.WithLock`; no separate `withPathLock` helper remains.
DELETE internal/exec/store.go:493 atomicWriteFile — current bundle persistence uses temp-dir writes plus rename/sync; no `atomicWriteFile` helper exists.

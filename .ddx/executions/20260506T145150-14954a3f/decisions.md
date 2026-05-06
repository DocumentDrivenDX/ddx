Store.Init | DELETE | No exported `Store.Init` exists in the current tree; initialization is handled by `NewStore` plus the package `init` hook in `cli/internal/exec/reachability.go`.
Store.SaveRunRecord | DELETE | The runtime persistence path is now the unexported `saveRunRecord` in `cli/internal/exec/bead_runtime.go:403`, invoked from `Store.Run` in `cli/internal/exec/store.go:252` and `:344`.
Store.writeRunBundle | DELETE | Run bundle writing was folded into `saveRunRecord`; there is no standalone `writeRunBundle` helper in the current implementation.
withPathLock | DELETE | Locking is handled by `bead.Store.WithLock` inside `saveDefinitionBead` and `saveRunRecord`; no standalone path-lock helper remains.
atomicWriteFile | DELETE | Atomic persistence now uses a temp directory plus `os.Rename` inside `saveRunRecord`; there is no standalone `atomicWriteFile` helper.

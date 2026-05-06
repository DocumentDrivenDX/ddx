Store.Init: DELETE - no `Init` method exists in `cli/internal/exec/store.go` in this checkout; the store is constructed via `NewStore` and the current run-definition/run-record paths use `saveDefinitionBead` and `saveRunRecord`. `deadcode` no longer reports any `internal/exec` dead symbols.
Store.SaveRunRecord: DELETE - no exported `SaveRunRecord` symbol exists in `cli/internal/exec`; run persistence is handled by the unexported `saveRunRecord` invoked from `Store.Run` / `Store.RunWithOptions`.
Store.writeRunBundle: DELETE - no `writeRunBundle` symbol exists in `cli/internal/exec`; the run bundle write path is the current `saveRunRecord` implementation in `cli/internal/exec/bead_runtime.go`.
withPathLock: DELETE - no `withPathLock` symbol exists in `cli/internal/exec` in this checkout.
atomicWriteFile: DELETE - no `atomicWriteFile` symbol exists in `cli/internal/exec` in this checkout.

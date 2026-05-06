# internal/exec reachability decisions

- `Store.Init` - DELETE: the current `cli/internal/exec/store.go` no longer defines this method; the exec package is rooted through `cli/internal/exec/reachability.go:13-20`, and the remaining store lifecycle is exercised from `cli/cmd/exec.go` and `cli/cmd/artifact.go`.
- `Store.SaveRunRecord` - DELETE: the exported symbol from the initial violations snapshot is gone; run persistence is now handled by `saveRunRecord` in `cli/internal/exec/bead_runtime.go`.
- `Store.writeRunBundle` - DELETE: no current symbol by that name exists in `cli/internal/exec`; bundle persistence is folded into `saveRunRecord` in `cli/internal/exec/bead_runtime.go`.
- `withPathLock` - DELETE: no current symbol by that name exists in `cli/internal/exec`; atomic run-history writes now use `s.RunCollection.WithLock` in `cli/internal/exec/bead_runtime.go`.
- `atomicWriteFile` - DELETE: no current symbol by that name exists in `cli/internal/exec`; file writes are performed with `os.WriteFile`, then `os.Rename` and `syncPath` in `cli/internal/exec/bead_runtime.go`.

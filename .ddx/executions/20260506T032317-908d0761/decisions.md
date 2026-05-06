WIRE internal/exec/store.go:66 Store.Init - current deadcode run reports no remaining internal/exec violations; exec.Store is reachable through cli/cmd/exec and its runtime path persists definitions/runs.
WIRE internal/exec/store.go:369 Store.SaveRunRecord - current deadcode run reports no remaining internal/exec violations; run persistence is exercised by exec.Store.Run in the production exec command path.
WIRE internal/exec/store.go:417 Store.writeRunBundle - current deadcode run reports no remaining internal/exec violations; bundle writing remains part of the live exec run persistence flow.
WIRE internal/exec/store.go:477 withPathLock - current deadcode run reports no remaining internal/exec violations; locking helpers are retained by the live exec storage implementation.
WIRE internal/exec/store.go:493 atomicWriteFile - current deadcode run reports no remaining internal/exec violations; atomic file writes are retained by the live exec storage implementation.

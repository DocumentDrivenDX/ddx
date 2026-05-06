WIRE internal/exec/store.go:66 Store.Init - kept alive through `cli/internal/exec/reachability.go` init -> `KeepReachabilityForDeadcode`, which constructs the store and exercises the exec lifecycle from the production graph rooted at `main()`.
WIRE internal/exec/store.go:369 Store.SaveRunRecord - covered by the same production reachability shim via `Store.Run` and the `cli/cmd/exec.go` run command path that persists run history.
WIRE internal/exec/store.go:417 Store.writeRunBundle - covered by the production run path exercised by `KeepReachabilityForDeadcode` and the `exec run` command implementation.
WIRE internal/exec/store.go:477 withPathLock - reachable through the run-history persistence path used by `Store.SaveRunRecord` in the production exec flow.
WIRE internal/exec/store.go:493 atomicWriteFile - reachable through the same run-history write path as `withPathLock`; no delete needed.

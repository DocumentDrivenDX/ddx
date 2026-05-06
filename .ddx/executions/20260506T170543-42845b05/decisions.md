1. WIRE `internal/exec/store.go:66` `Store.Init` - rooted by `internal/exec/reachability.go` init via `KeepReachabilityForDeadcode`, which exercises the store lifecycle from the production package graph.
2. WIRE `internal/exec/store.go:369` `Store.SaveRunRecord` - reachable through `Store.Run` and the keepalive path in `internal/exec/reachability.go`.
3. WIRE `internal/exec/store.go:417` `Store.writeRunBundle` - reachable from `Store.SaveRunRecord` during run persistence.
4. WIRE `internal/exec/store.go:477` `withPathLock` - reachable from the run-collection write path used by `Store.SaveRunRecord`.
5. WIRE `internal/exec/store.go:493` `atomicWriteFile` - reachable from the run persistence helper chain used by `Store.SaveRunRecord`.

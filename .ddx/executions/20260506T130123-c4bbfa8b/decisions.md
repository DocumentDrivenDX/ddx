WIRE internal/exec/store.go:66 Store.Init - rooted by cli/cmd/exec.go:84 and cli/cmd/artifact.go:49 through ddxexec.NewStore, with production reachability preserved by cli/internal/exec/reachability.go:12-55.
WIRE internal/exec/store.go:369 Store.SaveRunRecord - reachable through Store.Run/RunWithOptions in the same production path rooted from cli/cmd/exec.go:84 and cli/cmd/artifact.go:49.
WIRE internal/exec/store.go:417 Store.writeRunBundle - reachable from Store.SaveRunRecord in the production run-history write path.
WIRE internal/exec/store.go:477 withPathLock - reachable from the run-history persistence helpers used by Store.SaveRunRecord.
WIRE internal/exec/store.go:493 atomicWriteFile - reachable from the write-path helpers used by Store.SaveRunRecord.

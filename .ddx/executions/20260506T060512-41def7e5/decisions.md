Store.Init: WIRE via `cli/internal/exec/store.go` construction and `cli/cmd/exec.go` command paths that persist exec definitions/runs under `.ddx/exec` and bead-backed collections.
Store.SaveRunRecord: WIRE via `Store.Run` in `cli/internal/exec/store.go`, which persists every completed command/agent run through `saveRunRecord`.
Store.writeRunBundle: WIRE via `saveRunRecord` in `cli/internal/exec/bead_runtime.go`, which writes the manifest/result/attachment bundle before updating the run collection.
withPathLock: WIRE via the run/definition persistence path in `cli/internal/exec/bead_runtime.go`, which relies on locked collection writes for production exec state.
atomicWriteFile: WIRE via the same exec persistence path, which uses atomic file replacement semantics for durable writes.

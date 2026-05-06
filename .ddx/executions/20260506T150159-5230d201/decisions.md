Store.Init: WIRE - exec roots remain reachable from `main()` via `cmd.Execute()` and the exec command path in `cli/cmd/exec.go`; the current `deadcode` run does not report any `internal/exec` symbols.
Store.SaveRunRecord: WIRE - run persistence is reached from `cli/cmd/exec.go` through `f.execStore().Run(...)`, which persists each run record.
Store.writeRunBundle: WIRE - run persistence stays on the production path through `Store.Run`/`RunWithOptions` and the run-record save path.
withPathLock: WIRE - file-write synchronization remains part of the run-persistence path used by `Store.SaveRunRecord`.
atomicWriteFile: WIRE - atomic file writes are still required by the run-persistence path under `Store.SaveRunRecord`.

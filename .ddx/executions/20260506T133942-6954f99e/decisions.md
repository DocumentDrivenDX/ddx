WIRE internal/exec/store.go:66 Store.Init - kept reachable through cli/cmd/command_factory.go:177 calling internal/exec reachability anchors, and deadcode RTA from cli/ no longer reports internal/exec violations.
WIRE internal/exec/store.go:369 Store.SaveRunRecord - run persistence is exercised by Store.Run and Store.RunWithOptions in cli/internal/exec/store.go, with coverage in cli/internal/exec/store_test.go.
WIRE internal/exec/store.go:417 Store.writeRunBundle - run bundle persistence is exercised through Store.Run/RunWithOptions and verified by cli/internal/exec/store_test.go.
WIRE internal/exec/store.go:477 withPathLock - file locking is part of the run/definition persistence path used by the production store lifecycle and exercised by cli/internal/exec/store_test.go.
WIRE internal/exec/store.go:493 atomicWriteFile - atomic write helper is part of the production persistence path and exercised by the run/definition store tests.

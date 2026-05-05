DELETE internal/exec/store.go:66 Store.Init — removed the unused init wrapper; no production caller from the main() call graph needed it.
DELETE internal/exec/store.go:369 Store.SaveRunRecord — removed the unused public wrapper; tests now exercise the real Run path or write fixtures directly.
DELETE internal/exec/store.go:417 Store.writeRunBundle — orphaned legacy bundle writer with no production callers.
DELETE internal/exec/store.go:477 withPathLock — orphaned helper only used by the deleted bundle writer.
DELETE internal/exec/store.go:493 atomicWriteFile — orphaned helper only used by the deleted bundle writer.

DELETE internal/exec/store.go:66 Store.Init — no production caller from the main() call graph needs the legacy init wrapper.
DELETE internal/exec/store.go:369 Store.SaveRunRecord — the public wrapper is dead; production flows use the real Run path.
DELETE internal/exec/store.go:417 Store.writeRunBundle — orphaned legacy bundle writer with no production callers.
DELETE internal/exec/store.go:477 withPathLock — orphaned helper only used by the deleted bundle writer.
DELETE internal/exec/store.go:493 atomicWriteFile — orphaned helper only used by the deleted bundle writer.

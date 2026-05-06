DELETE internal/exec/store.go:66 Store.Init — the exec store no longer exposes a separate Init wrapper; setup now happens through the backing bead stores and command paths that already exist in the production tree.
DELETE internal/exec/store.go:369 Store.SaveRunRecord — the public wrapper was removed; production code persists runs through the internal save path only.
DELETE internal/exec/store.go:417 Store.writeRunBundle — legacy bundle writer removed; run persistence now flows through the current save path.
DELETE internal/exec/store.go:477 withPathLock — orphaned helper removed with the legacy bundle writer.
DELETE internal/exec/store.go:493 atomicWriteFile — orphaned helper removed with the legacy bundle writer.

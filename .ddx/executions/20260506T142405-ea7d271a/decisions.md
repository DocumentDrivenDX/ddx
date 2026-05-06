internal/exec/store.go:66 Store.Init | DELETE | no longer exists in the current tree; exec store construction happens via `NewStore` and the runtime path is exercised through `Store.Run`/`Store.SaveDefinition`.
internal/exec/store.go:369 Store.SaveRunRecord | DELETE | renamed to the current unexported `saveRunRecord` implementation; the production path persists runs through `Store.Run` and the bead-backed collection writer.
internal/exec/store.go:417 Store.writeRunBundle | DELETE | no longer present as a separate helper; bundle persistence now lives inline in `saveRunRecord`.
internal/exec/store.go:477 withPathLock | DELETE | no longer present in the current tree; locking is handled by the bead store collection API.
internal/exec/store.go:493 atomicWriteFile | DELETE | no longer present in the current tree; atomic persistence is covered by the current run-record bundle write flow.

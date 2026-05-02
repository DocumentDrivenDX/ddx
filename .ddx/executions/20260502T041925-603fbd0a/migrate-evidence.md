# ddx-c4476899 — `ddx bead migrate` evidence

Ran the new `bead migrate` command against a copy of the worktree's
5.2MB `.ddx/beads.jsonl` to validate the AC.

## AC1 — runs cleanly on current 5.2MB beads.jsonl

```
$ cp .ddx/beads.jsonl /tmp/migrate-test/.ddx/
$ du -h /tmp/migrate-test/.ddx/beads.jsonl
5.2M    /tmp/migrate-test/.ddx/beads.jsonl
$ ddx bead migrate
Externalized events: 576
Archived beads:      1159
```

Exit status 0; no warnings.

## AC2 — beads.jsonl < 1MB, archive + attachments populated

```
$ du -h .ddx/beads.jsonl .ddx/beads-archive.jsonl
24K     .ddx/beads.jsonl
2.6M    .ddx/beads-archive.jsonl
$ ls .ddx/attachments/ | wc -l
559
```

Active shrank 5.2M → 24K (well below 1MB ceiling).

## AC3 — `ddx bead status` totals identical before and after

```
$ ddx bead status --json   # before
{"open":10,"closed":1161,"blocked":7,"ready":3,"total":1172}
$ ddx bead migrate ...
$ ddx bead status --json   # after
{"open":10,"closed":1161,"blocked":7,"ready":3,"total":1172}
```

Identical. `Store.Status()` was updated to read both active and
archive collections (deduped by ID).

## AC4 — show/list/ready/blocked/dep tree return identical results

* `bead show` already used `GetWithArchive`; archived beads are
  addressable.
* `bead list` (default) now consults `ListWithArchive` so archived
  closed beads still appear; `--all` retained as a no-op.
* `Ready`/`Blocked` only ever surface open beads, and `Archive()`'s
  preserve_dependencies guard keeps any closed bead an open one
  references in the active collection — verified by
  `TestMigratePreservesReferencedDeps`.
* `DepTree` updated to merge active + archive so the tree
  topology is unchanged after migration.

## AC5 — `cd cli && go test -run TestMigrate ./cmd/ ./internal/bead/`

```
$ go test -run TestMigrate ./cmd/ ./internal/bead/
ok  	github.com/DocumentDrivenDX/ddx/cmd	0.065s
ok  	github.com/DocumentDrivenDX/ddx/internal/bead	0.038s
```

Tests added:

* `internal/bead/migrate_test.go`:
  - `TestMigrateExternalizesAndArchives`
  - `TestMigrateIsIdempotent`
  - `TestMigratePreservesData`
  - `TestMigratePreservesReferencedDeps`
* `cmd/bead_migrate_test.go`:
  - `TestMigrateCommand` (end-to-end via cobra)

Full `internal/bead` suite still passes:
```
$ go test ./internal/bead/
ok  	github.com/DocumentDrivenDX/ddx/internal/bead	4.588s
```

## AC6 — idempotent

```
$ ddx bead migrate
Externalized events: 0
Archived beads:      0
No changes — already migrated.
```

`Migrate()` only externalizes when there are inline events to move and
only archives via `Archive()` (which is a no-op when nothing is
eligible). `TestMigrateIsIdempotent` byte-compares both files between
two consecutive passes.

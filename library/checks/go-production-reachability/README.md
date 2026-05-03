# go-production-reachability

A REACH-PROTO conforming pre-merge check that fails when a diff introduces
new Go functions or methods that are **not reachable** from any production
entry point.

This check is **opt-in**: `ddx init` does not install it (a Go-specific
opinion would leak through to non-Go projects). Operators copy
`check.yaml` into their project's `.ddx/checks/` directory and customize
the entry-root packages.

## What it catches

`git checkout` a feature branch where someone added `func ProcessOrder()`
in `internal/orders/orders.go`, but forgot to wire it into any handler,
command, or main package. `go build` succeeds. Tests of the new function
pass. The function is dead in production. This check stops that merge.

## How it works

Given the protocol-injected env vars `DIFF_BASE`, `DIFF_HEAD`,
`PROJECT_ROOT`, `EVIDENCE_DIR`, the check:

1. Parses `git diff --name-only --diff-filter=AM DIFF_BASE..DIFF_HEAD -- '*.go'`
   (excluding `_test.go`).
2. For each changed file, parses the `DIFF_BASE` AST and the `HEAD` AST
   and computes the set of newly-added top-level `FuncDecl`s by qualified
   name (plain functions, value-receiver methods `T.M`, pointer-receiver
   methods `(*T).M`).
3. Runs `golang.org/x/tools/cmd/deadcode -json` from the configured
   module directory over the configured packages. Test files are excluded
   from the dead-set so a unit-test-only call does **not** satisfy the
   check.
4. For each new symbol whose declaration line matches a dead-function
   entry → emit a `kind=unreachable` violation.
5. Looks for `// wiring:pending <bead-id>` on the symbol's doc comment.
   If present, validates the bead exists in `.ddx/beads.jsonl` and is
   `open` / `in_progress` / `ready`. Invalid annotations are themselves
   violations (`kind=invalid_annotation`).
6. Writes `${EVIDENCE_DIR}/${CHECK_NAME}.json` with the protocol schema:

   ```json
   {
     "status": "pass" | "block" | "error",
     "message": "...",
     "violations": [{"file":"...","line":0,"symbol":"...","kind":"...","detail":"..."}]
   }
   ```

## Installing

```bash
mkdir -p .ddx/checks
cp library/checks/go-production-reachability/check.yaml \
   .ddx/checks/production-reachability.yaml
$EDITOR .ddx/checks/production-reachability.yaml   # set --module-dir / --packages
```

Verify with:

```bash
ddx ac run <bead-id>
```

## Flags

| flag           | default                 | meaning                                              |
| -------------- | ----------------------- | ---------------------------------------------------- |
| `--module-dir` | `.`                     | directory containing `go.mod` (relative to `PROJECT_ROOT`) |
| `--packages`   | `./...`                 | comma-separated package patterns passed to deadcode  |
| `--deadcode`   | (auto)                  | path to deadcode binary; auto-detects PATH or `go run` |
| `--beads-file` | `.ddx/beads.jsonl`      | beads ledger used to validate `wiring:pending`       |
| `--name`       | `$CHECK_NAME`           | basename for the result file                         |

## Escape hatch

```go
// wiring:pending ddx-1234abcd
//
// Will be called from cmd/migrate.go once the migration plan lands.
func MigrateOrders(ctx context.Context) error { /* ... */ }
```

The annotation is rejected (and itself becomes a violation) when:

- the named bead does not exist in `.ddx/beads.jsonl`, or
- the bead's status is closed (only `open`, `in_progress`, `ready` accepted).

## Limitations

- Only `FuncDecl`s are evaluated (functions and methods). New types and
  exported variables are not checked because RTA reachability is defined
  over functions; type-only changes are caught by the Go compiler when
  unused.
- The configured entry packages must contain at least one `main` package
  for deadcode to start its RTA traversal.
- Generated files are deferred to deadcode's own `Generated` flag.

# Decisions Log — fixture_repo.go Residual Cluster

## Bead: ddx-17c54930

### Symbols from production-reachability-final.json

| Symbol | Line | Decision | Rationale |
|---|---|---|---|
| `NewFixtureRepo` | 14 | **DELETE** | Test-only helper (`*testing.T` receiver) with zero callers in the current tree. The sole integration test (`probe_integration_test.go`) rolled its own `buildFixtureRepo` and no longer imports `testutils`. |
| `fixtureRepoScriptPath` | 39 | **DELETE** | Internal helper of `NewFixtureRepo`; no independent callers. Removed together with parent. |
| `buildDDXBinary` | 50 | **DELETE** | Internal helper of `NewFixtureRepo`; no independent callers. Removed together with parent. |

### Files Changed

- **`cli/internal/testutils/fixture_repo.go`** — deleted (entire file was test scaffolding stranded in a non-test file).
- **`cli/internal/testutils/`** — directory removed (empty after file deletion).

### Verification

1. `deadcode` returns no hits for `internal/testutils/fixture_repo.go`.
2. `go test ./...` passes (all packages green).
3. No `_test.go` files remained referencing the deleted package.

### Notes

The shell script `scripts/build-fixture-repo.sh` is retained because it is still called directly by `probe_integration_test.go` via its own inline `buildFixtureRepo` function.

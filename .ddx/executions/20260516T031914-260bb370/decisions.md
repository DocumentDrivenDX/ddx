# Decisions — residual production-reachability: internal/docprose

| Symbol | File | Line (per artifact) | Decision | Rationale |
|---|---|---|---|---|
| `countWords` | `cli/internal/docprose/checker.go` | 360 | DELETE | Dead code: never called from production or test code. The prior `unsupportedClaim` function used to call `countWords(line) <= 8` as a heuristic, but that logic was removed in a prior refactor. |
| `TempValeConfig.StylesPath` | `cli/internal/docprose/valeconfig.go` | 22 | DELETE | Dead code: the method was never called from production code. `vale_runner.go` uses `NewTempValeConfig` and accesses `INIPath()` and `Cleanup()` but never `StylesPath()`. Tests referenced it and were updated to compute the path directly via `filepath.Join(filepath.Dir(cfg.INIPath()), "styles")`. |

## Verification

- `cd cli && go test ./internal/docprose/...` — passes
- `cd cli && go test ./...` — passes (all packages)
- `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/docprose/(checker|valeconfig)\.go'` — returns no hits
- `lefthook run pre-commit` — passes

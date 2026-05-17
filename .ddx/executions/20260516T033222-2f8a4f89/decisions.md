# Decisions: ddx-c6b07648 — internal/registry installer residual

Source artifact: `.ddx/executions/20260515T210515-1a20052a/production-reachability-final.json`

| Symbol | Position | Decision | Rationale |
|---|---|---|---|
| `InstallPackageFromDir` | `cli/internal/registry/installer.go:65` | DELETE | Zero callers in `cli/` (production or tests). The local install path (`cli/cmd/install.go:installLocal`) uses its own symlink-based overlay, not the shared core install. The remaining trio entrypoints (`InstallPackageFromRemote`, `InstallPackageFromFS`) and the `InstallPackage` shim continue to cover the production paths. Removing the function leaves no unreachable shim behind. |

Verification: `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/registry/installer\.go'` returns no hits.

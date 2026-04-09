---
ddx:
  id: ddx-concerns
  depends_on:
    - helix.prd
---
# Project Concerns

## Active Concerns

- go-std (tech-stack, cli/api)
- typescript-bun (tech-stack, ui)
- e2e-playwright (testing, ui/site)
- hugo-hextra (microsite, site)
- demo-asciinema (demo, all)
- security-owasp (security, all)
- testing (quality-attribute, all)

## Area Labels

| Label | Applies to |
|-------|-----------|
| `all` | Every bead |
| `cli` | Core DDx binary, commands, internal packages |
| `api` | Server HTTP/MCP endpoints |
| `ui` | Web UI frontend (React SPA) |
| `site` | website/, microsite content and deployment |
| `infra` | CI, Docker, release infrastructure |

## Project Overrides

### go-std

- **Source**: Go source is in `cli/` within this monorepo
- **CLI framework**: Cobra with CommandFactory pattern
- **Testing**: `go test` with acceptance, integration, contract, and E2E smoke tests
- **Linter**: golangci-lint v2 with project `.golangci.yml`
- **Build**: `make build` in `cli/`, multi-platform via `make build-all`
- **Pre-commit**: lefthook runs `go fmt`, `go vet`, `golangci-lint`, `go test`

### typescript-bun

- **Build tool**: Vite (not Bun's native bundler) per ADR-002. Vite provides
  React Fast Refresh and the plugin ecosystem for the embedded SPA.
- **Test runner**: Playwright for E2E (not `bun:test`). Tests need the Go
  server running.
- **Formatter**: Project uses existing Vite/TypeScript conventions. Biome
  migration deferred.
- **Package manager**: Bun for install and scripts (`bun install`, `bun run build`).
- **Lockfile**: `bun.lock` committed (not `package-lock.json`).
- **Runtime**: Frontend is browser-only (React SPA embedded in Go binary via
  `embed.FS`). Bun-native server APIs (`Bun.serve()`) do not apply.
- **Data layer**: SQLite-WASM (sql.js) for client-side bead queries per ADR-005.
  TanStack Query manages fetch lifecycle.
- **Location**: `cli/internal/server/frontend/`

### e2e-playwright

Two Playwright configurations:

1. **Visual regression** (`playwright.config.ts`):
   - Tests in `cli/internal/server/frontend/e2e/`
   - Screenshots in `e2e/*.spec.ts-snapshots/`
   - Runs against Go server on port 18080

2. **Demo recording** (`playwright.demo.config.ts`):
   - `demo-recording.spec.ts` navigates all 5 pages with interactions
   - Video recording at 1280x720
   - Output in `demo-output/`
   - Run via `bun run demo:record`

3. **Microsite** (`website/playwright.config.ts`):
   - Tests in `website/e2e/microsite.spec.ts`
   - Screenshots for homepage, docs, navigation

### hugo-hextra

- **Theme version**: Hextra pinned in `website/go.mod`
- **Deployment**: GitHub Pages at `DocumentDrivenDX.github.io/ddx/`
- **Custom shortcode**: `asciinema.html` for terminal recording embeds
- **E2E tests**: Playwright tests in `website/e2e/`

### demo-asciinema

- **Recording environment**: Docker containers for reproducibility. All
  recording tools (asciinema, agg) are containerized.
- **Demo scripts**: `scripts/demos/` with deterministic shell scripts
- **Embedding**: asciinema shortcode loads player from CDN, plays `.cast`
  files from `website/static/demos/`
- **Agent credentials**: Demos invoking AI agents mount `~/.claude.json` (ro)
  and `~/.claude/` (rw) from host into container.

### security-owasp

- **Dependency audit**: `govulncheck ./...` for Go, `bun audit` for frontend
- **Secret scanning**: lefthook pre-commit hook via gitleaks
- **Path traversal**: Server validates all document paths, rejects `..`
- **Localhost guard**: All write and dispatch endpoints reject non-localhost requests
- **Network auth**: ts-net (Tailscale) for authenticated remote access per ADR-006.
  No custom API keys. Identity from tailnet membership.

### testing

- **Go tests**: Acceptance, integration, contract, E2E smoke in `cli/cmd/`
  and `cli/internal/*/`
- **Frontend tests**: Playwright E2E against running server
- **Pre-commit**: lefthook runs full Go test suite on changed packages
- **CI**: GitHub Actions runs `lefthook run ci` + E2E smoke tests

## ADR References

| Concern | ADR | Decision |
|---------|-----|----------|
| typescript-bun | ADR-002 | Web stack: Bun + Vite + React + Tailwind |
| typescript-bun | ADR-005 | SQLite-WASM client-side data layer for beads UI |
| security-owasp | ADR-003 | Package integrity via commit SHA + tree hash |
| security-owasp | ADR-004 | Bead-backed collections for runtime storage |
| security-owasp | ADR-006 | ts-net for network authentication (no API keys) |

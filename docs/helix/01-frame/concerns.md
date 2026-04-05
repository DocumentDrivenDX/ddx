# Project Concerns

## Active Concerns
- go-std (tech-stack, core CLI)
- hugo-hextra (microsite)
- demo-asciinema (demo)

## Area Labels

| Label | Applies to |
|-------|-----------|
| `all` | Every bead |
| `cli` | Core DDx binary, commands, plugin system |
| `site` | website/, microsite content and deployment |
| `demo` | Demo scripts and recordings |

## Project Overrides

### go-std
- **Source**: Go source is in a separate build repo; this checkout has the compiled binary
- **Binary distribution**: `ddx` ELF binary committed to repo, `install.sh` for setup
- **Testing**: integration tests via shell scripts, not `go test`

### hugo-hextra
- **Theme version**: Hextra — pinned in `website/go.mod`
- **Deployment**: GitHub Pages at `DocumentDrivenDX.github.io/ddx/`
- **E2E tests**: Playwright tests in `website/e2e/microsite.spec.ts` with screenshot snapshots
- **Custom shortcode**: `asciinema.html` for terminal recording embeds
- **Website package.json**: contains only Playwright dev dependency for e2e tests

### demo-asciinema
- **Embedding**: asciinema shortcode loads player from CDN, plays `.cast` files from `static/demos/`
- **Cast files**: stored in `website/static/demos/`

# Project Concerns

## Active Concerns
- hugo-hextra (microsite)
- demo-asciinema (demo)
- e2e-playwright (testing)

## Area Labels

| Label | Applies to |
|-------|-----------|
| `all` | Every bead |
| `cli` | scripts/helix, CLI wrapper |
| `workflow` | workflows/actions, workflows/concerns, workflow engine |
| `site` | website/, microsite content and deployment |
| `demo` | docs/demos/, demo scripts and recordings |

## Project Overrides

### hugo-hextra
- **Theme version**: Hextra v0.12.1 — pinned in `website/go.mod`
- **Hugo version**: 0.159.2 extended — pinned in `.github/workflows/pages.yml`
- **Deployment**: GitHub Pages at `easel.github.io/helix/`
- **Custom shortcode**: `asciinema.html` for terminal recording embeds

### demo-asciinema
- **Current demos**: `helix-quickstart` (full lifecycle), `helix-concerns` (drift detection), `helix-evolve` (requirement threading)
- **Agent harness**: `ddx agent run --harness claude` — not `claude -p`
- **Recording container**: Ubuntu 24.04 with project-specific deps (Node.js or Bun)
- **Cast files**: archived in `docs/demos/*/recordings/` and copied to `website/static/demos/`

### e2e-playwright
- **Test location**: `website/e2e/microsite.spec.ts`
- **Config**: `website/playwright.config.ts` with video on, trace on failure
- **Dev server**: Hugo server on port 1313, started automatically by Playwright
- **Test data**: site content files are the test data — all pages, shortcodes, and navigation patterns are exercised
- **Coverage**: every page loads, every glossary sub-page verified, navigation workflows tested, screenshots for visual regression
- **Videos**: generated for every test in `website/test-results/`

# Project Concerns

## Active Concerns
- hugo-hextra (microsite)
- product-microsite-ia (microsite)
- demo-asciinema (demo)
- e2e-playwright (testing)

## Area Labels

| Label | Applies to |
|-------|-----------|
| `all` | Every bead |
| `cli` | scripts/helix, CLI wrapper |
| `workflow` | workflows/actions, workflows/concerns, workflow engine |
| `docs` | docs/, plans, reports, and user-facing workflow documentation |
| `site` | website/, microsite content and deployment |
| `demo` | docs/demos/, demo scripts and recordings |
| `testing` | tests/, deterministic harnesses, and verification-only slices |
| `artifacts` | workflows/phases/*/artifacts metadata, prompts, and templates |

Use combined labels when the work spans more than one concern surface.
Examples:
- `area:workflow,area:docs` for workflow-contract doc updates
- `area:site,area:testing` for Playwright coverage on the microsite
- `area:workflow,area:artifacts` for artifact-definition changes

## Project Overrides

### hugo-hextra
- **Theme version**: Hextra v0.12.1 — pinned in `website/go.mod`
- **Hugo version**: 0.159.2 extended — pinned in `.github/workflows/pages.yml`
- **Deployment**: GitHub Pages at `documentdrivendx.github.io/helix/`
- **Custom shortcode**: `asciinema.html` for terminal recording embeds

### product-microsite-ia
- **Reader modes**: evaluators, new users, active HELIX users, and contributors
- **Top-level sections**: Why explains the thesis, Use gets a user productive,
  Types explains methodology objects, Reference provides exact lookup, and
  Artifacts exposes HELIX's own governing documents
- **Artifact Types navigation**: activity order is the primary ordering model;
  core artifacts are visible immediately within each activity; supporting
  artifacts stay in activity context with lighter treatment or inline
  disclosure
- **Navigation contract**: left navigation shows site hierarchy and active
  location; right navigation shows headings for the current page only
- **Proof path**: public examples, source artifacts, and deterministic checks
  must support product claims about shared memory and trustworthy context

### demo-asciinema
- **Current demos**: `helix-quickstart` (full lifecycle), `helix-concerns` (drift detection), `helix-evolve` (requirement threading), `helix-experiment` (metric-driven optimization)
- **Experimental demos**: `helix-interactive` is kept in `docs/demos/` for internal exploration and manual recording workflows; it is not part of the shipped public microsite or Pages recording inventory
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

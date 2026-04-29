# Practices: hugo-hextra

## Requirements (Frame phase)
- Define the site's target audience and what content they need
- List the documentation sections required (at minimum: getting started, concepts, CLI reference)
- Determine whether demo recordings are needed (see `demo-asciinema` concern)
- Decide on deployment target (default: GitHub Pages at `<org>.github.io/<project>`)

## Design
- Site lives in `website/` subdirectory — not the repo root
- Use Hugo Module system for Hextra theme (`go.mod` + `go.sum`)
- Navigation structure defined in `hugo.yaml` `menu.main` section
- Content hierarchy mirrors sidebar navigation — `_index.md` files create sections
- Prefer Hextra shortcodes over custom HTML for maintainability
- Static assets (images, recordings, downloads) go in `website/static/`
- Custom shortcodes only when Hextra has no equivalent

## Implementation
- Initialize with `hugo new site website` then add `go.mod` with Hextra import
- Pin Hextra version in `go.mod` — update deliberately, not automatically
- Pin Hugo version in CI — use the same version locally and in GitHub Actions
- Every content page needs YAML frontmatter with at minimum `title` and `weight`
- Section index pages (`_index.md`) use `cards` shortcode to link child pages
- Home page uses `layout: hextra-home` with hero and feature-grid shortcodes
- Menu items use `pageRef` for internal links, `url` for external (GitHub icon)
- Set `params.editURL.base` to enable "Edit this page" links
- Keep content readable as plain markdown — someone reading on GitHub should
  understand the docs even without Hextra rendering

## Content Authoring
- Write for the getting-started reader first — assume no prior knowledge of the project
- Getting Started page must get a user from zero to working in under 5 minutes
- CLI Reference must document every command with examples
- Use `tabs` shortcode for platform-specific instructions (macOS/Linux/Docker)
- Use `cards` shortcode for index pages that link to child content
- Embed terminal recordings with `asciinema` shortcode (see `demo-asciinema` concern)
- Include version badge on homepage hero using `hextra/hero-badge`
- Feature cards should each describe one capability with a concrete benefit

## Artifact-to-Docs Sync

The microsite must reflect the current state of the project's governing
artifacts. When artifacts are created, renamed, removed, or materially
changed, the corresponding microsite pages must be updated as part of the
same evolution or build pass.

### What triggers a docs update

Any change to a HELIX artifact that is surfaced on the microsite:
- New or renamed CLI commands → update CLI Reference
- New, renamed, or removed features → update glossary/artifacts page
- Changed phases, authority order, or workflow rules → update workflow page
- New or changed artifact types → update glossary/artifacts with description
  from `.ddx/plugins/helix/workflows/phases/*/artifacts/<name>/meta.yml` (description field)
  and `.ddx/plugins/helix/workflows/phases/*/artifacts/<name>/prompt.md` (Purpose section)
- New or changed concerns → update glossary/concerns page
- Changed install process → update Getting Started
- New demo reels → update Demos page and copy cast/video files to
  `website/static/demos/`

### Glossary generation from artifact metadata

Each HELIX artifact type has structured metadata at
`.ddx/plugins/helix/workflows/phases/<NN>-<phase>/artifacts/<name>/`:

| File | What it provides |
|------|-----------------|
| `meta.yml` | Name, description, dependencies, validation rules, relationships |
| `prompt.md` | Purpose section — 2-3 paragraphs explaining what the artifact is, why it matters, and how it fits the authority order |
| `template.md` | The structure to fill in |
| `example.md` | A concrete example |

The glossary artifacts page (`website/content/docs/glossary/artifacts.md`)
must include a substantive description for each artifact — not just a one-line
purpose. Pull the description from `meta.yml` and the Purpose section of
`prompt.md`. The reader should understand what each artifact is, when to
create one, and how it relates to artifacts above and below it in the
authority order.

### When evolve or frame changes artifacts

When `helix evolve` or `helix frame` creates or modifies a governing artifact
that is documented on the microsite, the agent must also update the
corresponding microsite page. This is not a separate task — it is part of
completing the evolution. The concern makes this a requirement, not a
suggestion.

Specifically:
- If a new artifact type is added to `.ddx/plugins/helix/workflows/phases/`, add it to the
  glossary artifacts page with its description from `meta.yml`/`prompt.md`
- If an artifact's purpose or scope changes, update the glossary entry
- If a CLI command is added or its behavior changes, update the CLI
  reference page
- If installation steps change, update Getting Started

## Testing
- Playwright e2e tests in `website/e2e/` verify page loads and navigation
- Screenshot snapshot tests catch visual regressions after theme updates
- Test every section landing page and the homepage
- Run `hugo --gc --minify` locally before pushing to catch build errors
- After Hextra version bumps, run screenshot update and review diffs

## Quality Gates
- `hugo --gc --minify` builds without errors or warnings
- `playwright test` passes (if e2e tests exist)
- No broken internal links (Hugo reports these as build warnings)
- Content files have valid YAML frontmatter

## Deployment
- GitHub Actions deploys on successful test + push to main
- Hugo build uses `--gc --minify` with `HUGO_ENVIRONMENT=production`
- Checkout must use `fetch-depth: 0` for git-based last-modified dates
- Cache Hugo modules in CI for faster builds
- Enable concurrent deployment protection to prevent race conditions

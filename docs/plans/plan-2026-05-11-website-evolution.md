---
ddx:
  id: plan-2026-05-11-website-evolution
---
# Website Evolution: Design System, Auto-Generation, Hugo Renderer, GitHub Pages

Date: 2026-05-11
Status: Reviewed — direction locked for all four pieces. Detailed auto-generation + templating architecture moved to companion plan `plan-2026-05-11-website-autogen.md`. This document is now the high-level summary; bead breakdown follows the companion plan's sequencing.

## Locked decisions (2026-05-11)

1. **Hextra removed.** Replace with custom layouts (Option A from Piece 1).
2. **Templates share design tokens via a single CSS file imported by one `baseof.html` chrome.** Tokens never duplicate across layouts. See companion plan §"Templating architecture."
3. **Auto-generation is the spine.** No hand-authoring, no LLM, no staleness. Deterministic generators produce Hugo data files in CI; generated artifacts are not version-controlled. See companion plan for the full surface and generator architecture.
4. **Publishing stays at `DocumentDrivenDX.github.io/ddx/`** (subdirectory). No org-pages migration.

## Purpose

DDx has a working promotional website (`website/`) built with a custom Hugo
layout for the homepage and Hextra theme for inner pages. The user has
described a desired future state: a new design system, scripts to
autogenerate site components from documents and source code, and a Hugo
renderer that publishes to GitHub Pages. This plan decomposes that into four
pieces, surveys current state, and recommends sequencing.

---

## Piece 1: Design System

### Current state

- `website/DESIGN.md` — design tokens, palette, typography, and section map
  (`website/DESIGN.md:1-115`)
- `website/assets/css/custom.css` — sole CSS file; homepage editorial layout
  (`website/layouts/index.html`) and the Hextra inner pages use different
  design stacks
- **Homepage**: fully custom, standalone — `layouts/index.html` inlines all
  styles via `custom.css`. Palette: lever/load/fulcrum (steel blue + warm
  brass). Fonts: Inter (headlines), Space Grotesk (UI), Newsreader (prose).
- **Inner pages (docs, features, etc.)**: Hextra theme with Tailwind-based
  `hx:` utility classes. Hextra components are configured via
  `website/hugo.yaml` (params, nav menus). There is no shared CSS bridge
  between the custom homepage and Hextra inner pages — they are visually
  disconnected.
- Brand assets in `website/static/visuals/` (six images: principle-composite,
  tool-*.png/jpg, user-workflow.png). A hero image is blocked (`website/static/hero/HERO_BLOCKER.md`).

### Desired state

A coherent design system used by both the homepage and inner pages: shared
tokens, shared component patterns, consistent typography and color across all
routes.

### Gap

The homepage and Hextra inner pages share no CSS. Hextra's Tailwind utilities
are `hx:` prefixed and cannot be applied to `custom.css` markup. Inner pages
use Hextra's default palette, not the lever/load/fulcrum tokens. Hextra is
either replaced, customized at the theme level, or the inner pages get a
custom layout that imports the design tokens.

### How to close

**Option A (preferred)**: Replace Hextra inner pages with a single Hugo base
layout (extend `layouts/_default/baseof.html`) that imports `custom.css` and
shares the homepage design tokens. Inner page typography uses Newsreader
(prose) + Space Grotesk (UI), matching the homepage. Navigation is the custom
`ddx-home-nav`. Hextra is removed as a Hugo module.

**Option B**: Extend Hextra's CSS overrides. Hextra supports
`assets/css/extended/` for user CSS injections. Add token variables and
override Tailwind color scales. Feasible but fragile — Hextra upgrades can
break overrides, and the Tailwind `hx:` prefix remains in markup, limiting
design uniformity.

Option A produces a cleaner long-term codebase and is the right path for a
project that intends to own its design system.

### Open questions

- Should the design system use a CSS token file separate from `custom.css` so
  it can be imported by multiple layouts without duplication?
- Is Newsreader appropriate for technical docs (it is a serif face)? The
  current inner pages use Hextra's sans-serif stack.
- Are the existing `website/static/visuals/` images consistent with the target
  design, or should they be regenerated?

### Bead-sized vs FEAT-spec

A replacement of Hextra inner-page layouts with custom layouts is large enough
to warrant a FEAT spec update (FEAT-003-website.md already exists). The CSS
token unification is bead-sized (one bead: add a shared token layer to
`custom.css` and extend inner layouts).

---

## Piece 2: Auto-Generate Site Components from Source Documents + Source Code

### Current state

All site content is hand-authored markdown. The only semi-automated content is:

- `website/content/docs/cli/_index.md` — marked "hand-curated; do not
  auto-regenerate" (`website/content/docs/cli/_index.md:4`); an older bead
  intended per-command generation under `/docs/cli/commands/` but it is not
  implemented
- `website/.github/workflows/demos.yml` — demo cast regeneration (exists in
  `.github/workflows/demos.yml`); regenerates `.cast` files when CLI changes
- `website/data/release.yaml` — injected by the deploy workflow at build time
  for version display

No persona showcase, no skill catalog, no library prompts index, no bead
metrics/status pages, and no CLI command reference are currently generated.

### What source documents and code are realistic auto-gen targets

| Source | Target section | Generation mechanism |
|--------|---------------|---------------------|
| `cli/cmd/*.go` Cobra help strings | `/docs/cli/commands/` | `ddx __introspect` (proposed in `plan-2026-05-10-pip-distribution-and-python-api.md`) or `cobra-docs` markdown generation from the Cobra command tree |
| `library/personas/*.md` | `/docs/personas/` | Hugo data template reading YAML frontmatter; each persona becomes a data file |
| `library/prompts/` | `/docs/library/prompts/` | Hugo content generation script; directory listing with descriptions |
| `docs/helix/01-frame/features/FEAT-*.md` | Not public yet | Public feature pages are in scope for FEAT-003 but not implemented |
| Bead metrics / status | `/status/` | Requires ddx-server MCP hook; not a static-site concern yet |
| Skill manifests (`.agents/skills/*/SKILL.md`) | `/docs/skills/` | Hugo data template from YAML frontmatter; the existing `/docs/skills/` page is hand-written |
| Plugin catalog (`cli/internal/registry/registry.go`) | `/docs/plugins/` | Go codegen or `ddx search` JSON output piped to Hugo `data/` |

The most immediately valuable and feasible targets:

1. **CLI command reference from Cobra** — high value (keeps docs in sync with
   code), well-understood pattern. Cobra ships `docs.GenMarkdownTree` which
   writes per-command markdown files. A small Go program in
   `cli/tools/gen-docs/` can call this and write to `website/content/docs/cli/commands/`. The deploy workflow can run it as a pre-build step.

2. **Skills catalog from SKILL.md frontmatter** — medium value. A short script
   (bash or Go) reads YAML frontmatter from `.agents/skills/*/SKILL.md` and
   writes a Hugo data file (`website/data/skills.yaml`), which the skills page
   template renders. This replaces the hand-written skills table.

3. **Plugin catalog from the Go registry** — medium value. A `go run` tool
   reads `cli/internal/registry/registry.go` and writes
   `website/data/plugins.yaml`. The plugins page uses this data.

4. **Persona showcase from library** — low priority. Personas are not yet
   public-facing enough to merit a dedicated page, but a data-file approach
   identical to skills would work.

### Gap

None of these generation scripts exist. The deploy workflow at
`.github/workflows/pages.yml` has a step for Hugo build but no pre-build
codegen step.

### How to close

1. Add `cli/tools/gen-docs/main.go` — Cobra `GenMarkdownTree` → `website/content/docs/cli/commands/`
2. Add `scripts/gen-website-data.sh` — reads library/skills and registry, writes Hugo data files
3. Modify `.github/workflows/pages.yml` — add a pre-build step `go run ./tools/gen-docs ./website/content/docs/cli/commands` and `bash scripts/gen-website-data.sh`
4. Update `website/content/docs/skills.md` and `website/content/docs/plugins.md` to use Hugo data templates instead of hand-authored content

### Open questions

- The `ddx __introspect` command from `plan-2026-05-10-pip-distribution-and-python-api.md` is not yet implemented. Should CLI doc generation use Cobra's built-in `GenMarkdownTree` instead (simpler, available now)?
- Should the generated CLI docs live under `website/content/docs/cli/commands/` (checked in) or be generated at build time only (not checked in)? Generated-at-build is cleaner but requires the gen tool to run in CI.
- How stale is acceptable? The current hand-curated CLI reference drifts from the actual CLI. Generated-at-build is the right answer; the current hand-curated approach is a maintenance liability.

### Bead-sized vs FEAT-spec

- Cobra markdown generation tool: one bead (additive, self-contained)
- Skills/plugin data generation script: one bead
- CI integration of codegen step: one bead
- These are all bead-sized, not FEAT-spec scope. They slot under the existing FEAT-003-website.md.

---

## Piece 3: Hugo Renderer

### Current state

The Hugo site structure is:

```
website/
  hugo.yaml                   # site config, Hextra module
  layouts/
    index.html                # custom homepage layout (standalone, no Hextra)
    shortcodes/
      maturity.html           # maturity badge shortcode
      asciinema.html          # asciinema player shortcode
  assets/css/
    custom.css                # homepage CSS (not loaded by Hextra inner pages)
  content/
    _index.md                 # homepage (content ignored; layout/index.html drives it)
    docs/                     # documentation pages (Hextra themed)
    features/                 # features page (Hextra themed)
    why/                      # why page (Hextra themed)
  static/
    demos/                    # .cast and .gif demo assets
    visuals/                  # principle-composite, tool-*.png/jpg
    hero/                     # hero image placeholder
    ui/                       # feature screenshot placeholders
```

Two separate rendering stacks:
- Homepage: `layouts/index.html` (self-contained, custom CSS, design system applied)
- Inner pages: Hextra theme (Tailwind `hx:` classes, Hextra components)

### Desired state

A coherent Hugo renderer where:
- All pages share the design system from Piece 1
- Auto-generated content from Piece 2 is included in the build
- Navigation reflects the full site structure
- Search is available across all pages
- Theming (dark/light mode) works uniformly

### Gap

Currently:
- The homepage does not use Hextra's search or navigation components
- Inner pages do not use the design system tokens or custom fonts
- There is no Hugo partial for shared navigation across both stacks
- The `why/` section exists (`website/content/why/`) but the homepage does not link to it as a "Why DDx" nav item (it links to Features, Docs, Concepts)

### How to close

If Option A (custom base layout) is chosen for Piece 1:
1. Create `layouts/_default/baseof.html` with shared nav, footer, and CSS import
2. Create `layouts/_default/single.html` and `layouts/_default/list.html` using the base layout
3. Remove `hugo.yaml` module import for Hextra; remove the `hx:` class references from content files (or leave them as inert attributes)
4. Port Hextra shortcodes used in content (`hextra/hero-badge`, `hextra/feature-card`, etc.) to custom equivalents or remove them
5. Add search: either a client-side Fuse.js index on the Hugo JSON output, or pagefind (pagefind.app), which works entirely at build time

The `content/_index.md` can either remain as an unused content file (the custom layout ignores it) or be simplified to a stub.

### Open questions

- Does the user want to keep Hextra for inner pages (simpler) and only unify at the CSS token level, or do a full replacement? Option B from Piece 1 is the "keep Hextra" path.
- If Hextra is removed, which shortcodes are used in content files that would need replacement? Survey: `asciinema`, `maturity`, `hextra/hero-badge`, `hextra/hero-headline`, `hextra/hero-subtitle`, `hextra/hero-button`, `hextra/feature-grid`, `hextra/feature-card` are used in `content/_index.md` (but the homepage ignores content). Inner pages use `asciinema` and `maturity` — both are custom shortcodes already. So removing Hextra may have near-zero content impact if the homepage content file is a stub.
- Should the site have a sidebar navigation for docs, or top-navigation only?

### Bead-sized vs FEAT-spec

A full Hextra replacement is feature-scope (update FEAT-003-website.md). A
CSS token unification without Hextra removal is bead-sized (one bead). The
search addition is bead-sized.

---

## Piece 4: Publishing to GitHub Pages

### Current state

The deploy workflow (`.github/workflows/pages.yml`) deploys from
`DocumentDrivenDX/ddx` to GitHub Pages. The `hugo.yaml` base URL is
`https://DocumentDrivenDX.github.io/ddx/` (`website/hugo.yaml:1`).

The workflow:
1. Triggers on `workflow_run` (after CI passes) or `push` to tags
2. Builds with Hugo to `website/public/`
3. Runs Playwright e2e tests against a second build at `website/public-test/`
4. Uploads `website/public/` as a Pages artifact
5. Deploys via `actions/deploy-pages@v4`

The site is served from the `gh-pages` environment of the same repo (not a
separate `ddx.github.io` repo). The subdirectory `/ddx/` in the base URL
indicates this repo deploys to `<org>.github.io/ddx/` not to
`<org>.github.io` root.

### Desired state (open question)

The user mentions "publishing to github pages" as a distinct piece. Two
plausible interpretations:

1. **Keep current setup** — ddx builds and deploys to `DocumentDrivenDX.github.io/ddx/`. No change needed to the publishing pipeline; only the build quality (from Pieces 1–3) changes.
2. **Separate `ddx.github.io` repo** — create a `DocumentDrivenDX/ddx.github.io` org pages repo and deploy the built site there. This would change the base URL to `https://DocumentDrivenDX.github.io/` (no subdirectory). Requires a workflow change to push to the external repo.

### Gap

If the intent is (1): no gap — the pipeline works (see Phase 1 fix). The
pre-build codegen steps from Piece 2 need to be added to `pages.yml`.

If the intent is (2): the deploy target changes. The workflow would need
`actions/deploy-pages` replaced with a `git push` to `ddx.github.io`. The
`hugo.yaml` `baseURL` would change from `.../ddx/` to `https://DocumentDrivenDX.github.io/`.

### Open questions

- Does the user intend to deploy to a separate `ddx.github.io` repo (separate
  Pages domain, different from the current subdirectory path)?
- Is the current subdirectory URL (`...github.io/ddx/`) acceptable long-term,
  or should the org get a root Pages site?

### Bead-sized vs FEAT-spec

- Adding codegen pre-build steps to `pages.yml`: bead-sized
- Moving to a separate `ddx.github.io` repo: bead-sized (workflow change + `hugo.yaml` baseURL update)

---

## Recommended Sequencing

### Phase A: Foundation (now, bead-sized work)

These are independent, high-value, low-risk and can be done in any order:

1. **CLI command reference generation** — add `cli/tools/gen-docs/` using
   Cobra `GenMarkdownTree`, wire into `pages.yml`. Closes the CLI docs drift
   problem. No design changes required.

2. **Skills and plugin data generation** — add `scripts/gen-website-data.sh`,
   update skills and plugins pages to use Hugo data templates.

3. **Codegen pre-build step in `pages.yml`** — wire codegen into the deploy
   workflow (depends on #1 and #2).

### Phase B: Design System (next, FEAT-003 update needed)

4. **CSS token unification (Option B, keep Hextra)** — add token variables to
   `assets/css/extended/` so inner pages inherit lever/load/fulcrum palette.
   Safer, smaller scope. Evaluate whether full Hextra removal is warranted
   after seeing the result.

5. **Full custom base layout (Option A)** — if Option B is unsatisfying,
   replace Hextra inner layouts with a custom `baseof.html`. This requires a
   FEAT-003 update before beads are filed.

### Phase C: Publishing clarification (needs user decision)

6. **Decide on publishing target** — answer the open question (subdirectory
   vs separate org Pages site). This is a one-bead change once decided.

### What Phase A unlocks

Phase A can be filed as beads immediately (no FEAT update needed — they are
additive improvements under FEAT-003). The CLI docs bead in particular is high
value and fully self-contained.

Phase B requires a FEAT-003 amendment documenting the design direction (Hextra
replacement or override). File the amendment before breaking down Phase B
beads.

---

## What is Not in Scope Here

- Interactive document browser requiring `ddx-server` (FEAT-008, FEAT-021)
- Blog or news feed
- i18n
- Bead metrics/status live pages (needs server hook, not a static-site problem)
- Hero image generation (blocked on `OPENROUTER_API_KEY`; tracked in `website/static/hero/HERO_BLOCKER.md`)

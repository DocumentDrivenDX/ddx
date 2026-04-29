# Concern: Microsite (Hugo + Hextra)

## Category
microsite

## Areas
all

## Components

- **Static site generator**: Hugo (extended edition)
- **Theme**: Hextra (`github.com/imfing/hextra`), imported as a Hugo Module
- **Content format**: Markdown with YAML frontmatter
- **Deployment**: GitHub Pages via GitHub Actions
- **E2E testing**: Playwright with screenshot snapshots

## Constraints

- All site content lives under `website/` in the project root
- Hugo Module system for theme management — not git submodules, not manual copy
- Hextra theme provides layout, navigation, search, and responsive design — do
  not duplicate what the theme already does
- `go.mod` and `go.sum` in `website/` pin the theme version
- No custom CSS unless Hextra genuinely cannot achieve the design goal
- Markdown content must render correctly without the theme (readable on GitHub)
- `enableGitInfo: true` — Hugo uses git metadata for last-modified dates;
  CI must checkout with `fetch-depth: 0`
- `goldmark.renderer.unsafe: true` — allows raw HTML in markdown for shortcodes
- Hugo version pinned in CI workflow — do not rely on ambient `hugo` from PATH
- When governing artifacts change (new features, renamed commands, changed
  workflow rules, new artifact types), the corresponding microsite pages must
  be updated in the same pass — not deferred to a separate task

## Content Guidelines

Every microsite must include at minimum:

| Page | Path | Purpose |
|------|------|---------|
| Home | `content/_index.md` | Hero, value prop, feature grid, CTA |
| Getting Started | `content/docs/getting-started.md` | Install, first-run, quickstart |
| Core Concepts | `content/docs/<topic>/_index.md` | One section per major concept |
| CLI Reference | `content/docs/cli/_index.md` | Complete command reference |
| Demo | `content/docs/demos/_index.md` | Embedded terminal recordings |

### Home page pattern

Use `layout: hextra-home` and Hextra shortcodes. **Spacing is critical** —
the hero section must breathe. Use Hextra's `hx-` Tailwind utilities for
vertical rhythm between hero elements:

```markdown
---
title: Project Name
layout: hextra-home
---

{{</* hextra/hero-badge link="https://github.com/org/repo" */>}}
  <span>Open Source</span>
  {{</* icon name="arrow-circle-right" attributes="height=14" */>}}
{{</* /hextra/hero-badge */>}}

<div class="hx-mt-6 hx-mb-6">
{{</* hextra/hero-headline */>}}
  Tagline line one.&nbsp;<br class="sm:hx-block hx-hidden" />Tagline line two.
{{</* /hextra/hero-headline */>}}
</div>

<div class="hx-mb-12">
{{</* hextra/hero-subtitle */>}}
  One-sentence description of the project.
{{</* /hextra/hero-subtitle */>}}
</div>

<div class="hx-mb-12">
{{</* hextra/hero-button text="Get Started" link="docs/getting-started" */>}}
{{</* hextra/hero-button text="Learn More" link="docs/concepts" style="alt" */>}}
</div>

<div class="hx-mt-8"></div>

{{</* hextra/feature-grid */>}}
  {{</* hextra/feature-card title="Feature" subtitle="Description" */>}}
{{</* /hextra/feature-grid */>}}

<div class="hx-mt-16"></div>

## Below-the-fold section
```

#### Landing page spacing rules

| Element | Class | Purpose |
|---------|-------|---------|
| Hero headline wrapper | `hx-mt-6 hx-mb-6` | Breathing room above and below headline |
| Hero subtitle wrapper | `hx-mb-12` | Generous gap before CTA buttons |
| CTA button wrapper | `hx-mb-12` | Prevent buttons from crowding the feature grid |
| Spacer before feature grid | `hx-mt-8` | Visual separation between hero and features |
| Spacer before below-fold section | `hx-mt-16` | Clear break before secondary content |

**Do not** use `hx-mb-6` between CTA buttons and the feature grid — it looks
cramped. Use `hx-mb-12` on the button wrapper plus a `hx-mt-8` spacer div.

#### Feature card pattern

Use `hx-aspect-auto md:hx-aspect-[1.1/1]` and `max-md:hx-min-h-[340px]` on
the top row of feature cards to ensure consistent height. Use radial gradient
backgrounds for visual distinction:

```
style="background: radial-gradient(ellipse at 50% 80%,rgba(72,120,198,0.15),hsla(0,0%,100%,0));"
```

### Documentation page pattern

```markdown
---
title: Page Title
weight: 1
prev: /docs
next: /docs/next-section
---
```

Use `weight` for sidebar ordering, `prev`/`next` for sequential navigation.

### Shortcode preference

Use Hextra built-in shortcodes before creating custom ones:

- `hextra/hero-*` — landing page components
- `hextra/feature-grid` / `hextra/feature-card` — feature showcase
- `cards` / `card` — documentation index grids
- `tabs` / `tab` — tabbed content blocks
- `icon` — inline SVG icons

Only create a custom shortcode in `layouts/shortcodes/` when no Hextra
component covers the need (e.g., `asciinema.html` for terminal recordings).

## Hugo Configuration Pattern

```yaml
baseURL: https://<org>.github.io/<project>/
languageCode: en-us
title: <PROJECT> — <tagline>

module:
  imports:
    - path: github.com/imfing/hextra

enableRobotsTXT: true
enableGitInfo: true

markup:
  goldmark:
    renderer:
      unsafe: true
  highlight:
    noClasses: false

params:
  navbar:
    displayTitle: true
    displayLogo: false
  page:
    width: wide
  footer:
    displayCopyright: true
    displayPoweredBy: false
  editURL:
    enable: true
    base: https://github.com/<org>/<project>/edit/main/website/content
```

## E2E Testing Pattern

Playwright tests live at `website/e2e/` with `website/package.json`:

```json
{
  "name": "<project>-website-tests",
  "private": true,
  "type": "module",
  "scripts": {
    "test:e2e": "playwright test",
    "test:screenshots": "playwright test --update-snapshots"
  },
  "devDependencies": {
    "@playwright/test": "^1.59.1"
  }
}
```

Tests should verify:
- Homepage loads with hero content and feature cards
- Each documentation page loads and has expected headings
- Navigation links work
- Screenshot snapshots for visual regression

## CI/CD Pattern

GitHub Actions workflow for GitHub Pages deployment:

1. Trigger: successful test workflow on main + version tags + manual dispatch
2. Install Hugo (extended, pinned version) and Go
3. Checkout with `fetch-depth: 0` (required for `enableGitInfo`)
4. Build: `hugo --gc --minify` in `website/` directory
5. Deploy to GitHub Pages

## Drift Signals (anti-patterns to reject in review)

- CLI command added or changed without updating CLI Reference page → update the docs
- New artifact type in `.ddx/plugins/helix/workflows/phases/` without a glossary entry → add it
- Feature spec created or evolved without updating the microsite → update it
- Install process changed without updating Getting Started → fix it
- Demo reel recorded but not copied to `website/static/demos/` → publish it
- `hx-mb-6` between CTA buttons and feature grid on landing page → use `hx-mb-12` + `hx-mt-8` spacer
- Custom CSS when Hextra utility classes would suffice → use `hx-` classes
- Missing `_index.md` in a content section → add it for proper sidebar nav
- Hardcoded URLs instead of Hugo `relref` or `ref` → use Hugo link functions

## When to use

Any project that needs a public-facing documentation site. The Hugo + Hextra
pattern provides search, responsive design, dark mode, and navigation with
minimal configuration.

## ADR References

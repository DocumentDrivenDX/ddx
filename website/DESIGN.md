# DDx Website Design System

Design tokens for the DDx microsite. All tokens are defined in
`assets/css/custom.css` and flow into the Hugo build via the pipeline:

```
resources.Get "css/custom.css" | minify | fingerprint
```

The fingerprinted stylesheet is linked in `layouts/index.html`:
```go-html-template
{{- $css := resources.Get "css/custom.css" | minify | fingerprint -}}
<link rel="stylesheet" href="{{ $css.RelPermalink }}">
```

---

## Palette — lever / load / fulcrum

The DDx color system uses a mechanical metaphor: **lever** (primary action),
**load** (supporting tint), **fulcrum** (anchor/dark), **brass** (warm accent).

| Token | Light | Dark | Role |
|---|---|---|---|
| `--ddx-accent-lever` | `#3B5B7A` | `#3B5B7A` | Primary links, CTAs, card numbers |
| `--ddx-accent-load` | `#5A85B0` | `#5A85B0` | Secondary card accent |
| `--ddx-accent-fulcrum` | `#2A3F56` | `#2A3F56` | Tertiary card accent, hover state |
| `--ddx-accent-brass` | `#9A6A2F` | `#C79B5B` | Kicker labels, quote border, action underline |

Hextra's primary tint scale is driven by:

```css
--primary-hue: 210deg;
--primary-saturation: 36%;
--primary-lightness: 35%;   /* 55% in dark mode for legibility */
```

This re-tints Hextra's link, focus-ring, and badge utilities to the lever color
without touching the theme source.

---

## Surface & Foreground

| Token | Light | Dark | Role |
|---|---|---|---|
| `--ddx-bg-canvas` | `#F7F1E7` | `#0F1117` | Page background |
| `--ddx-bg-surface` | `#FFFDF8` | `#1A1E2A` | Card/panel background |
| `--ddx-fg-ink` | `#17130F` | `#E8EAF0` | Body text |
| `--ddx-fg-muted` | `#675F54` | `#9CA3AF` | Secondary text, captions |
| `--ddx-border-line` | `#D9CFC0` | `#2D3340` | Dividers, card borders |

---

## Typography

Fonts are imported from Google Fonts in `custom.css`:

```css
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@700;800
  &family=Newsreader:ital,wght@0,400;0,600;1,400
  &family=Space+Grotesk:wght@400;500;600&display=swap');
```

| Face | Family | Weights | Usage |
|---|---|---|---|
| Headlines | Inter | 700, 800 | `h1` (hero title), section headlines, CTA |
| Body | Space Grotesk | 400, 500, 600 | Navigation, cards, labels, UI chrome |
| Prose | Newsreader | 400, 600, 400i | Lead paragraphs, card body copy, blockquotes |

---

## Homepage Section Map

The custom homepage template (`layouts/index.html`) renders seven sections,
each styled with a dedicated CSS block in `custom.css`:

| # | Selector | Section |
|---|---|---|
| 1 | `.ddx-home-hero` | Hero: headline, prose, CTA, pull-quote |
| 2 | `.ddx-home-problem` | Problem: the context gap AI agents cannot close |
| 3 | `.ddx-home-how` | How it works: three-step workflow |
| 4 | `.ddx-home-features` | Features preview: 4-up card grid → /features/ |
| 5 | `.ddx-home-principles` | Principles: 6-card lever/load/fulcrum grid |
| 6 | `.ddx-home-demo` | Demo: asciinema terminal recording |
| 7 | `.ddx-home-cta` | CTA: get started call-to-action |

---

## Maturity Badges

Feature maturity is communicated via the `maturity` shortcode
(`layouts/shortcodes/maturity.html`). Usage in content:

```markdown
### Bead Tracker {{</* maturity "stable" */>}}
### MCP Server {{</* maturity "beta" */>}}
### Remote Execution {{</* maturity "planned" */>}}
```

| Status | Light bg / fg | Dark bg / fg | Meaning |
|---|---|---|---|
| `stable` | `#d1fae5` / `#065f46` | `#064e3b` / `#6ee7b7` | Production-ready |
| `beta` | `#fef3c7` / `#92400e` | `#451a03` / `#fcd34d` | Available, API may change |
| `planned` | `#e5e7eb` / `#374151` | `#1f2937` / `#9ca3af` | On the roadmap |

---

## Responsive Breakpoints

| Breakpoint | Behaviour |
|---|---|
| `≤ 820px` | Hero collapses to single column; 3-col and 4-col grids go 1-col (4-col → 2-col first) |
| `≤ 520px` | Full-bleed layout; nav gap tightened; footer stacks vertically; 4-col → 1-col |

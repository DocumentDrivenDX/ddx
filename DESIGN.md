---
version: "alpha"
name: DDx
description: >
  Visual language for Document-Driven Development eXperience — a terminal-first developer
  tool whose three-color identity maps to the lever/load/fulcrum metaphor.
  Applied palette: Cold Steel, Aged Brass, Iron Gray on warm parchment surfaces.

colors:
  # --- Accent triple (lever/load/fulcrum) ---
  accent-lever:    "#3B5B7A"   # Cold Steel Blue — primary actions, active states, links
  accent-load:     "#A8801F"   # Aged Brass — in-progress, emphasis, warnings
  accent-fulcrum:  "#3F4147"   # Iron Gray — neutral structural, secondary actions

  # --- Light mode surfaces ---
  bg-canvas:       "#F4EFE6"   # Warm cream paper — page background
  bg-surface:      "#FBF8F2"   # Parchment — cards, sidebars, containers
  bg-elevated:     "#FFFFFF"   # Pure white — inputs, highest elevation

  # --- Light mode text ---
  fg-ink:          "#1F2125"   # Near-black ink — primary text
  fg-muted:        "#6B6558"   # Sepia muted — labels, placeholders, secondary text

  # --- Light mode borders ---
  border-line:     "#E4DDD0"   # Hairline tan — all dividers and borders

  # --- Dark mode surfaces ---
  dark-bg-canvas:  "#1A1815"   # Deep warm near-black — page background
  dark-bg-surface: "#26231F"   # Warm charcoal — cards, sidebars
  dark-bg-elevated:"#2E2A25"   # Lifted charcoal — highest elevation panels

  # --- Dark mode accents ---
  dark-accent-lever:   "#7BA3CC"   # Ice Steel Blue
  dark-accent-load:    "#D4A53D"   # Warm Brass
  dark-accent-fulcrum: "#9CA0A8"   # Neutral Silver

  # --- Dark mode text ---
  dark-fg-ink:     "#EDE6D6"   # Warm bone — primary text
  dark-fg-muted:   "#8E8674"   # Dusty — secondary text, labels

  # --- Dark mode borders ---
  dark-border-line: "#34302A"  # Ember hairline

  # --- Terminal (shared both modes) ---
  terminal-bg:      "#1F2125"
  terminal-bg-deep: "#0F0E0C"  # Void black — microsite dark terminal
  terminal-fg:      "#D8D2C4"

  # --- Semantic status ---
  status-open:       "#3B5B7A"  # maps to accent-lever
  status-in-progress:"#A8801F"  # maps to accent-load
  status-closed:     "#15803D"
  status-blocked:    "#B91C1C"
  status-parked:     "#6B6558"  # maps to fg-muted

  # --- Error ---
  error:            "#BA1A1A"
  dark-error:       "#FFB4AB"

typography:
  # Admin console (dense, Inter-only)
  headline-lg:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: 800
    lineHeight: 1.2
    letterSpacing: -0.02em
  headline-md:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: -0.01em
  body-md:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: -0.01em
  body-sm:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.4
    letterSpacing: -0.005em
  label-caps:
    fontFamily: Inter
    fontSize: 11px
    fontWeight: 700
    lineHeight: 1
    letterSpacing: 0.05em
  mono-code:
    fontFamily: "ui-monospace, SFMono-Regular, JetBrains Mono, monospace"
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.4
    letterSpacing: 0.01em

  # Microsite additional scales
  hero-tagline:
    fontFamily: Inter
    fontSize: 64px
    fontWeight: 800
    lineHeight: 1.0
    letterSpacing: -0.02em
  h1:
    fontFamily: Inter
    fontSize: 40px
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: -0.01em
  body-editorial:
    fontFamily: "Newsreader, Georgia, serif"
    fontSize: 18px
    fontWeight: 400
    lineHeight: 1.6
  mono-label:
    fontFamily: "Space Grotesk, Inter, sans-serif"
    fontSize: 14px
    fontWeight: 500
    lineHeight: 1.4

rounded:
  none:    0px
  sm:      2px
  DEFAULT: 4px
  lg:      8px
  full:    9999px

spacing:
  base:       4px
  xs:         4px
  sm:         8px
  gutter:     12px
  md:         16px
  lg:         24px
  xl:         32px
  section:    128px
  prose-max:  720px
  container-max: 1200px

components:
  button-primary:
    background: "{colors.accent-lever}"
    color: "{colors.bg-elevated}"
    borderRadius: "{rounded.none}"
  button-ghost:
    background: "transparent"
    borderColor: "{colors.border-line}"
    borderRadius: "{rounded.none}"
  feature-card:
    background: "{colors.bg-surface}"
    borderColor: "{colors.border-line}"
    borderRadius: "{rounded.none}"
    padding: 24px
  terminal-block:
    background: "{colors.terminal-bg-deep}"
    color: "{colors.terminal-fg}"
    borderRadius: "{rounded.DEFAULT}"
    padding: 16px
  nav-active:
    borderLeft: "2px solid {colors.accent-lever}"
    background: "{colors.bg-surface}"
  status-badge:
    borderRadius: "{rounded.none}"
    typography: "{typography.label-caps}"
---

## Overview

DDx is a terminal-first developer platform. Its visual language is **sober, utilitarian, and purposely non-decorative** — the antithesis of AI-gloss, dreamy illustration, and gradient excess. Every visual choice serves comprehension, not style.

The product's three-color identity maps directly to its core metaphor: the **lever, the load, and the fulcrum**. Abstraction is the lever — it transmits intent across levels of the stack. LLMs are the load — probabilistic, costly, not fully in control. Human-AI collaboration is the fulcrum — the stable pivot where judgment meets execution. These are not decorative categories; they are architectural facts about how DDx works, and the palette encodes them.

The design system spans two surfaces sharing one token vocabulary:

- **Admin Console** — high-density terminal dashboard. Maximum information per pixel. 0px border-radius everywhere, zero shadows, 4px grid, Inter-only.
- **Marketing Microsite** — a scholarly engineering monograph. Generous whitespace, 128px section breaks, Newsreader serif body copy, Space Grotesk mono-labels.

## Colors

The palette is three industrial accents over a warm-paper foundation. Nothing is neutral gray or cool-toned — every surface has warmth.

**Accent triple** — every interactive or state-bearing element maps to one of three roles:

- **Lever** (`#3B5B7A` / `#7BA3CC` dark) — Cold Steel Blue. The force multiplier. Primary buttons, active nav indicators, links, focus rings.
- **Load** (`#A8801F` / `#D4A53D` dark) — Aged Brass. The thing being moved. In-progress states, claimed beads, emphasis text, warnings.
- **Fulcrum** (`#3F4147` / `#9CA0A8` dark) — Iron Gray. The pivot point. Neutral structural accents, secondary actions.

**Surfaces** — three tiers of warm parchment:

- Canvas (`#F4EFE6` / `#1A1815`) — page background. Never used for card fill.
- Surface (`#FBF8F2` / `#26231F`) — the default container background.
- Elevated (`#FFFFFF` / `#2E2A25`) — inputs, modal overlays, highest-layer panels.

**Text** — ink and muted sepia, never cold gray:

- Ink (`#1F2125` / `#EDE6D6`) — primary content.
- Muted (`#6B6558` / `#8E8674`) — secondary labels, metadata, timestamps.

**Borders** — one value per mode: `#E4DDD0` (light hairline tan) / `#34302A` (dark ember).

Do not use standard Tailwind gray-XXX classes. All colors must come from this token set.

## Typography

Two typographic registers:

**Admin (dense):** Inter only, all weights. Scale is compressed — headlines stay small to maximize screen real estate. Hierarchy through weight (400→800) and case (sentence vs ALL CAPS). Monospace for IDs, timestamps, and system data.

**Microsite (editorial):** Inter for structural/UI text. Newsreader serif for body prose. Space Grotesk for mono-labels and CTAs.

Rules that apply everywhere:
- `label-caps`: always uppercase, 0.05em letter-spacing. Never sentence case.
- `mono-code`: reserved for machine-readable data — IDs, hashes, timestamps, CLI output.
- Headlines use negative letter-spacing. No loose tracking on large type.
- Body text never exceeds 720px / ~70ch column width.

## Layout

**Admin:** 256px fixed left sidebar, 40px fixed top bar, scrollable main area with 12px gutter. 4px base unit, 12-column grid. Sections separated by border lines, not whitespace.

**Microsite:** 1200px max container, 720px prose column, 128px vertical rhythm between sections, 64px fixed top nav.

## Elevation & Depth

Zero shadows anywhere. Depth through:
1. Tonal layering — Canvas → Surface → Elevated.
2. Hairline borders — 1px `border-line` everywhere.
3. Left-rail accent — 2px `accent-lever` left border on active nav item.
4. Inversion for primary actions — `accent-lever` fill with `bg-elevated` text.

## Shapes

**Admin console:** 0px border-radius on everything. Completely sharp. Exception: 9999px for 2×2px status indicator dots only.

**Microsite:** 4px default radius for interactive elements (buttons, inputs). Cards remain fully sharp (0px).

## Components

**Buttons:** Primary = `accent-lever` fill, `bg-elevated` text, 0px radius, uppercase `label-caps`. Ghost = transparent fill, `border-line` border. No shadows, no pill shapes.

**Status Badges:** 0px radius. Color = semantic role at 10% opacity background, full color text and 20% border. Always `label-caps`.

**Feature Cards:** Sharp (0px radius), `bg-surface` fill, `border-line` hairline, 24px padding. No gradient washes — tonal distinction comes from surface layering alone.

**Terminal Pane:** `terminal-bg-deep` background, `terminal-fg` text, mono-code font, 4px radius on outer container.

**Navigation (Admin):** Fixed 256px sidebar. Active: 2px `accent-lever` left border + `bg-surface` fill + bold. Inactive: transparent, `fg-muted` text.

**Navigation (Microsite):** Fixed 64px top bar, uppercase Space Grotesk links, active underline, collapses at 768px.

## Do's and Don'ts

**Do:**
- Map accent colors to their lever/load/fulcrum roles consistently.
- Keep the sober, utilitarian register — if it could appear on an AI product launch deck, it doesn't belong here.
- Use dark backgrounds (`terminal-bg-deep`) for CLI and demo visuals.
- Derive all spacing from the 4px base unit.

**Don't:**
- Use gradient washes, glows, or particle effects.
- Use the three accents interchangeably — each carries a specific mechanical role.
- Use standard Tailwind gray-XXX classes anywhere.
- Round cards, badges, or table containers (admin surface).
- Anthropomorphize AI components in illustration.

## Metaphor System

| Element | Role | Visual treatment |
|---|---|---|
| Lever | Abstraction transmitting intent | Mechanism, directional force, stack layers |
| Load | LLM output — probabilistic, costly | Mass, weight, irregular shape |
| Fulcrum | Human-AI collaboration pivot | Contact point, seam between judgment and execution |

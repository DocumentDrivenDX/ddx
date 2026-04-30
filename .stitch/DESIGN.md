---
version: alpha
name: DDx Design System
description: >
  Unified design language for the DDx platform. Shared across the admin console
  (dense terminal dashboard) and the marketing microsite (editorial monograph).
  Both surfaces use the same token vocabulary — lever/load/fulcrum — applied at
  different densities.

colors:
  # --- Accent triple (semantic, mode-invariant names) ---
  accent-lever: "#3B5B7A"        # Cold Steel Blue — primary actions, active states
  accent-load: "#A8801F"         # Aged Brass — in-progress, emphasis, warnings
  accent-fulcrum: "#3F4147"      # Iron Gray — neutral structural, secondary actions

  # --- Light mode surfaces ---
  bg-canvas: "#F4EFE6"           # Warm cream paper — page background
  bg-surface: "#FBF8F2"          # Parchment — cards, sidebars, containers
  bg-elevated: "#FFFFFF"         # Pure white — inputs, highest elevation

  # --- Light mode text ---
  fg-ink: "#1F2125"              # Near-black ink — primary text
  fg-muted: "#6B6558"            # Sepia muted — labels, placeholders, secondary text

  # --- Light mode borders ---
  border-line: "#E4DDD0"         # Hairline tan — all dividers and borders

  # --- Dark mode surfaces ---
  dark-bg-canvas: "#1A1815"      # Deep warm near-black — page background
  dark-bg-surface: "#26231F"     # Warm charcoal — cards, sidebars
  dark-bg-elevated: "#2E2A25"    # Lifted char — highest elevation panels

  # --- Dark mode accents (lightened for readability) ---
  dark-accent-lever: "#7BA3CC"   # Ice Steel Blue — primary actions
  dark-accent-load: "#D4A53D"    # Warm Brass — in-progress, emphasis
  dark-accent-fulcrum: "#9CA0A8" # Neutral Silver — structural neutral

  # --- Dark mode text ---
  dark-fg-ink: "#EDE6D6"         # Warm bone — primary text
  dark-fg-muted: "#8E8674"       # Dusty — secondary text, labels

  # --- Dark mode borders ---
  dark-border-line: "#34302A"    # Ember hairline — all dividers

  # --- Terminal (shared both modes) ---
  terminal-bg: "#1F2125"
  terminal-bg-deep: "#0F0E0C"   # Microsite dark terminal — void black
  terminal-fg: "#D8D2C4"

  # --- Semantic status (light) ---
  status-open: "#1D4ED8"
  status-in-progress: "#A8801F"  # maps to accent-load
  status-closed: "#15803D"
  status-blocked: "#B91C1C"
  status-parked: "#6B6558"       # maps to fg-muted

  # --- Error ---
  error: "#BA1A1A"

typography:
  # Admin console scale (Inter-only, dense)
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
    fontWeight: 700
    lineHeight: 1.1
    letterSpacing: -0.02em
  h1:
    fontFamily: Inter
    fontSize: 40px
    fontWeight: 600
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
  none: 0px
  sm: 2px
  DEFAULT: 4px
  lg: 8px
  full: 9999px

spacing:
  base: 4px
  xs: 4px
  sm: 8px
  gutter: 12px
  md: 16px
  lg: 24px
  xl: 32px
  section: 128px
  prose-max: 720px
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
  nav-active:
    borderLeft: "2px solid {colors.accent-lever}"
    background: "{colors.bg-surface}"
  status-badge:
    borderRadius: "{rounded.none}"
    typography: "{typography.label-caps}"
---

## Overview

DDx is a platform for document-driven AI development. The visual identity reflects its
guiding metaphor: **mechanical advantage** — levers, loads, and fulcrums. Nothing decorative.
Everything structural.

The design system spans two distinct surfaces that share one token vocabulary:

- **Admin Console** — a high-density terminal dashboard. Maximum information per pixel.
  Think a hardware diagnostic panel or a printed maintenance ledger. Inter-only typography,
  0px border-radius everywhere, zero shadows, 4px spacing grid.

- **Marketing Microsite** — a scholarly engineering monograph. Generous whitespace, 128px
  section breaks, editorial Newsreader serif body copy, Space Grotesk mono-labels. The same
  palette applied at low density.

Both surfaces reject: gradients, glassmorphism, rounded-pill buttons, drop shadows,
decorative icons, AI-gloss aesthetics. Hierarchy comes from tonal layering and hairline
borders alone.

## Colors

The palette is three accents over a warm-paper foundation. All surfaces are warm-toned
(cream, bone, near-black) rather than cold gray or pure white/black.

**Accent triple** — every interactive or state-bearing element maps to one of three roles:

- **Lever** (`#3B5B7A` light / `#7BA3CC` dark) — the force multiplier. Primary buttons,
  active nav indicators, links, focus rings. Cold Steel Blue.
- **Load** (`#A8801F` light / `#D4A53D` dark) — the thing being moved. In-progress states,
  claimed beads, emphasis text, warnings. Aged Brass / Warm Brass.
- **Fulcrum** (`#3F4147` light / `#9CA0A8` dark) — the pivot point. Neutral structural
  accents, secondary actions, iron-gray elements. Iron Gray / Neutral Silver.

**Surfaces** — three tiers of warmth:

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

**Admin (dense):** Inter only, all weights. Scale is compressed — headlines stay small to
maximize screen real estate. Hierarchy through weight (400→800) and case (sentence vs ALL
CAPS). Monospace is `ui-monospace` / JetBrains Mono for IDs, timestamps, and system data.

**Microsite (editorial):** Inter for all structural/UI text. Newsreader serif for body
prose (the academic signature of the site). Space Grotesk for mono-labels and CTAs — more
legible than full monospace, less cold.

Typography rules that apply everywhere:
- `label-caps`: always uppercase, always 0.05em letter-spacing. Never sentence case.
- `mono-code`: reserved for machine-readable data — IDs, hashes, timestamps, CLI output.
- Headlines use negative letter-spacing (`-0.01em` to `-0.02em`). No loose tracking on
  large type.
- Body text never exceeds 720px / ~70ch column width.

## Layout

**Admin layout shell:**
- 256px fixed left sidebar (nav + branding)
- 40px fixed top bar (search + status + tools)
- Scrollable main area with 12px gutter padding
- 4px base unit. All spacing in multiples of 4.
- 12-column grid for main content areas
- Sections separated by border lines, not whitespace

**Microsite layout:**
- 1200px max content container, centered
- 720px prose column (h1 + body paragraphs)
- 128px (`xl: section`) vertical rhythm between major sections
- Fixed 64px top nav

**Both:** No horizontal scrolling. No breakpoint below desktop in the admin. Microsite
responds at md (768px) for nav collapse.

## Elevation & Depth

Zero shadows anywhere. Depth is communicated exclusively through:

1. **Tonal layering** — Canvas → Surface → Elevated is the progression. In dark mode
   this inverts relative lightness but the same three tiers apply.
2. **Hairline borders** — 1px `border-line` separates every functional zone.
3. **Left-rail accent** — The active nav item gets a 2px left border in `accent-lever`.
   This is the strongest spatial indicator in the admin UI.
4. **Inversion for primary actions** — `accent-lever` fill with `bg-elevated` text. The
   high-contrast inversion reads as "press me" without any shadow.

Focus rings use `accent-lever` border color (no outer glow/box-shadow).

## Shapes

**Admin console:** 0px border-radius on everything — buttons, inputs, cards, badges,
table containers. Completely sharp. Exceptions: `border-radius: 9999px` is allowed for
status indicator dots only (the 2×2px pulse indicators).

**Microsite:** 4px default radius (`rounded-DEFAULT`) for interactive elements (buttons,
inputs). Still reads as nearly sharp — just enough to distinguish interactive from static.
Cards remain fully sharp (0px).

## Components

**Buttons**
- Primary: `accent-lever` fill, `bg-elevated` text, 0px radius, `label-caps` typography,
  uppercase, tracked. Hover: opacity 90%. No shadow.
- Ghost: transparent fill, `border-line` border, same shape. Hover: `bg-surface` fill.
- Destructive: `error` fill, white text.

**Status Badges**
Sharp rectangles (`border-radius: 0px`). Color = semantic role tint at 10% opacity for
background, full color for text and border at 20% opacity. Typography is `label-caps`.
States: OPEN/RUNNING (steel), IN-PROGRESS/CLAIMED (brass), CLOSED/COMPLETED (green),
BLOCKED/FAULT (red), PARKED/IDLE (muted).

**Data Tables**
Full-width, `border-collapse`. Header row: subtle canvas tint. Row dividers: `border-line`
1px. Row hover: light canvas tint. ID columns: `mono-code`. Actions column: icon-only.
Progress bars: 1px height, no radius, `accent-load` fill.

**Navigation (Admin)**
Left sidebar, fixed 256px. Active item: 2px `accent-lever` left border + bold text + 
`bg-surface` background. Inactive: transparent left border, `fg-muted` text. Hover:
`bg-surface` background.

**Navigation (Microsite)**
Fixed 64px top bar. Logo: `font-mono font-bold tracking-tighter`. Links: 12px uppercase
Space Grotesk, 8-unit spacing. Active: underline. Mobile: collapses at 768px.

**Inputs / Search**
`bg-elevated` background, `border-line` border, 0px radius (admin) / 4px (microsite).
Focus: border becomes `accent-lever`. No outline ring. Placeholder: `fg-muted`.

**Sidebar Nav (Admin)**
Items: 12px vertical padding, 24px horizontal. Leading icon (18px). All-caps label.
Bottom section: Docs, Logs — separated by a `border-line` top border.

**Terminal Pane**
Background: `terminal-bg-deep` (`#0F0E0C`), foreground: `terminal-fg` (`#D8D2C4`).
Font: mono-code stack. 4px border-radius on the outer container.

**Editorial Cards (Microsite Principles Grid)**
Square `aspect-square`, 48px padding, white at 50% opacity over canvas. `border-line`
hairline. Hover: border color shifts to thematic accent. Interior animated underline:
`h-[2px] w-8 → w-full` transition.

**Pull Quote (Microsite)**
2px `accent-load` left rail. 30% white background. Italic `body-editorial` text. No icon.

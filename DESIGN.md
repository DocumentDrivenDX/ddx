---
version: "alpha"
name: DDx
description: Visual language for Document-Driven Development eXperience — a terminal-first developer tool whose three-color identity maps to the lever/load/fulcrum metaphor.
colors:
  primary: "#4878C6"
  secondary: "#35A35F"
  tertiary: "#8E35A3"
  background: "#FFFFFF"
  surface: "#F9FAFB"
  surface-dark: "#0F1117"
  text: "#111827"
  text-muted: "#6B7280"
  text-on-primary: "#FFFFFF"
  text-on-secondary: "#FFFFFF"
  text-on-tertiary: "#FFFFFF"
  border: "#E5E7EB"
  terminal-bg: "#0F1117"
  terminal-fg: "#E8EAF0"
typography:
  h1:
    fontFamily: Inter
    fontSize: 2.5rem
    fontWeight: 700
    lineHeight: 1.2
  h2:
    fontFamily: Inter
    fontSize: 1.75rem
    fontWeight: 600
    lineHeight: 1.3
  body-md:
    fontFamily: Inter
    fontSize: 1rem
    fontWeight: 400
    lineHeight: 1.6
  body-sm:
    fontFamily: Inter
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.5
  code:
    fontFamily: JetBrains Mono
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: Inter
    fontSize: 0.75rem
    fontWeight: 500
    lineHeight: 1.4
    letterSpacing: 0.05em
rounded:
  sm: 4px
  md: 8px
  lg: 12px
spacing:
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 40px
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.text-on-primary}"
    rounded: "{rounded.md}"
    padding: 10px
  button-alt:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text}"
    rounded: "{rounded.md}"
    padding: 10px
  feature-card:
    backgroundColor: "{colors.background}"
    textColor: "{colors.text}"
    rounded: "{rounded.lg}"
    padding: 24px
  terminal-block:
    backgroundColor: "{colors.terminal-bg}"
    textColor: "{colors.terminal-fg}"
    rounded: "{rounded.md}"
    padding: 16px
---

## Overview

DDx is a terminal-first developer platform. Its visual language is **sober, utilitarian, and purposely non-decorative** — the antithesis of AI-gloss, dreamy illustration, and gradient excess. Every visual choice serves comprehension, not style.

The product's three-color identity maps directly to its core metaphor: the **lever, the load, and the fulcrum**. Abstraction is the lever — it transmits intent across levels of the stack. LLMs are the load — probabilistic, costly, not fully in control. Human-AI collaboration is the fulcrum — the stable pivot where judgment meets execution. These are not decorative categories; they are architectural facts about how DDx works, and the palette encodes them.

The primary surface is documentation — markdown, terminal output, CLI reference pages. Visuals live alongside text, not in competition with it. The register is developer-grade: precise, legible at a glance, trustworthy at scale.

## Colors

Three brand colors anchor the palette, each carrying semantic weight from the metaphor system.

- **Primary (#4878C6 — lever blue):** The mechanism. Used for primary actions, links, dispatch signals, and any UI element that transmits intent — the lever arm in motion.
- **Secondary (#35A35F — fulcrum green):** The pivot. Used for success states, progress indicators, and human-AI collaboration surfaces — stable ground where control is exercised.
- **Tertiary (#8E35A3 — load purple):** The weight. Used for AI/agent surfaces, stochastic outputs, cost indicators, and any context where probabilistic or uncertain mass is present.

These three colors appear as radial gradients on feature cards on the landing page (15% opacity on white), preserving legibility while anchoring semantic meaning. At full opacity they are used only for icons, badges, and deliberate accent marks — never as large fill blocks.

Surface and text tokens support both light-mode documentation and dark terminal contexts. The terminal palette (`terminal-bg: #0F1117`, `terminal-fg: #E8EAF0`) matches the tone of the animated demo recordings that are the product's primary visual artifact.

## Typography

Two families. One for prose and UI; one for code.

- **Inter** handles all headings, body copy, labels, and navigation. It is neutral without being generic: the right weight for technical documentation that needs to be read fast, not admired.
- **JetBrains Mono** handles all code blocks, terminal output, CLI references, and inline code. Monospace is not decorative here — it is the medium. Most of what DDx produces and consumes is text in a terminal.

Type scale is conservative: three heading sizes, two body sizes, one label size. No display sizes. This is reference material, not marketing copy.

## Layout

Wide-page layout, no max-width on docs pages. Content takes the space it needs; sidebars carry navigation, not decoration. Spacing scale is small-grained at the low end (4px grid) to support tight CLI output tables and artifact metadata blocks.

Horizontal rhythm: content rarely bleeds to the viewport edge. Comfortable line lengths (65–80 characters) for prose sections; full width for tables and code blocks.

## Components

**button-primary:** The primary call-to-action — lever blue with white text. Used for "Get Started" and primary agent-dispatch triggers. Square-ish radius (8px), not pill. Pill buttons signal consumer software; DDx is a developer tool.

**button-alt:** Secondary action — light surface with dark text. Used alongside button-primary for "Learn More" and non-destructive secondary choices.

**feature-card:** White card with 12px radius, 24px interior padding. Background accent is a radial gradient at 15% opacity using the appropriate brand color (blue for lever/tracker, green for fulcrum/plugins, purple for load/execution). Never a full-opacity color fill.

**terminal-block:** Dark (#0F1117) block with light text (#E8EAF0), 8px radius, 16px padding. Used for CLI command examples, agent output, and demo code. This is the product's native context; treat it as first-class, not supplementary.

## Do's and Don'ts

**Do:**
- Use the three brand colors with semantic intent — blue for action/dispatch, green for collaboration/success, purple for AI/stochastic contexts.
- Maintain the sober, utilitarian register. If an image could appear on an AI company's landing page, it does not belong here.
- Render the lever/load/fulcrum metaphor concretely: physical objects, mechanical diagrams, DAG graphs, state machines. Abstract geometry is acceptable only if the metaphor parses without explanatory copy.
- Use dark backgrounds for terminal-context visuals. The animated demo recordings set the tone; generated images should match it.
- Keep file sizes under 200KB for generated visuals. Compress and downsize before committing.

**Don't:**
- Use gradient washes, glows, lens flares, particle effects, or any visual treatment that signals "AI art." This is a developer tool, not a product launch deck.
- Use the three brand colors interchangeably. Each color carries a specific role in the metaphor system; mixing them dilutes the signal.
- Place the lever/load/fulcrum metaphor as decorative background texture. If it appears, it carries meaning.
- Use illustration styles that anthropomorphize the AI components (no robots, no glowing brains, no circuit-board heads). LLMs are load — treat them as mass, not character.
- Ship visuals that require explanatory copy to parse. If the metaphor doesn't read in the image, revise the visual concept, not the caption.

## Metaphor System

The lever/load/fulcrum triad is the load-bearing metaphor for all visual communication across the three lenses (principles, tools, user workflow).

| Element | Role | Visual treatment |
|---|---|---|
| Lever | Abstraction transmitting intent across levels | Arm, mechanism, directional force, stack layers |
| Load | LLM output — probabilistic, costly, non-uniform | Mass, weight, irregular shape, probabilistic scatter |
| Fulcrum | Human-AI collaboration — the stable pivot | Contact point, pivot, seam between human judgment and agent execution |

The three brand colors map to these roles. Visuals for the **principles lens** use this metaphor directly and concretely — not as abstract icon, but as mechanical or physical object. Visuals for the **tools lens** encode the metaphor through product structure (DAGs, worktrees, plugin composition). The **workflow capstone** shows the full loop: lever in motion, load being moved, fulcrum holding.

A visual that replaces the metaphor with generic tech imagery (nodes, code text, glowing interfaces) fails the brand-fit criterion regardless of color accuracy.

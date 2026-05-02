---
title: "Principle 1 — Abstraction is the lever"
principle: "Abstraction is the lever"
component: "lever arm"
generator: nano-banana-pro-openrouter
model: gemini-3-pro-image-preview
aspect_ratio: "16:9"
size: "2K"
depends_on:
  - website/assets/img-prompts/_preamble.md
  - .stitch/DESIGN.md
  - docs/helix/00-discover/product-vision.md
---

# Preamble

Apply the shared DDx visual preamble at `website/assets/img-prompts/_preamble.md`:
patent-grade engineering schematic on a factory blueprint, software-factory
mood, slightly steampunk in materiality (brass cartouche, copper rivet
punctuation at the chrome only) and slightly cyberpunk in lighting (faint
terminal-cyan readouts, restrained amber signal lamps). The diagram below is
the subject; the industrial framing lives at the perimeter — corner cartouche,
registration ticks, blueprint-grid bleed, and a small `DDx · FIG.P1`
maker's mark in the bottom-right registration block.

# Prompt

A single mechanical lever arm rendered as a precise technical diagram, viewed from the side. The arm is a long straight beam, segmented into four stacked horizontal levels stepping upward from left to right — each segment labeled with a thin sans-serif annotation in **#111827** on **#FFFFFF**: from bottom to top, "machine code", "language", "framework", "intent". Force vector arrows along the beam in **#4878C6** (lever blue, hex 4878C6) point upward and to the right, indicating intent transmitting from the lowest level of the stack to the highest. The beam is solid matte blue **#4878C6** with crisp matte shading — no gloss, no gradient wash, no glow.

The arrows are the lever blue at full opacity; the beam itself is the same blue at roughly 85% value. The fulcrum (a small triangular pivot in **#35A35F**, fulcrum green, hex 35A35F) sits at the lower-left base of the lever where it meets a thin horizontal ground line in **#6B7280**. The load is not depicted in this image — only the lever transmitting force.

Background: flat **#FFFFFF** (light) with subtle grid registration marks at 4% opacity in **#E5E7EB** for engineering-drawing register. Typography in Inter, regular weight, small label size. Tone: engineering schematic, patent-drawing precision, technical reference manual. Sober, utilitarian, mechanical.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6** primary; fulcrum green **#35A35F** as the pivot accent; neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#FFFFFF** only. No tertiary load purple in this image.
- Typography: Inter for labels, regular weight, small (label/0.75rem) scale. No display type, no decorative text.
- Register: technical diagram, mechanical drawing. Treat as a patent figure, not an illustration.
- No gradient washes, glows, lens flares, particle effects, dreamy lighting, or AI-art treatments.
- No anthropomorphism (no hands, no faces, no characters).
- Metaphor must parse without caption: a stacked lever arm with intent transmitting upward.

# Industrial framing (preamble enforcement)

Wrap the figure in a thin **brass `#9A6A2F` cartouche** (1 px stroke, 6 px inset
from the canvas edge) with four small registration ticks at the corners. The
canvas background is blueprint paper (`#F7F1E7` light variant) overlaid with
the existing 4 % `#E5E7EB` registration grid; allow a 2 % brushed-steel vignette
at the deep margins. Place a small bottom-right registration block reading
**`DDx · FIG.P1`** in JetBrains Mono 0.625 rem `#6B7280`, and at top-left a
**`DOC-DRIVEN SOFTWARE FACTORY`** kicker in Space Grotesk small-caps 0.625 rem
`#9A6A2F` (brass), letter-spacing 0.08 em. Permit at most **two** pinpoint
amber `#C79B5B` or terminal-cyan `#7FE3D4` signal-lamp dots (≤ 6 px diameter,
matte, no halation) as instrument punctuation; never as the figure's subject.
Optional: a single faint copper rivet glyph at each cartouche corner. The
diagram itself (lever arm with stacked levels of abstraction) must remain the dominant read at first glance —
the industrial framing rewards the second look without competing with the
metaphor.

# Anchoring (recently-landed positioning)

Anchored to the **Abstraction is the lever** principle (vision §Physics of Software, principle 1) and to the locked **Spec-First Development** principle on the live site (`/docs/principles/spec-first-development/`). The stacked beam visualizes the artifact graph — vision → spec → test → code — that DDx's document library maintains as the spine of the software factory.

# Negative prompt

3D render, photographic realism, gradient backgrounds, neon, glow, bokeh, depth-of-field blur, particles, sparkles, cinematic lighting, painterly illustration, hand-drawn sketch style, watercolor, isometric video-game aesthetic, robots, brains, circuit boards, cables, screens, characters, faces, hands, marketing-deck composition.

# Acceptance criteria

- Reads as a lever arm with stacked levels of abstraction without explanatory caption.
- Lever blue **#4878C6** dominates; fulcrum green **#35A35F** appears only at the pivot.
- Sober/utilitarian register; no AI-gloss.
- Light/dark mode contrast OK; mobile crop preserves the metaphor.
- File size ≤ 200KB after compression.

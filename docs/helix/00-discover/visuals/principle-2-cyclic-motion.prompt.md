---
title: "Principle 2 — Software is iteration over tracked work"
principle: "Software is iteration over tracked work"
component: "cyclic motion of the lever"
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
registration ticks, blueprint-grid bleed, and a small `DDx · FIG.P2`
maker's mark in the bottom-right registration block.

# Prompt

A side-view technical diagram of a single lever arm caught mid-stroke, with motion-study overlays showing the same lever in five discrete positions arcing through a cycle — a chronophotograph in the style of a 19th-century mechanical motion study (Marey/Muybridge), but rendered as a clean engineering schematic, not a photograph. The lever beam is matte **#4878C6** (lever blue) at full opacity for the current position; prior positions of the same arm are drawn as thinner outlines in **#4878C6** at 30% opacity, ghosted along the arc.

Along the arc path, place five small discrete square tokens — one at each lever position — labeled with thin Inter labels: "bead-001", "bead-002", "bead-003", "bead-004", "bead-005". Each token is a flat outlined square in **#111827** stroke on **#FFFFFF** fill, no shading. The tokens are not connected by a curve; they are discrete tracked items that the lever reaches in sequence.

A small fulcrum triangle in **#35A35F** (fulcrum green) anchors the pivot at the base of the arc. A thin curved dashed arrow in **#6B7280** indicates direction of cyclic motion — the cycle continues, the work is countable but never finished.

Background: **#FFFFFF** with 4% **#E5E7EB** grid registration. Typography: Inter, label size, regular weight.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6** for the lever; fulcrum green **#35A35F** for the pivot; neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#FFFFFF**. No tertiary load purple.
- Typography: Inter labels at small scale.
- Register: motion-study schematic — Marey-style chronophotograph rendered as engineering line art.
- No gradient washes, glows, particles, cinematic lighting, AI-art aesthetics.
- The discreteness of tracked items must read: five distinct square tokens, not a continuous curve.
- Metaphor must parse without caption: a lever in cyclic motion, work as discrete countable items.

# Industrial framing (preamble enforcement)

Wrap the figure in a thin **brass `#9A6A2F` cartouche** (1 px stroke, 6 px inset
from the canvas edge) with four small registration ticks at the corners. The
canvas background is blueprint paper (`#F7F1E7` light variant) overlaid with
the existing 4 % `#E5E7EB` registration grid; allow a 2 % brushed-steel vignette
at the deep margins. Place a small bottom-right registration block reading
**`DDx · FIG.P2`** in JetBrains Mono 0.625 rem `#6B7280`, and at top-left a
**`DOC-DRIVEN SOFTWARE FACTORY`** kicker in Space Grotesk small-caps 0.625 rem
`#9A6A2F` (brass), letter-spacing 0.08 em. Permit at most **two** pinpoint
amber `#C79B5B` or terminal-cyan `#7FE3D4` signal-lamp dots (≤ 6 px diameter,
matte, no halation) as instrument punctuation; never as the figure's subject.
Optional: a single faint copper rivet glyph at each cartouche corner. The
diagram itself (cyclic motion of the lever over a queue of tracked work) must remain the dominant read at first glance —
the industrial framing rewards the second look without competing with the
metaphor.

# Anchoring (recently-landed positioning)

Anchored to **Software is iteration over tracked work** and to the **Work is a DAG** principle (`/docs/principles/work-is-a-dag/`). The five labeled bead tokens are real DDx beads — the priority queue that `ddx work` drains. The cycle is the Ralph loop: a fresh bounded context per iteration.

# Negative prompt

3D render, photographic realism, neon, glow, bokeh, particles, motion blur (other than ghosted outlines), sparkles, gradient sky, dreamy illustration, watercolor, hand-drawn sketch, robots, characters, faces, hands, screens, code-as-imagery, marketing aesthetic.

# Acceptance criteria

- Reads as cyclic motion of a lever with discrete tracked work items, without caption.
- Five distinct beads visible along the arc.
- Lever blue dominates; pivot green only at fulcrum.
- Sober/utilitarian register; no AI-gloss.
- Light/dark contrast OK; mobile crop preserves at least three motion frames + tokens.
- File size ≤ 200KB.

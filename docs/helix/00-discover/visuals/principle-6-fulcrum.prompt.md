---
title: "Principle 6 — Human-AI collaboration is the fulcrum"
principle: "Human-AI collaboration is the fulcrum"
component: "fulcrum / pivot — contact seam between human and AI"
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
registration ticks, blueprint-grid bleed, and a small `DDx · FIG.P6`
maker's mark in the bottom-right registration block.

# Prompt

A close-up technical detail view of the fulcrum joint of a lever — the pivot triangle where the beam meets the ground — rendered as a precise mechanical-engineering cross-section. The fulcrum itself is a solid matte **#35A35F** (fulcrum green) triangular wedge, bearing the lever beam (in matte **#4878C6**, lever blue) at its apex.

The fulcrum's two faces are clearly differentiated by surface treatment, indicating two materials meeting at a stable seam:
- The left face is a plain matte **#35A35F** field, labeled in Inter small as "human judgment".
- The right face is the same **#35A35F** but cross-hatched with thin parallel lines suggesting a probabilistic texture, labeled "agent execution".
- A thin **#111827** stroke marks the seam where the two faces meet at the apex of the triangle — this is the contact line.

The lever beam rests directly on this seam, distributing force across both faces. A small load implied off-frame to the right, lever blue arrow indicating force direction. The composition is tight on the joint — the rest of the lever and the load are out of frame — to emphasize that the fulcrum is the load-bearing contact point.

Background: **#FFFFFF** with 4% **#E5E7EB** registration grid. Typography: Inter labels.

# Style constraints (DESIGN.md)

- Palette: fulcrum green **#35A35F** dominates as the wedge body; lever blue **#4878C6** for the beam resting on it; neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#FFFFFF**. No load purple.
- Typography: Inter, label scale, regular weight.
- Register: mechanical-engineering cross-section / detail callout. Patent-figure precision.
- The two faces of the fulcrum must be visually distinguishable but co-equal — neither human nor agent is privileged.
- No anthropomorphism (no hand, no face, no humanoid icon — the "human" face is a labeled material surface, not a figure).
- No gradient washes, glows, particles, cinematic lighting.
- Metaphor must parse without caption: the seam where two faces meet at the pivot is the load-bearing point.

# Industrial framing (preamble enforcement)

Wrap the figure in a thin **brass `#9A6A2F` cartouche** (1 px stroke, 6 px inset
from the canvas edge) with four small registration ticks at the corners. The
canvas background is blueprint paper (`#F7F1E7` light variant) overlaid with
the existing 4 % `#E5E7EB` registration grid; allow a 2 % brushed-steel vignette
at the deep margins. Place a small bottom-right registration block reading
**`DDx · FIG.P6`** in JetBrains Mono 0.625 rem `#6B7280`, and at top-left a
**`DOC-DRIVEN SOFTWARE FACTORY`** kicker in Space Grotesk small-caps 0.625 rem
`#9A6A2F` (brass), letter-spacing 0.08 em. Permit at most **two** pinpoint
amber `#C79B5B` or terminal-cyan `#7FE3D4` signal-lamp dots (≤ 6 px diameter,
matte, no halation) as instrument punctuation; never as the figure's subject.
Optional: a single faint copper rivet glyph at each cartouche corner. The
diagram itself (fulcrum joint where human judgment and agent execution meet) must remain the dominant read at first glance —
the industrial framing rewards the second look without competing with the
metaphor.

# Anchoring (recently-landed positioning)

Anchored to **Human-AI collaboration is the fulcrum** and to **Least Privilege for Agents** and **Inspect and Adapt** (`/docs/principles/least-privilege-for-agents/`, `/docs/principles/inspect-and-adapt/`). The two co-equal faces of the wedge are the seam DDx exists to support — neither side is privileged; both bear the load.

# Negative prompt

3D render, photographic realism, neon, glow, bokeh, sparkles, gradient sky, dreamy illustration, watercolor, hand-drawn sketch, robots, brains, characters, faces, hands, handshake imagery, human-and-robot pairing clichés, screens, marketing aesthetic.

# Acceptance criteria

- Reads as the fulcrum joint with two co-equal faces meeting at a load-bearing seam, without caption.
- Fulcrum green **#35A35F** dominates; lever blue **#4878C6** only on the beam; no load purple.
- Two faces visually distinguishable (plain vs. cross-hatched) and co-equal in weight.
- Sober/utilitarian register; no AI-gloss; no anthropomorphism, no handshake clichés.
- Light/dark contrast OK; mobile crop preserves both faces + seam + beam.
- File size ≤ 200KB.

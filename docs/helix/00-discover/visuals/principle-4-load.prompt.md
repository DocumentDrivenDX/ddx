---
title: "Principle 4 — LLMs are stochastic, unreliable, costly"
principle: "LLMs are stochastic, unreliable, costly"
component: "the load — probabilistic mass"
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
registration ticks, blueprint-grid bleed, and a small `DDx · FIG.P4`
maker's mark in the bottom-right registration block.

# Prompt

A technical diagram of a lever's load-end (right side of the beam) bearing an irregular, non-uniform mass rendered as a cluster of small **#8E35A3** (load purple) particles of varying size — a probabilistic scatter plot collapsed into a physical heap. The lever beam is matte **#4878C6** (lever blue) extending in from the left edge, terminating in a small flat platform on which the purple mass rests. The mass is not a single solid shape: it is composed of dozens of small circles and irregular polygons of different diameters, each at a slightly different position, suggesting weight that resists prediction.

A small "$" cost annotation in **#111827** Inter small sits beside the mass, with a thin **#6B7280** bracket indicating its variable extent. A second annotation reads "p(success) = ?" — also Inter small, **#111827**. The lever beam droops slightly under the load, conveying that the mass is heavy and uneven.

The fulcrum is implied off-frame to the left; this image is about the load, not the pivot.

Background: **#FFFFFF** with 4% **#E5E7EB** registration grid. Typography: Inter labels.

The purple mass must read as **mass** — not as character, not as glow, not as cloud. Solid pigment dots and polygons. Probabilistic, but physical.

# Style constraints (DESIGN.md)

- Palette: load purple **#8E35A3** for the mass; lever blue **#4878C6** for the beam; neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#FFFFFF**. No fulcrum green — fulcrum is off-frame.
- Typography: Inter, label scale, regular weight.
- Register: physics/statics diagram — load on a beam, with probabilistic annotation.
- No gradient washes, glows, particles-as-sparkle, cinematic lighting.
- Mass must read as physical weight, not as decorative purple cloud or AI-glow.
- No anthropomorphism: do not depict the LLM as a robot, brain, character, or face.
- Metaphor must parse without caption: irregular probabilistic mass on the load end of a beam.

# Industrial framing (preamble enforcement)

Wrap the figure in a thin **brass `#9A6A2F` cartouche** (1 px stroke, 6 px inset
from the canvas edge) with four small registration ticks at the corners. The
canvas background is blueprint paper (`#F7F1E7` light variant) overlaid with
the existing 4 % `#E5E7EB` registration grid; allow a 2 % brushed-steel vignette
at the deep margins. Place a small bottom-right registration block reading
**`DDx · FIG.P4`** in JetBrains Mono 0.625 rem `#6B7280`, and at top-left a
**`DOC-DRIVEN SOFTWARE FACTORY`** kicker in Space Grotesk small-caps 0.625 rem
`#9A6A2F` (brass), letter-spacing 0.08 em. Permit at most **two** pinpoint
amber `#C79B5B` or terminal-cyan `#7FE3D4` signal-lamp dots (≤ 6 px diameter,
matte, no halation) as instrument punctuation; never as the figure's subject.
Optional: a single faint copper rivet glyph at each cartouche corner. The
diagram itself (probabilistic, costly load on the lever) must remain the dominant read at first glance —
the industrial framing rewards the second look without competing with the
metaphor.

# Anchoring (recently-landed positioning)

Anchored to **LLMs are stochastic, unreliable, costly** and to the **Right-Size the Model** principle (`/docs/principles/right-size-the-model/`). The variable mass and the `$` annotation are the empirical drivers behind cost-tier routing: cheap models do, strong models review, deterministic checks at the top of the ladder.

# Negative prompt

3D render, photographic realism, neon, glow, bokeh, sparkles, gradient sky, dreamy illustration, watercolor, hand-drawn sketch, robots, brains, glowing orbs, characters, faces, hands, circuit boards, screens, code-as-imagery, marketing aesthetic, particle effects suggesting magic, purple haze.

# Acceptance criteria

- Reads as probabilistic, costly load on a lever, without caption.
- Load purple **#8E35A3** dominates the mass; lever blue **#4878C6** for the beam; no fulcrum green.
- Mass is rendered as discrete physical particles, not as glow or cloud.
- Sober/utilitarian register; no AI-gloss; no anthropomorphism.
- Light/dark contrast OK; mobile crop preserves the mass cluster + beam end.
- File size ≤ 200KB.

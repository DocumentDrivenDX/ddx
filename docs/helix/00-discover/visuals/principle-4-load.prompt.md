---
title: "Principle 4 — LLMs are stochastic, unreliable, costly"
principle: "LLMs are stochastic, unreliable, costly"
component: "the load — probabilistic mass"
generator: nano-banana-pro-openrouter
model: gemini-3-pro-image-preview
aspect_ratio: "16:9"
size: "2K"
depends_on:
  - .stitch/DESIGN.md
  - docs/helix/00-discover/product-vision.md
---

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

# Negative prompt

3D render, photographic realism, neon, glow, bokeh, sparkles, gradient sky, dreamy illustration, watercolor, hand-drawn sketch, robots, brains, glowing orbs, characters, faces, hands, circuit boards, screens, code-as-imagery, marketing aesthetic, particle effects suggesting magic, purple haze.

# Acceptance criteria

- Reads as probabilistic, costly load on a lever, without caption.
- Load purple **#8E35A3** dominates the mass; lever blue **#4878C6** for the beam; no fulcrum green.
- Mass is rendered as discrete physical particles, not as glow or cloud.
- Sober/utilitarian register; no AI-gloss; no anthropomorphism.
- Light/dark contrast OK; mobile crop preserves the mass cluster + beam end.
- File size ≤ 200KB.

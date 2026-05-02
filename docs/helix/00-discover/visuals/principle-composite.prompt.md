---
title: "Principle Composite — The full lever metaphor"
principle: "All six physics principles in one diagram"
component: "lever arm + fulcrum + load + trail + cycle + handles"
generator: nano-banana-pro-openrouter
model: gemini-3-pro-image-preview
aspect_ratio: "16:9"
size: "2K"
depends_on:
  - website/assets/img-prompts/_preamble.md
  - .stitch/DESIGN.md
  - docs/helix/00-discover/product-vision.md
  - docs/helix/00-discover/visuals/principle-1-lever.prompt.md
  - docs/helix/00-discover/visuals/principle-2-cyclic-motion.prompt.md
  - docs/helix/00-discover/visuals/principle-3-interchangeable-handles.prompt.md
  - docs/helix/00-discover/visuals/principle-4-load.prompt.md
  - docs/helix/00-discover/visuals/principle-5-trail.prompt.md
  - docs/helix/00-discover/visuals/principle-6-fulcrum.prompt.md
---

# Preamble

Apply the shared DDx visual preamble at `website/assets/img-prompts/_preamble.md`:
patent-grade engineering schematic on a factory blueprint, software-factory
mood, slightly steampunk in materiality (brass cartouche, copper rivet
punctuation at the chrome only) and slightly cyberpunk in lighting (faint
terminal-cyan readouts, restrained amber signal lamps). The diagram below is
the subject; the industrial framing lives at the perimeter — corner cartouche,
registration ticks, blueprint-grid bleed, and a small `DDx · FIG.P0`
maker's mark in the bottom-right registration block.

# Prompt

A single wide mechanical-engineering schematic, viewed from the side, that integrates all six elements of the lever metaphor in one coherent technical diagram — patent-figure precision, sober and utilitarian, no AI gloss.

Centered in the frame: a long straight **lever beam** in solid matte **#4878C6** (lever blue), segmented into four stacked horizontal levels stepping upward from left to right. Each segment carries a thin Inter label in **#111827**: from bottom to top, "machine code", "language", "framework", "intent". Force-vector arrows in the same lever blue along the beam point upward and to the right — intent transmitting through the abstraction stack.

At the lower-left base of the beam, a **fulcrum** triangle in solid matte **#35A35F** (fulcrum green). The fulcrum's two faces are visibly differentiated: the left face plain matte green, labeled "human judgment" in Inter small; the right face the same green cross-hatched with thin parallel lines suggesting probabilistic texture, labeled "agent execution". A thin **#111827** stroke marks the seam where the two faces meet at the apex — the load-bearing contact line. The beam rests on this seam.

To the right of the fulcrum, off the high end of the beam: the **load** — a non-uniform cluster of small irregular shapes (circles, squares, asymmetric blobs) in matte **#7A4E9F** (load purple), of varying size and value, suggesting probabilistic, heterogeneous mass. Some shapes overlap; a few have faint dashed outlines indicating uncertainty. The load purple appears only here.

To the left of the fulcrum, on the low end of the beam: three **interchangeable handles** drawn as different mechanical grip silhouettes in **#6B7280** outline — a T-handle, a crank, a lever bar — stacked or fanned, indicating they snap into the same socket on the beam. A small label "methodology" in Inter small underneath.

Below the beam, running along the **#6B7280** ground line: a horizontal **trail** of small uniform tick marks and tiny rectangular receipts (chain of small **#6B7280** glyphs at 60% opacity), spaced regularly along a thin baseline — the audit trail left by the lever's cyclic motion.

Above the beam, in the upper portion of the frame: faint **cyclic motion arcs** in **#4878C6** at 30% opacity — two or three nested curved arrows looping back, indicating the beam strokes repeatedly, discretizing work into tracked iterations. Small Inter labels "iterate" along the arcs.

The composition reads left-to-right as: handles (input choice) → fulcrum (human-AI seam) → beam with stacked abstraction levels (the lever doing work) → load (probabilistic output). The cyclic arcs frame the whole motion above; the trail of receipts records it below.

Background: flat **#FFFFFF** with 4% **#E5E7EB** registration grid in light mechanical-drawing style. Typography: Inter, regular weight, label/0.75rem scale only. Tone: engineering schematic, patent-drawing precision, technical reference manual. Sober, utilitarian, mechanical.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6**, fulcrum green **#35A35F**, load purple **#7A4E9F**, neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#FFFFFF**. Each color carries semantic weight: blue for the lever/motion, green for the human-AI seam, purple only for the load.
- Typography: Inter, regular, small label scale. No display type, no decorative text.
- Register: technical diagram, mechanical drawing. Treat as a single patent figure assembled from six callouts.
- No gradient washes, glows, lens flares, particle effects, dreamy lighting, or AI-art treatments.
- No anthropomorphism (no hands, no faces, no characters, no handshake imagery). The "human" face of the fulcrum is a labeled material surface.
- All six elements must be present and individually identifiable: stacked beam, fulcrum with two faces, load cluster, interchangeable handles, trail of receipts, cyclic arcs.
- Metaphor must parse without caption: a working lever lifting a probabilistic load, pivoting on a human-AI seam, driven by interchangeable methodologies, leaving an audit trail through cyclic strokes.

# Industrial framing (preamble enforcement)

Wrap the figure in a thin **brass `#9A6A2F` cartouche** (1 px stroke, 6 px inset
from the canvas edge) with four small registration ticks at the corners. The
canvas background is blueprint paper (`#F7F1E7` light variant) overlaid with
the existing 4 % `#E5E7EB` registration grid; allow a 2 % brushed-steel vignette
at the deep margins. Place a small bottom-right registration block reading
**`DDx · FIG.P0`** in JetBrains Mono 0.625 rem `#6B7280`, and at top-left a
**`DOC-DRIVEN SOFTWARE FACTORY`** kicker in Space Grotesk small-caps 0.625 rem
`#9A6A2F` (brass), letter-spacing 0.08 em. Permit at most **two** pinpoint
amber `#C79B5B` or terminal-cyan `#7FE3D4` signal-lamp dots (≤ 6 px diameter,
matte, no halation) as instrument punctuation; never as the figure's subject.
Optional: a single faint copper rivet glyph at each cartouche corner. The
diagram itself (all six lever-metaphor elements composed in one figure) must remain the dominant read at first glance —
the industrial framing rewards the second look without competing with the
metaphor.

# Anchoring (recently-landed positioning)

Capstone of the principles lens. Reads alongside the locked ten domain principles on `/docs/principles/` (Spec-First, Executable Specifications, Audit Trail Required, Context is King, Work is a DAG, Right-Size the Model, Avoid Vendor Lock-in, Drift is Debt, Least Privilege for Agents, Inspect and Adapt). Frames DDx as a **document-driven software factory** — the lever is the artifact graph, the fulcrum is the human-AI seam, the load is generative AI, the handles are plural methodologies, the trail is captured evidence, the cycle is the Ralph loop.

# Negative prompt

3D render, photographic realism, gradient backgrounds, neon, glow, bokeh, depth-of-field blur, particles, sparkles, cinematic lighting, painterly illustration, hand-drawn sketch style, watercolor, isometric video-game aesthetic, robots, brains, circuit boards, cables, screens, characters, faces, hands, handshake imagery, marketing-deck composition, infographic-with-icons style, flat-design illustration with mascots.

# Acceptance criteria

- All six elements (lever beam with stacked levels, fulcrum with two faces, load, handles, trail, cyclic arcs) are individually visible and identifiable without caption.
- Lever blue dominates the beam and motion; fulcrum green only at the pivot seam; load purple only at the load cluster; neutrals elsewhere.
- Sober/utilitarian register; reads as a patent figure, not an illustration.
- Light/dark mode contrast OK; mobile crop preserves the beam + fulcrum + load triple at minimum.
- File size ≤ 400KB after compression (composite is denser than per-principle frames).

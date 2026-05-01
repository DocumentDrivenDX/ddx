---
title: "Principle 3 — Methodology is plural"
principle: "Methodology is plural"
component: "interchangeable handles / grips"
generator: nano-banana-pro-openrouter
model: gemini-3-pro-image-preview
aspect_ratio: "16:9"
size: "2K"
depends_on:
  - .stitch/DESIGN.md
  - docs/helix/00-discover/product-vision.md
---

# Prompt

A technical exploded-view diagram of a single lever beam with four interchangeable grip attachments laid out in a horizontal row above it — the kind of figure you'd see in a tool catalog or a patent drawing showing accessories for a base implement. The beam itself is matte **#4878C6** (lever blue), centered horizontally. Above the beam, four distinct handle/grip shapes are drawn in matte **#4878C6** outline on **#FFFFFF** fill, each labeled in Inter small: "HELIX", "agile", "waterfall", "ad-hoc".

Each handle is geometrically distinct:
- HELIX: a structured grip with discrete notched segments (phase gates).
- agile: a compact rounded grip with iteration marks.
- waterfall: a long straight-edged grip with sequential bands.
- ad-hoc: an irregular asymmetric grip.

Thin **#6B7280** dashed alignment lines drop from each handle down to a single shared mounting socket on the lever beam, indicating any of them can be attached to the same lever. A small fulcrum triangle in **#35A35F** sits at the lever's base.

The composition emphasizes interchangeability: same lever, same fulcrum, same load (implied, off-frame), but plural handles. No handle is privileged; all are drawn at equal weight.

Background: **#FFFFFF** with 4% **#E5E7EB** registration grid. Typography: Inter labels.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6** for beam and handle outlines; fulcrum green **#35A35F** at pivot; neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#FFFFFF**.
- Typography: Inter, label scale, regular weight.
- Register: patent-figure / tool-catalog schematic — exploded view of interchangeable parts.
- No gradient washes, glows, particles, cinematic lighting.
- Equal visual weight for all four handles — no methodology privileged.
- Metaphor must parse without caption: one lever, multiple interchangeable handles.

# Negative prompt

3D render, photographic realism, neon, glow, bokeh, sparkles, gradient sky, dreamy illustration, watercolor, hand-drawn sketch, robots, characters, faces, hands, screens, marketing aesthetic, decorative ornament.

# Acceptance criteria

- Reads as interchangeable methodology grips on a shared lever, without caption.
- Four labeled handles, equal visual weight, distinct geometry.
- Lever blue dominates; fulcrum green only at pivot.
- Sober/utilitarian register; no AI-gloss.
- Light/dark contrast OK; mobile crop preserves at least three handles + beam.
- File size ≤ 200KB.

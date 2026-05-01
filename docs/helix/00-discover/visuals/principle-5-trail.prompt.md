---
title: "Principle 5 — Evidence provides memory"
principle: "Evidence provides memory"
component: "trail / chain of receipts left by the moving lever"
generator: nano-banana-pro-openrouter
model: gemini-3-pro-image-preview
aspect_ratio: "16:9"
size: "2K"
depends_on:
  - .stitch/DESIGN.md
  - docs/helix/00-discover/product-vision.md
---

# Prompt

A technical diagram of a lever beam in motion, leaving behind a trail of small printed receipts/dockets along the path of its load end. The lever is matte **#4878C6** (lever blue), captured at its current position on the right side of the frame; behind it, trailing leftward and slightly downward, is a sequence of six small rectangular receipt cards laid out like fallen tickets in chronological order — earlier ones further back, more faded.

Each receipt is a small **#FFFFFF** rectangle with a thin **#111827** stroke border, containing a few lines of monospace **JetBrains Mono** text in **#111827** at a very small size: hash-like identifiers ("a3f1c…", "b71e9…"), a timestamp, and a one-line "ran: ddx work" or "merged: bead-…" annotation. The receipts are not glossy — they are flat, like printed dockets from a thermal printer.

A thin **#6B7280** dotted line connects the receipts in sequence, suggesting a chain of provenance. The most recent receipt sits closest to the lever; the oldest fades to **#6B7280** outline at 50% opacity.

A small fulcrum triangle in **#35A35F** (fulcrum green) anchors the pivot at frame-left base. The load is implied (off-frame, on the right).

Background: **#FFFFFF** with 4% **#E5E7EB** registration grid. Typography: Inter for labels, JetBrains Mono for receipt content.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6** for the beam; fulcrum green **#35A35F** at pivot; neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#FFFFFF**. No load purple — load is off-frame.
- Typography: Inter for diagram labels; **JetBrains Mono** for receipt content (this is the only place monospace appears).
- Register: archival / forensics diagram — chain-of-custody dockets behind a moving instrument.
- Receipts must read as discrete printed artifacts, not as glowing UI cards.
- No gradient washes, glows, particles, cinematic lighting.
- Metaphor must parse without caption: a lever leaves a trail of receipts.

# Negative prompt

3D render, photographic realism, neon, glow, bokeh, sparkles, gradient sky, dreamy illustration, watercolor, hand-drawn sketch, robots, characters, faces, hands, screens, glowing UI cards, code-editor screenshots, marketing aesthetic.

# Acceptance criteria

- Reads as a chain of receipts trailing behind a moving lever, without caption.
- Six discrete receipt cards, with monospace content, in chronological order.
- Lever blue dominates; fulcrum green only at pivot.
- Sober/utilitarian register; no AI-gloss.
- Light/dark contrast OK; mobile crop preserves at least three receipts + beam.
- File size ≤ 200KB.

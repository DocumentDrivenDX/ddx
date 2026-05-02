---
title: "Principle 5 — Evidence provides memory"
principle: "Evidence provides memory"
component: "trail / chain of receipts left by the moving lever"
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
registration ticks, blueprint-grid bleed, and a small `DDx · FIG.P5`
maker's mark in the bottom-right registration block.

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

# Industrial framing (preamble enforcement)

Wrap the figure in a thin **brass `#9A6A2F` cartouche** (1 px stroke, 6 px inset
from the canvas edge) with four small registration ticks at the corners. The
canvas background is blueprint paper (`#F7F1E7` light variant) overlaid with
the existing 4 % `#E5E7EB` registration grid; allow a 2 % brushed-steel vignette
at the deep margins. Place a small bottom-right registration block reading
**`DDx · FIG.P5`** in JetBrains Mono 0.625 rem `#6B7280`, and at top-left a
**`DOC-DRIVEN SOFTWARE FACTORY`** kicker in Space Grotesk small-caps 0.625 rem
`#9A6A2F` (brass), letter-spacing 0.08 em. Permit at most **two** pinpoint
amber `#C79B5B` or terminal-cyan `#7FE3D4` signal-lamp dots (≤ 6 px diameter,
matte, no halation) as instrument punctuation; never as the figure's subject.
Optional: a single faint copper rivet glyph at each cartouche corner. The
diagram itself (audit trail of evidence receipts left by the moving lever) must remain the dominant read at first glance —
the industrial framing rewards the second look without competing with the
metaphor.

# Anchoring (recently-landed positioning)

Anchored to **Evidence provides memory** and to the locked **Audit Trail Required** principle (`/docs/principles/audit-trail-required/`). Each receipt is a real DDx execution evidence bundle: bead id, commit hash, harness, cost. The chain of dockets is the substrate that survives a fresh-context Ralph-loop iteration.

# Negative prompt

3D render, photographic realism, neon, glow, bokeh, sparkles, gradient sky, dreamy illustration, watercolor, hand-drawn sketch, robots, characters, faces, hands, screens, glowing UI cards, code-editor screenshots, marketing aesthetic.

# Acceptance criteria

- Reads as a chain of receipts trailing behind a moving lever, without caption.
- Six discrete receipt cards, with monospace content, in chronological order.
- Lever blue dominates; fulcrum green only at pivot.
- Sober/utilitarian register; no AI-gloss.
- Light/dark contrast OK; mobile crop preserves at least three receipts + beam.
- File size ≤ 200KB.

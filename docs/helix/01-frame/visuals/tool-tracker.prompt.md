---
title: "Tool — Bead tracker"
tool: "Bead tracker"
component: "DAG with priority queue + ready/blocked states"
generator: nano-banana-pro-openrouter
model: gemini-3-pro-image-preview
aspect_ratio: "16:9"
size: "2K"
depends_on:
  - website/assets/img-prompts/_preamble.md
  - .stitch/DESIGN.md
  - docs/helix/01-frame/prd.md
---

# Preamble

Apply the shared DDx visual preamble at `website/assets/img-prompts/_preamble.md`:
patent-grade engineering schematic on a factory blueprint, software-factory
mood, slightly steampunk in materiality (brass cartouche, copper rivet
punctuation at the chrome only) and slightly cyberpunk in lighting (faint
terminal-cyan readouts, restrained amber signal lamps). The diagram below is
the subject; the industrial framing lives at the perimeter — corner cartouche,
registration ticks, blueprint-grid bleed, and a small `DDx · FIG.T4`
maker's mark in the bottom-right registration block.

# Prompt

A precise technical diagram of a directed acyclic graph (DAG) of work items, viewed straight on as an engineering schematic. Roughly twelve small rounded-rectangle nodes (rounded corner radius 8px) are laid out left-to-right across four implicit columns; each node is labeled with a short id like **DDX-001**, **DDX-002**, etc. in **Inter, 0.75rem, weight 500**, color **#111827**. Directed arrows in **#6B7280** connect parent nodes to dependent children, all flowing left to right; arrowheads are small filled triangles, not gradients.

Three node states are visually distinguished by fill, with a small legend along the bottom edge:
- **Ready** — solid fill **#4878C6** (lever blue) with white label text **#FFFFFF**, indicating no unmet dependencies.
- **Blocked** — outlined only, 1.5px stroke **#6B7280** on a **#FFFFFF** fill, label in **#6B7280**, indicating an unmet upstream dependency.
- **Done** — solid fill **#35A35F** (fulcrum green) with white label text, with a thin checkmark glyph in **#FFFFFF**.

On the left edge of the canvas, a vertical **priority queue** column is depicted: a tall thin rounded rectangle on **#F9FAFB** surface containing the ready nodes (blue) stacked top-to-bottom in priority order, with a small numeric "1, 2, 3" rank in **#6B7280** at each slot's left margin. A thin **#4878C6** arrow exits the top of the queue and points right into the wider DAG, indicating "next bead pulled from the queue."

Background: flat **#FFFFFF** with subtle 4%-opacity grid registration marks in **#E5E7EB** for engineering-drawing register. A thin **#E5E7EB** legend strip along the bottom shows three small swatches (Ready / Blocked / Done) labeled in **Inter 0.75rem #6B7280**. Tone: technical reference manual, project-management diagram drawn as a patent figure, sober and utilitarian.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6** for ready/active, fulcrum green **#35A35F** for done, neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#F9FAFB**, **#FFFFFF** only. No tertiary purple in this image.
- Typography: Inter for all labels, regular/medium weight, label scale (0.75rem) only. JetBrains Mono permitted for the bead id text if it improves legibility. No display type, no decorative text.
- Register: technical schematic, project-management diagram. Treat as a patent figure, not an illustration.
- No gradient washes, glows, lens flares, particles, dreamy lighting, or AI-art treatments.
- No anthropomorphism (no hands, no faces, no characters, no avatars).
- Metaphor must parse without caption: a DAG of work items with a priority queue feeding ready beads into execution.

# Industrial framing (preamble enforcement)

Wrap the figure in a thin **brass `#9A6A2F` cartouche** (1 px stroke, 6 px inset
from the canvas edge) with four small registration ticks at the corners. The
canvas background is blueprint paper (`#F7F1E7` light variant) overlaid with
the existing 4 % `#E5E7EB` registration grid; allow a 2 % brushed-steel vignette
at the deep margins. Place a small bottom-right registration block reading
**`DDx · FIG.T4`** in JetBrains Mono 0.625 rem `#6B7280`, and at top-left a
**`DOC-DRIVEN SOFTWARE FACTORY`** kicker in Space Grotesk small-caps 0.625 rem
`#9A6A2F` (brass), letter-spacing 0.08 em. Permit at most **two** pinpoint
amber `#C79B5B` or terminal-cyan `#7FE3D4` signal-lamp dots (≤ 6 px diameter,
matte, no halation) as instrument punctuation; never as the figure's subject.
Optional: a single faint copper rivet glyph at each cartouche corner. The
diagram itself (bead tracker DAG with priority queue) must remain the dominant read at first glance —
the industrial framing rewards the second look without competing with the
metaphor.

# Anchoring (recently-landed positioning)

Anchored to **Work is a DAG** (`/docs/principles/work-is-a-dag/`). The priority queue is what `ddx work` drains; the DAG enforces dependency order. Ready beads enter execution; blocked beads wait on upstream closure; done beads carry evidence and a green check.

# Negative prompt

3D render, photographic realism, gradient backgrounds, neon, glow, bokeh, depth-of-field blur, particles, sparkles, cinematic lighting, painterly illustration, hand-drawn sketch style, watercolor, isometric video-game aesthetic, robots, brains, circuit boards, cables, screens, characters, faces, hands, marketing-deck composition, kanban board mockups, sticky notes, chat bubbles, gears.

# Acceptance criteria

- Reads as a DAG of tracked work items with a priority queue and three clear states without explanatory caption.
- Lever blue **#4878C6** dominates ready/active items; fulcrum green **#35A35F** appears only on completed nodes.
- Sober/utilitarian register; no AI-gloss.
- Light/dark mode contrast OK; mobile crop preserves the metaphor.
- File size ≤ 200KB after compression.

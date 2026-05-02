---
title: "Tool — Document graph (artifacts)"
tool: "Document graph"
component: "Multi-node graph with depends_on and generated_by edges"
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
registration ticks, blueprint-grid bleed, and a small `DDx · FIG.T1`
maker's mark in the bottom-right registration block.

# Prompt

A precise technical diagram of a multi-node document graph rendered as an engineering schematic, viewed straight on. Roughly ten document nodes are arrayed across the canvas in a layered layout: source artifacts on the left (vision, PRD, feature specs), derived artifacts in the middle (design plans, beads), and generated artifacts on the right (visuals, reports). Each node is a small rounded rectangle (rounded corner radius 8px), filled **#FFFFFF** with a 1.5px stroke **#111827**, labeled in **Inter 0.75rem weight 500 #111827** with a short doc name like `vision.md`, `prd.md`, `tool-tracker.png`, `bead-DDX-001`.

Two distinct edge kinds connect the nodes, with a small legend on the bottom edge:
- **`depends_on` edge** — solid 1.5px line **#4878C6** (lever blue) with a small filled-triangle arrowhead. Connects a derived doc to the upstream source it relies on.
- **`generated_by` edge** — dashed 1.5px line **#8E35A3** (load purple) with a small open-triangle arrowhead. Connects a generated artifact (image, report) back to the prompt or template that produced it.

Edges flow generally left-to-right but cross layers where needed, showing the network nature of the graph rather than a strict tree. A small subset of nodes on the right are clearly **generated** (e.g., `principles-composite.png`, `tool-tracker.png`) and have both a blue `depends_on` edge to a source doc *and* a purple dashed `generated_by` edge to a `.prompt.md` node, illustrating the dual-edge model.

Background: flat **#FFFFFF** with subtle 4%-opacity grid registration marks in **#E5E7EB**. Bottom legend strip shows two short edge samples — a solid blue line labeled "depends_on" and a dashed purple line labeled "generated_by" — in **Inter 0.75rem #6B7280**. Tone: technical reference manual, knowledge-graph schematic drawn as a patent figure, sober and utilitarian.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6** for `depends_on` edges, load purple **#8E35A3** for `generated_by` edges; neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#FFFFFF** only. Fulcrum green **#35A35F** is not used in this image.
- Typography: Inter for labels, regular/medium weight, label scale (0.75rem). JetBrains Mono permitted for filename labels if it improves legibility. No display type.
- Register: technical schematic, graph-theory diagram. Treat as a patent figure, not an illustration.
- No gradient washes, glows, lens flares, particles, dreamy lighting, or AI-art treatments.
- No anthropomorphism (no hands, no faces, no characters).
- Metaphor must parse without caption: a directed graph of documents with two distinct edge kinds (dependency vs. provenance).

# Industrial framing (preamble enforcement)

Wrap the figure in a thin **brass `#9A6A2F` cartouche** (1 px stroke, 6 px inset
from the canvas edge) with four small registration ticks at the corners. The
canvas background is blueprint paper (`#F7F1E7` light variant) overlaid with
the existing 4 % `#E5E7EB` registration grid; allow a 2 % brushed-steel vignette
at the deep margins. Place a small bottom-right registration block reading
**`DDx · FIG.T1`** in JetBrains Mono 0.625 rem `#6B7280`, and at top-left a
**`DOC-DRIVEN SOFTWARE FACTORY`** kicker in Space Grotesk small-caps 0.625 rem
`#9A6A2F` (brass), letter-spacing 0.08 em. Permit at most **two** pinpoint
amber `#C79B5B` or terminal-cyan `#7FE3D4` signal-lamp dots (≤ 6 px diameter,
matte, no halation) as instrument punctuation; never as the figure's subject.
Optional: a single faint copper rivet glyph at each cartouche corner. The
diagram itself (artifact graph — the spine of the DDx software factory) must remain the dominant read at first glance —
the industrial framing rewards the second look without competing with the
metaphor.

# Anchoring (recently-landed positioning)

Anchored to **Spec-First Development**, **Context is King**, and **Drift is Debt** (`/docs/principles/spec-first-development/`, `/docs/principles/context-is-king/`, `/docs/principles/drift-is-debt/`). The graph is the spine: `depends_on` resolves upstream context for any agent invocation, `generated_by` captures provenance for every produced artifact. When a source moves, dependents go stale and DDx files reconciliation beads.

# Negative prompt

3D render, photographic realism, gradient backgrounds, neon, glow, bokeh, depth-of-field blur, particles, sparkles, cinematic lighting, painterly illustration, hand-drawn sketch style, watercolor, isometric video-game aesthetic, robots, brains, circuit boards, cables, screens, characters, faces, hands, marketing-deck composition, file-folder icons, paper-stack illustrations, cloud icons.

# Acceptance criteria

- Reads as a graph of documents with two clearly distinct edge kinds (depends_on vs. generated_by) without explanatory caption.
- Lever blue **#4878C6** appears only on solid `depends_on` edges; load purple **#8E35A3** appears only on dashed `generated_by` edges.
- Sober/utilitarian register; no AI-gloss.
- Light/dark mode contrast OK; mobile crop preserves the metaphor.
- File size ≤ 200KB after compression.

---
title: "Tool — Agentic execution (run / try / work)"
tool: "Agentic execution"
component: "Three-layer wrapping with worktree isolation"
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
registration ticks, blueprint-grid bleed, and a small `DDx · FIG.T2`
maker's mark in the bottom-right registration block.

# Prompt

A precise technical diagram showing three nested rounded-rectangle layers, viewed straight on as an engineering schematic — concentric containers stepping inward from outermost to innermost, illustrating how `ddx work` wraps `ddx try` which wraps `ddx run`. Each layer is labeled at the top-left of its bounding rectangle in **Inter 0.75rem weight 500**, with a short subtitle in **Inter 0.75rem #6B7280** beneath:

- **Outer layer — `ddx work`** (queue drain). Stroke 1.5px **#4878C6** (lever blue), fill **#F9FAFB**. Subtitle: "drain the bead queue". On the left edge of this outer layer, a small priority-queue column shows three stacked blue rounded rectangles labeled **DDX-001**, **DDX-002**, **DDX-003**, with a thin **#4878C6** arrow exiting the queue and entering the middle layer, indicating beads are pulled one at a time.
- **Middle layer — `ddx try` (`execute-bead`)** (worktree-isolated attempt). Stroke 1.5px **#4878C6**, fill **#FFFFFF**. Subtitle: "isolated worktree • merge or discard". Inside this layer, a clearly delineated **worktree boundary** is drawn as a dashed 1.5px **#6B7280** rounded rectangle labeled `.execute-bead-wt-…` in **JetBrains Mono 0.75rem #6B7280**, visually communicating filesystem isolation. A small fork-and-merge glyph on the right edge of this layer (two thin **#4878C6** lines branching out and rejoining) represents the merge-or-discard outcome.
- **Inner layer — `ddx run`** (a single agent invocation). Stroke 1.5px **#4878C6**, fill **#FFFFFF**. Subtitle: "one agent call". Contains a small node labeled **agent** in **Inter 0.75rem #111827** with a thin **#4878C6** arrow looping into and out of it, indicating the actual model invocation.

To the right of the diagram, a thin **#E5E7EB** evidence rail shows three small stacked document glyphs (rounded rectangles, 1.5px **#6B7280** stroke) labeled `bundle/`, `result/`, `commit` in **Inter 0.75rem #6B7280**, with thin **#8E35A3** (load purple) dashed `generated_by` edges pointing back into the middle layer — indicating execution evidence is captured per-attempt.

Background: flat **#FFFFFF** with subtle 4%-opacity grid registration marks in **#E5E7EB**. Tone: technical reference manual, systems-architecture diagram drawn as a patent figure, sober and utilitarian.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6** for layer strokes and flow arrows; load purple **#8E35A3** only on `generated_by` evidence edges; neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#F9FAFB**, **#FFFFFF**. Fulcrum green **#35A35F** is not used in this image.
- Typography: Inter for layer labels and subtitles, JetBrains Mono for the worktree path and command names. Label scale (0.75rem) only. No display type.
- Register: technical schematic, systems-architecture diagram. Treat as a patent figure, not an illustration.
- No gradient washes, glows, lens flares, particles, dreamy lighting, or AI-art treatments.
- No anthropomorphism (no hands, no faces, no characters, no robot avatars for "agent").
- Metaphor must parse without caption: three nested layers with the middle layer visibly isolated as a worktree.

# Industrial framing (preamble enforcement)

Wrap the figure in a thin **brass `#9A6A2F` cartouche** (1 px stroke, 6 px inset
from the canvas edge) with four small registration ticks at the corners. The
canvas background is blueprint paper (`#F7F1E7` light variant) overlaid with
the existing 4 % `#E5E7EB` registration grid; allow a 2 % brushed-steel vignette
at the deep margins. Place a small bottom-right registration block reading
**`DDx · FIG.T2`** in JetBrains Mono 0.625 rem `#6B7280`, and at top-left a
**`DOC-DRIVEN SOFTWARE FACTORY`** kicker in Space Grotesk small-caps 0.625 rem
`#9A6A2F` (brass), letter-spacing 0.08 em. Permit at most **two** pinpoint
amber `#C79B5B` or terminal-cyan `#7FE3D4` signal-lamp dots (≤ 6 px diameter,
matte, no halation) as instrument punctuation; never as the figure's subject.
Optional: a single faint copper rivet glyph at each cartouche corner. The
diagram itself (three-layer run architecture — `ddx run` / `try` / `work`) must remain the dominant read at first glance —
the industrial framing rewards the second look without competing with the
metaphor.

# Anchoring (recently-landed positioning)

Anchored to **Bounded-context execution / Ralph loop** (vision §Physics of Generative AI, principle 6) and to **Audit Trail Required**. The middle worktree boundary is the bounded-context contract: each `ddx try` attempt runs in a fresh, narrowly-scoped context against an explicit acceptance criterion, with persistent state landing on disk as evidence rather than carried forward as transcript.

# Negative prompt

3D render, photographic realism, gradient backgrounds, neon, glow, bokeh, depth-of-field blur, particles, sparkles, cinematic lighting, painterly illustration, hand-drawn sketch style, watercolor, isometric video-game aesthetic, robots, brains, circuit boards, cables, screens, characters, faces, hands, marketing-deck composition, terminal-window mockups, command-line screenshots, chat bubbles, gear icons.

# Acceptance criteria

- Reads as three nested execution layers with `work` outermost, `try` in the middle (visibly worktree-isolated), and `run` innermost — without explanatory caption.
- Lever blue **#4878C6** dominates the layer strokes and flow; load purple **#8E35A3** appears only on evidence/`generated_by` edges; the worktree boundary is visibly distinct (dashed) from solid layer strokes.
- Sober/utilitarian register; no AI-gloss.
- Light/dark mode contrast OK; mobile crop preserves the metaphor.
- File size ≤ 200KB after compression.

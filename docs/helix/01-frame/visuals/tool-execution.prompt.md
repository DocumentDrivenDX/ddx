---
title: "Tool — Agentic execution (run / try / work)"
tool: "Agentic execution"
component: "Three-layer wrapping with worktree isolation"
generator: nano-banana-pro-openrouter
model: gemini-3-pro-image-preview
aspect_ratio: "16:9"
size: "2K"
depends_on:
  - DESIGN.md
  - docs/helix/01-frame/prd.md
---

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

# Negative prompt

3D render, photographic realism, gradient backgrounds, neon, glow, bokeh, depth-of-field blur, particles, sparkles, cinematic lighting, painterly illustration, hand-drawn sketch style, watercolor, isometric video-game aesthetic, robots, brains, circuit boards, cables, screens, characters, faces, hands, marketing-deck composition, terminal-window mockups, command-line screenshots, chat bubbles, gear icons.

# Acceptance criteria

- Reads as three nested execution layers with `work` outermost, `try` in the middle (visibly worktree-isolated), and `run` innermost — without explanatory caption.
- Lever blue **#4878C6** dominates the layer strokes and flow; load purple **#8E35A3** appears only on evidence/`generated_by` edges; the worktree boundary is visibly distinct (dashed) from solid layer strokes.
- Sober/utilitarian register; no AI-gloss.
- Light/dark mode contrast OK; mobile crop preserves the metaphor.
- File size ≤ 200KB after compression.

---
title: "User workflow — iterative loop with DDx"
lens: "User workflow (capstone)"
component: "Six-stage closed loop: artifact → graph → bead → agent → evidence → re-align"
generator: nano-banana-pro-openrouter
model: gemini-3-pro-image-preview
aspect_ratio: "16:9"
size: "2K"
depends_on:
  - .stitch/DESIGN.md
  - docs/helix/01-frame/prd.md
  - docs/helix/01-frame/visuals/tool-tracker.prompt.md
  - docs/helix/01-frame/visuals/tool-doc-graph.prompt.md
  - docs/helix/01-frame/visuals/tool-execution.prompt.md
  - docs/helix/01-frame/visuals/tool-plugins.prompt.md
---

# Prompt

A precise technical diagram of a closed iterative workflow rendered as an engineering schematic, viewed straight on. The composition is a single horizontal **16:9** canvas with a clockwise circular loop occupying the center, formed by six numbered stages connected by directional **#4878C6** (lever blue) flow arrows with small filled-triangle arrowheads. The loop reads as a complete cycle: stages flow ① → ② → ③ → ④ → ⑤ → ⑥ → back to ① with no terminal arrow, communicating that the user re-enters the loop continuously rather than finishing.

Each stage is a rounded rectangle (rounded corner radius 8px) of equal size, **#FFFFFF** fill, 1.5px **#111827** stroke, with a small numbered badge in the top-left corner — a **#4878C6** filled circle 18px diameter containing a white numeral in **Inter 0.75rem weight 600 #FFFFFF**. Inside each stage card, the stage name is set in **Inter 0.875rem weight 500 #111827** with a one-line subtitle beneath in **Inter 0.75rem #6B7280**, and a small inline glyph echoes that stage's tool from the existing tool lens (consistent shorthand, not a re-render):

- **① Author / refine artifact** — subtitle: "human edits a markdown doc". Glyph: a small document rectangle with two faint horizontal text lines, 1.5px **#111827** stroke. Stage card sits at the **9 o'clock** position. The author here is a human; the input is a `.md` file in the helix tree.
- **② Graph synthesizes context** — subtitle: "depends_on resolves upstream". Glyph: three small connected nodes with two solid **#4878C6** edges between them (a miniature document graph). Stage card sits at **11 o'clock**. The graph walks `depends_on` to assemble the artifact's effective context.
- **③ Bead created** — subtitle: "tracked work item enters the queue". Glyph: a small DAG snippet with one solid **#4878C6** node (ready) and one **#6B7280** outlined node (blocked). Stage card sits at **1 o'clock**. The bead carries an id like **DDX-042** rendered in **JetBrains Mono 0.6875rem #6B7280** beneath the glyph.
- **④ Agent runs** — subtitle: "`ddx try` / `ddx work` in worktree". Stage card sits at **3 o'clock** and is visibly the most active node: stroke 1.5px **#4878C6** instead of **#111827**, fill **#F9FAFB**. Glyph: three concentric rounded rectangles (the three-layer wrapping from the execution lens), with the innermost containing a tiny **agent** label in **Inter 0.625rem #111827**. A dashed 1.5px **#6B7280** boundary around the inner two layers indicates the worktree.
- **⑤ Evidence captured** — subtitle: "bundle, result, commit". Glyph: a small evidence rail of three stacked document rectangles with a thin **#8E35A3** (load purple) dashed `generated_by` tick on the right edge. Stage card sits at **5 o'clock**.
- **⑥ Human re-aligns** — subtitle: "review evidence, refine artifact". Stage card sits at **7 o'clock** and is rendered with a thin **#35A35F** (fulcrum green) inner accent line — a 1px stroke offset 4px inside the stage rectangle — indicating the human-AI seam closes here. Glyph: a small fulcrum triangle in **#35A35F** beneath a horizontal beam segment, echoing the principles lens. The arrow leaving stage ⑥ points back to stage ① to visibly close the loop.

A thin **#8E35A3** dashed `generated_by` edge runs from stage ⑤ inward to a small **commit** glyph at the loop's center (a tiny **#111827** stroked rounded rectangle labeled `commit` in **JetBrains Mono 0.6875rem**), and from that center glyph two thin **#6B7280** dashed lines fan back to stages ① and ② — communicating that captured evidence updates the artifacts and the graph for the next iteration. The center glyph is small (~10% of canvas height) and does not compete with the loop ring.

The bottom edge of the canvas carries a **legend strip** in **#E5E7EB** at 4% opacity background with three short swatches in **Inter 0.75rem #6B7280**:
- A solid **#4878C6** arrow labeled "flow"
- A dashed **#8E35A3** line labeled "generated_by"
- A **#35A35F** fulcrum triangle labeled "human-AI seam"

The top-left corner carries a small heading in **Inter 0.875rem weight 600 #111827** reading **"Working with DDx"** with a one-line subhead in **Inter 0.75rem #6B7280** reading **"the loop closes; the artifact improves"**. No other display type, no marketing tagline.

Background: flat **#FFFFFF** with subtle 4%-opacity grid registration marks in **#E5E7EB**. Tone: technical reference manual, systems-workflow diagram drawn as a patent figure, sober and utilitarian, explanatory register suitable for a load-bearing homepage hero or PRD figure.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6** dominates the loop arrows and ready/active accents; load purple **#8E35A3** appears only on dashed `generated_by` edges (stage ⑤ tick and the center fan-out); fulcrum green **#35A35F** appears only on stage ⑥'s inner accent line and the legend's seam glyph. Neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#F9FAFB**, **#FFFFFF** elsewhere.
- Typography: Inter for stage names, subtitles, headings, and legend; JetBrains Mono for bead ids, the `commit` label, and other code-shaped tokens. Label scale (0.75rem) for subtitles, 0.875rem for stage names and the corner heading; no display type.
- Register: technical schematic, workflow diagram. Treat as a patent figure, not an illustration. Capstone of the visual suite — must read alongside the four tool diagrams without aesthetic discontinuity.
- No gradient washes, glows, lens flares, particles, dreamy lighting, or AI-art treatments.
- No anthropomorphism (no hands, no faces, no characters, no avatars for "human" or "agent" — the human-AI seam is communicated by the fulcrum glyph and the green accent only).
- The numbered loop must read clockwise, must close (no broken or open arc), and must carry six and only six stages. No additional decorative nodes.
- Metaphor must parse without caption: a closed iterative loop with the human re-entering at the seam and evidence flowing inward to update artifacts.

# Negative prompt

3D render, photographic realism, gradient backgrounds, neon, glow, bokeh, depth-of-field blur, particles, sparkles, cinematic lighting, painterly illustration, hand-drawn sketch style, watercolor, isometric video-game aesthetic, robots, brains, circuit boards, cables, screens, characters, faces, hands, body parts, marketing-deck composition, terminal-window mockups, command-line screenshots, chat bubbles, speech balloons, gears, infinity symbols, ouroboros snake, broken or open arcs that fail to close the loop, more than six numbered stages, fewer than six numbered stages, additional palette colors beyond the named tokens.

# Acceptance criteria

- Reads as a closed six-stage clockwise loop — author → graph → bead → agent → evidence → re-align → back to author — without explanatory caption.
- Stage ④ (agent runs) is visibly the most active node and shows the three-layer wrapping plus a worktree boundary echoing the execution lens.
- Stage ⑥ (human re-aligns) carries the fulcrum-green seam accent; no other stage uses fulcrum green.
- Lever blue **#4878C6** dominates the loop's flow arrows; load purple **#8E35A3** appears only on `generated_by` evidence edges; fulcrum green **#35A35F** appears only at the human-AI seam.
- Capstone reads as part of the visual suite — palette, line weights, label scale, and corner registration match the four tool diagrams (`tool-tracker`, `tool-doc-graph`, `tool-execution`, `tool-plugins`).
- Sober/utilitarian register; no AI-gloss, no kitsch.
- Light/dark mode contrast OK; mobile crop preserves the loop's closure and stage numbering.
- File size ≤ 200KB after compression.

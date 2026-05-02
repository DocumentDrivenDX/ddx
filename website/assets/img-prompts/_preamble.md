# DDx Visual Preamble — Industrial Factory / Cyberpunk-Steampunk Schematic

Apply this preamble to every DDx hero image prompt. It establishes a unified
aesthetic that frames DDx as a **document-driven software factory** — humans
and agents working a shared production line of artifacts, beads, and evidence.

---

## Aesthetic

Render the figure as a **patent-grade engineering schematic on a factory
blueprint** — a precise side-on or straight-on technical diagram, drafted with
the discipline of a 19th-century mechanical drawing but tinted with the
restrained accents of a near-future control room. The mood is a quiet
**software factory**: assembly lines, conveyor mechanics, brass instrumentation,
sub-panel telemetry, and dim signal lamps. Slightly **steampunk** in materiality
(brass fittings, copper rivets, machined bezels, valve-and-gear texture
*at the edges*, never as the subject); slightly **cyberpunk** in lighting
(thin terminal-cyan / neon-amber accents, faint holographic data overlays,
glass-and-metal surfaces). Tasteful — never theme-park. The figure itself
remains a clean, legible diagram; the industrial framing lives at the chrome:
corner cartouches, registration ticks, blueprint-grid bleed, faint
brass border seams, a small embossed "DDx" maker's mark in the corner.

## Palette (cohere with `website/DESIGN.md`)

Anchor every image to the DDx tokens, then layer the industrial accents:

- **Lever blue `#4878C6` / `#3B5B7A`** — primary structural lines, beams,
  flow arrows, active-state fills.
- **Fulcrum green `#35A35F`** — the human-AI seam, completed work, pivot
  surfaces.
- **Load purple `#8E35A3` / `#7A4E9F`** — probabilistic mass, generated_by
  edges only.
- **Brass `#9A6A2F` / `#C79B5B`** — restrained warm accent on framing chrome,
  rivets, instrument bezels, kicker labels (steampunk register).
- **Neutrals `#111827`, `#6B7280`, `#E5E7EB`, `#F9FAFB`, `#FFFFFF`** —
  inks, dividers, fills.
- **Optional cyberpunk accent: terminal cyan `#7FE3D4`** at ≤ 8% opacity for
  faint holographic overlays, signal-lamp glow, or readout text — never as a
  primary color.
- Background canvas: blueprint paper (`#F7F1E7` warm light, or `#0F1117`
  near-black for dark variants) with 4 % `#E5E7EB` registration grid and a
  subtle 2 % vignette of brushed-steel texture at the deep margins.

## Composition & lighting

- Straight-on or strict side elevation. No three-quarter perspective, no
  isometric video-game tilt.
- Crisp 1.5 px line work; matte fills; no glow, no bokeh, no lens flare.
- Lighting: even diffuse top-light, with one or two pinpoint amber/cyan
  signal lamps allowed as small punctuation (≤ 6 px diameter, no halation).
- Typography: **Inter** for diagram labels (regular/medium, label scale
  0.75 rem); **JetBrains Mono** for code-shaped tokens (bead ids, paths,
  hashes); **Space Grotesk** small-caps permitted on chrome cartouches.
- Frame the diagram inside a thin brass-edged cartouche at the canvas
  perimeter (1 px stroke `#9A6A2F`, 6 px inset). Bottom-right corner carries
  a small registration block: `DDx · FIG.<n>` in JetBrains Mono 0.625 rem
  `#6B7280`.

## Subject discipline

The diagram itself is the subject — the industrial frame is chrome, not
content. Each prompt's specific mechanism (lever, DAG, nested layers, loop,
plugin sockets, doc graph) must remain the dominant read at first glance.
The factory framing should reward the second look without competing with the
metaphor.

## Negative constraints (apply to every image)

3D render, photographic realism, gradient sky, painterly illustration,
watercolor, hand-drawn sketch, dreamy or cinematic lighting, neon glow as the
subject, depth-of-field blur, sparkles, particles-as-magic, robots, brains,
faces, hands, characters, mascots, marketing-deck composition, infographic
icon clusters, kanban boards, sticky notes, terminal-window mockups, gear
icons used as decoration, ouroboros snake, infinity symbol, USB-plug
photography, lego bricks, jigsaw puzzles, app-store icon grids,
handshake imagery, anthropomorphized agents, raygun-gothic kitsch,
heavy-handed steampunk costume, theme-park brass overload.

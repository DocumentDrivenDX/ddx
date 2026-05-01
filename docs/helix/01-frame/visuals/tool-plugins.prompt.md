---
title: "Tool — Plugins"
tool: "Plugins"
component: "Modular composition; HELIX/Dun snapping into shared core"
generator: nano-banana-pro-openrouter
model: gemini-3-pro-image-preview
aspect_ratio: "16:9"
size: "2K"
depends_on:
  - .stitch/DESIGN.md
  - docs/helix/01-frame/prd.md
---

# Prompt

A precise technical diagram of a modular plugin system rendered as an engineering schematic, viewed straight on. At the center sits a single large rounded-rectangle labeled **DDx core** in **Inter 1rem weight 600 #111827**, fill **#F9FAFB**, 1.5px stroke **#4878C6** (lever blue). The core has four small protruding **socket** notches along its perimeter (top, right, bottom, left) — each notch drawn as a clean trapezoidal cutout with 1.5px **#4878C6** stroke, labeled in **Inter 0.75rem #6B7280** with the capability the socket exposes: `library`, `beads`, `agents`, `personas`.

Around the core, four plugin modules are arranged at the cardinal positions, each drawn as a smaller rounded rectangle with a matching trapezoidal **tab** that visually snaps into the corresponding core socket (the geometry must read as plug-and-socket, not as floating rectangles). Each plugin is labeled in **Inter 0.875rem weight 600 #111827** with a one-line subtitle in **Inter 0.75rem #6B7280**:

- **HELIX** (top, snapping into `library`/`beads`). Subtitle: "phased workflow". Fill **#FFFFFF**, 1.5px stroke **#4878C6**.
- **Dun** (right, snapping into `agents`). Subtitle: "quality check runner". Fill **#FFFFFF**, 1.5px stroke **#4878C6**.
- **Plugin C** (bottom). Generic third-party plugin; label simply `your-plugin` in **JetBrains Mono 0.75rem #6B7280**, with a thin dashed 1.5px **#6B7280** stroke (indicating "extension point — bring your own"). Fill **#FFFFFF**.
- **Plugin D** (left). Same generic treatment as Plugin C, label `another-plugin`.

Two of the four plugins (HELIX, Dun) are fully snapped into their sockets with crisp solid strokes; the other two are drawn slightly **detached** from the core with a small gap between tab and socket and dashed strokes, visually communicating "optional / pluggable" without text.

Background: flat **#FFFFFF** with subtle 4%-opacity grid registration marks in **#E5E7EB**. A thin **#E5E7EB** legend strip along the bottom shows two short examples — a solid trapezoid labeled "installed plugin" and a dashed trapezoid labeled "available extension point" — in **Inter 0.75rem #6B7280**. Tone: technical reference manual, mechanical-assembly diagram drawn as a patent figure, sober and utilitarian.

# Style constraints (DESIGN.md)

- Palette: lever blue **#4878C6** for the core stroke and installed-plugin strokes; neutrals **#111827**, **#6B7280**, **#E5E7EB**, **#F9FAFB**, **#FFFFFF**. Fulcrum green and load purple are not used in this image.
- Typography: Inter for component labels and subtitles, JetBrains Mono for generic plugin slugs. No display type.
- Register: technical schematic, mechanical-assembly diagram. Treat as a patent figure, not an illustration.
- No gradient washes, glows, lens flares, particles, dreamy lighting, or AI-art treatments.
- No anthropomorphism (no hands, no faces, no characters, no mascots).
- Metaphor must parse without caption: a shared core with named sockets and modular plugins that visibly snap in.

# Negative prompt

3D render, photographic realism, gradient backgrounds, neon, glow, bokeh, depth-of-field blur, particles, sparkles, cinematic lighting, painterly illustration, hand-drawn sketch style, watercolor, isometric video-game aesthetic, robots, brains, circuit boards, cables, screens, characters, faces, hands, marketing-deck composition, jigsaw-puzzle pieces, USB-plug photography, lego-brick illustrations, app-store icon grids.

# Acceptance criteria

- Reads as a shared DDx core with named capability sockets and four plugins (two snapped in, two available) without explanatory caption.
- Lever blue **#4878C6** dominates installed strokes; dashed **#6B7280** clearly distinguishes optional/available plugins from snapped-in ones.
- Sober/utilitarian register; no AI-gloss.
- Light/dark mode contrast OK; mobile crop preserves the metaphor.
- File size ≤ 200KB after compression.

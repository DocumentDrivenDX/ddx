# Hero graphic — blocked

The microsite landing hero (AC#3 of bead `ddx-5028c8e6`) was not generated
because `OPENROUTER_API_KEY` was not available at execution time and the
`nano-banana-pro-openrouter` skill was not installed in the runtime
environment. Per the bead's AC#3 contingency, the SVG/Mermaid diagrams
(AC#1, AC#2) shipped in this attempt and the hero portion is deferred with
this blocker note + a ready integration point.

## To finish the hero in a follow-up

1. Set `OPENROUTER_API_KEY` in your shell (or this project's `.env`).
2. Install the skill: `ddx install nano-banana-pro-openrouter` (or invoke
   it via the Claude skill harness if running inside Claude Code).
3. Use the prompt below to generate variants (≥ 2K, pick the strongest).
4. Save the chosen image at `website/static/hero/landing.webp` (preferred
   for size) or `website/static/hero/landing.png`.
5. In `website/content/_index.md`, uncomment the
   `<!-- HERO_GRAPHIC_PLACEHOLDER ... -->` block (search for that marker).
6. Run `cd website && hugo --minify` to verify the build, then commit.

## Generation prompt

> A distinctive, editorial-style illustration of document-driven
> development: a structured tree of documents — specs, prompts, personas,
> patterns, templates rendered as crisp paper cards with subtle markdown
> typography — flowing along clean directional channels into a small
> cluster of agent figures (abstract geometric, not anthropomorphic
> robots), which in turn emit code, tests, and build artifacts as a
> secondary stream. Composition: documents-on-the-left, agents-in-the-
> middle, artifacts-on-the-right; strong horizontal flow lines suggest
> "documents drive the agents."
>
> Style: precise vector-feel illustration with soft gradients, muted
> editorial palette (deep indigo, warm parchment, signal teal accents,
> single warm-orange highlight on the active document), subtle paper
> grain. Avoid: glowing neural-net spheres, glowing circuit boards,
> humanoid robots, generic "AI brain" iconography, neon cyan/magenta
> gradients, glassmorphism, generic SaaS-hero crowd shots. The image
> should read as a calm engineering diagram crossed with a New Yorker
> spot illustration — distinctive, not generic AI.
>
> Aspect ratio: 16:9 (or 21:9 for wide hero). Resolution: ≥ 2048 px on
> the long edge. No text, no watermark.

## Why the contingency, not a stub

The bead AC explicitly allows AC#3 to be replaced by a documented blocker
+ a ready integration point when the key/skill are unavailable. Shipping a
placeholder PNG or generic stock art would violate AC#5 ("no generic AI
clip art").

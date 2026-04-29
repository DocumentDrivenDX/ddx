# Landing hero — blocked: OPENROUTER_API_KEY unavailable

This bead (ddx-5028c8e6, V3 of epic ddx-53c96967) was instructed to generate
a polished hero graphic for the microsite landing page using the
`nano-banana-pro-openrouter` skill. That skill calls OpenRouter and requires
`OPENROUTER_API_KEY` in the environment.

At execution time:
- `OPENROUTER_API_KEY` was not set in the environment or in any `.env` the
  worktree could read.
- The `nano-banana-pro-openrouter` skill was not installed in `~/.claude/skills/`
  or `.claude/skills/`.

Per the bead's AC #3 contingency clause, the hero portion is declined with this
documented blocker note. The SVG diagrams (AC #1, #2) are committed; the
landing-page integration point is wired and ready for the image (AC #5 — the
hero, when generated, must be distinctive, not generic AI clip art).

## Prompt for nano-banana-pro-openrouter

When the API key is available, run the skill with this prompt:

> A flat, editorial-illustration-style hero image for "DDx — the platform for
> document-driven development." Visual metaphor: a structured tree of crisp
> Markdown documents (headings, bullets, code fences visible) flowing as a
> directed graph into a constellation of small autonomous agents that emit
> green-checkmark commits on the right. The documents are the source of
> truth; the agents are downstream. Avoid: glowing brains, robot heads,
> generic blue circuit boards, neural-net swirls, "AI" in any glow effect.
> Aesthetic: muted teal/green/amber palette on off-white, thin 1.5px line
> work, occasional flat-color fills, subtle paper grain, generous negative
> space. Composition: left-to-right reading order, documents on the left,
> agents on the right, the graph edges labelled with tiny tag chips
> ("persona", "spec", "pattern"). 2K resolution minimum, 16:9, no text
> rendered into the image (we'll set the headline in HTML).

Generate 3–4 variants; pick the strongest by these criteria:
1. Documents are unambiguously the **source**, agents the **consumer**.
2. No generic AI imagery (brains, robots, glowing nodes).
3. Reads as editorial illustration, not stock-photo collage.
4. Works on both light and dark backgrounds (or supply two variants).

## Integration point

Save the chosen variant as one of:
- `website/static/hero/landing.webp` (preferred — smaller)
- `website/static/hero/landing.png` (fallback)

Then wire it into the landing hero by editing `website/content/_index.md`.
A ready-to-uncomment block has been left there (search for
`HERO_GRAPHIC_PLACEHOLDER`). Uncomment it once the asset exists.

## Follow-up

Reopen ddx-5028c8e6 (or open a follow-up bead) once `OPENROUTER_API_KEY` is
available in the execution environment. The diagrams below remain valid:

- `website/static/diagrams/three-layer-stack.svg`
- `website/static/diagrams/bead-lifecycle.svg`
- `website/static/diagrams/execute-loop.svg`
- `website/static/diagrams/persona-binding.svg`
- `website/static/diagrams/project-local-install.svg`

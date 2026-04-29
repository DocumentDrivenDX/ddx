# Visual Suite — Principles, Tools, User Workflow

Date: 2026-04-29
Status: Draft (pending review)

## Summary

DDx ships a coordinated graphical suite covering three lenses on the product:
**principles**, **tools**, and **user workflow**. Each lens gets per-element
graphics plus a composite/collage; the user-workflow lens is the capstone
that ties everything together. All visuals are generated artifacts (per
the artifact-and-run-architecture plan), checked in, with sidecar metadata
and `generated_by` provenance.

Visual style is established via Google's DESIGN.md skill (or equivalent) —
identified, integrated into DDx, and used to drive a DDx-specific visual
style guide before any prompts are authored.

The website redesign (separately scoped) becomes a downstream consumer of
this work.

## Architecture

```
DESIGN.md skill
      ↓
DDx visual style guide
      ↓
┌────────────────────────────────┐
│ Principle graphics (×6)        │
│   ↓                            │
│ Principle composite            │
├────────────────────────────────┤
│ Tool graphics (×4)             │
│   ↓                            │
│ Tool composite                 │
├────────────────────────────────┤
│ User-workflow diagram (capstone)
└────────────────────────────────┘
      ↓
Website redesign (beads 28, 29) consumes
```

## The three lenses

### Lens 1 — Principles (×6 + composite)

One graphic per thesis principle. The lever/load/fulcrum metaphor is
load-bearing: each principle has a natural place in a leverage machine.

| Principle | Visual concept |
|---|---|
| 1. Abstraction is the lever | The lever arm — multi-level stack (vision/spec/test/code) with intent transmitting through |
| 2. Software is iteration over tracked work | Cyclic motion of the lever — discrete tracked items moving through |
| 3. Methodology is plural | Multiple lever handles / interchangeable grips on the same arm |
| 4. LLMs are stochastic, unreliable, costly | The load — non-uniform, probabilistic mass |
| 5. Evidence provides memory | Trail/chain of receipts left by the moving lever |
| 6. Human-AI collaboration is the fulcrum | The pivot — humans on one side, AI on the other, contact point in the middle |

The principle composite assembles all six into one leverage machine — the
literal rendering of the metaphor introduced in the Core Thesis preamble.

### Lens 2 — Tools (×4 + composite)

One graphic per major DDx capability:

| Tool | Visual concept |
|---|---|
| Bead tracker | DAG with priority queue + ready/blocked states |
| Document graph (artifacts) | Multi-node graph with `depends_on` and `generated_by` edges |
| Agentic execution (run/try/work) | Three-layer wrapping: invocation atom inside worktree-isolated bead attempt inside queue drain |
| Plugins | Modular composition; attachment points; HELIX/Dun/etc. snapping into a shared core |

The tool composite shows the stack assembled — how the four tools sit in
relation to each other within DDx.

### Lens 3 — User workflow (the capstone)

A circular/iterative diagram showing how a user actually works with DDx
over time:

1. Author or refine an artifact (vision, spec, design)
2. Artifact graph synthesizes context
3. Bead created from a gap or work item
4. Agent runs (`ddx try` / `ddx work`); produces output + side effects
5. Evidence captured; review / verify
6. Human re-aligns: amend artifacts, adjust constraints, re-bead
7. Loop closes; iterate at the appropriate level

This diagram is the coup de grace — it ties principles + tools + the
human-AI seam into a single picture of how the system *actually gets used*.

## DESIGN.md integration

**Open question:** what is "DESIGN.md" exactly? Candidates:
- A specific Anthropic/Google skill (find via `find-skills` or web search)
- A pattern-doc convention (Google internal — DESIGN.md as a brief format)
- Both — a doc convention paired with a skill that operates on it

This needs a discovery step before the rest of the plan can proceed. Bead
listed below.

Once identified:
1. Install / integrate into DDx skill library at `library/.agents/skills/<design-skill>/` (or path determined by FEAT-011).
2. Use the skill to author DDx's visual style brief (palette, typography
   if any, line-weight conventions, composition rules, metaphor system,
   tone — sober/utilitarian per existing site precedent).
3. Style brief lives at `assets/visuals/STYLE.md` (repo-only; not shipped
   to user projects per CLAUDE.md project-local install conventions).

## Generation strategy

- **Generator:** nano-banana (`nano-banana-pro-openrouter`) by default.
  Hero/composite work pressure-tested first; iconography may shift to
  hand-authored SVG if cross-prompt consistency proves unworkable.
- **Each visual is a `generate-artifact` run** per the artifact-and-run
  plan — sidecar `.ddx.yaml` with prompt, generator, last-generated-from-hash;
  source-hash staleness rule; checked into git.
- **`SYSTEM_TEMPLATE` override:** the default template pulls toward "dreamy
  illustration" (codex flag from the previous review). Override with
  DDx-specific style brief from the style guide.
- **First real dogfood** of the artifact infrastructure — bugs uncovered
  here feed back into beads `ddx-d5e71fb3` (FEAT-010 refactor),
  `ddx-5d92b873` (FEAT-007 generated_by edge), and the `ddx artifact regenerate`
  CLI implementation.

## Storage layout

```
assets/visuals/
├── STYLE.md                                  ← visual style brief (DESIGN.md output)
├── principles/
│   ├── 01-lever.prompt.md                    ← prompt source artifact
│   ├── 01-lever.png                          ← rendered (checked in)
│   ├── 01-lever.png.ddx.yaml                 ← sidecar (media_type, generated_by, hashes)
│   ├── ... (×6 principles)
│   └── composite.{prompt.md,png,png.ddx.yaml}
├── tools/
│   ├── tracker.{prompt.md,png,png.ddx.yaml}
│   ├── doc-graph.{prompt.md,png,png.ddx.yaml}
│   ├── execution.{prompt.md,png,png.ddx.yaml}
│   ├── plugins.{prompt.md,png,png.ddx.yaml}
│   └── composite.{prompt.md,png,png.ddx.yaml}
└── workflow/
    └── user-experience.{prompt.md,png,png.ddx.yaml}
```

## Sequencing

Strict ordering for the upper portion (style work gates everything):

1. Identify DESIGN.md skill (research)
2. Install/integrate DESIGN.md skill into DDx
3. Author DDx visual style brief at `assets/visuals/STYLE.md` using the skill

Once style is locked, **principles**, **tools**, and **workflow** lenses can
proceed in parallel. Within each lens, per-element graphics precede the
composite.

Website redesign (beads `ddx-...` 28, 29 to be created) becomes downstream:
- Principle prose can land without visuals (text-first per prior decision)
- Visual integration in website / README waits for this plan to complete

## Bead breakdown

### Discovery + style (sequential — gates everything below)

- **V1.** Identify DESIGN.md — search `find-skills`, web research; output: skill name, install command, intended use. Small.
- **V2.** Install/integrate DESIGN.md into DDx library skills. Depends on V1.
- **V3.** Author DDx visual style brief at `assets/visuals/STYLE.md` using DESIGN.md. Captures existing precedent (`logo.svg`, color triad rgba(72,120,198) / rgba(53,163,95) / rgba(142,53,163), ASCII-diagram aesthetic, terminal-demo tone). Depends on V2.

### Principles lens (parallel, after V3)

- **V4.** Author 6 principle-graphic prompts (`assets/visuals/principles/0N-*.prompt.md`).
- **V5.** Generate 6 principle graphics. Pressure-test the lever metaphor with the first render before committing to the rest.
- **V6.** Author principle composite prompt + generate.

### Tools lens (parallel, after V3)

- **V7.** Author 4 tool-graphic prompts.
- **V8.** Generate 4 tool graphics.
- **V9.** Author tool composite prompt + generate.

### Workflow lens (capstone — after V5/V6 and V8/V9)

- **V10.** Author user-experience workflow diagram prompt + generate.

### Website integration (downstream, post-V10)

- Existing planned beads 28 (website refactor) and 29 (README refactor) gain dependency on V5 (principle visuals) and V6 (composite); the workflow diagram lands as a hero on Why DDx page or the homepage depending on prominence call.
- Existing principle-text in vision/PRD still ships text-first; doesn't wait on visuals.

## Risks and unknowns

- **DESIGN.md identity** — discovery step. Plan has a circular gap if no such skill exists; fallback would be authoring the style brief from first principles using existing DDx precedent. Bead V1 must report back before V2/V3 lock in.
- **Cross-prompt style consistency** — known weakness of `gemini-3-pro-image-preview`. Style brief must be specific enough to anchor consistency. If it doesn't, fall back to hand-authored SVG for some lenses (likely the per-principle icons; composites and capstone would still benefit from AI generation).
- **Metaphor strain when rendered** — pressure-test in V5 before committing the full suite. If lever/fulcrum/load doesn't render usefully, we revise the visual concept (not the principle wording — the principles stand on their own).
- **Artifact infra bugs** — generating these visuals exercises sidecar I/O, `generated_by` edges, and `ddx artifact regenerate`. Bugs surfaced here feed back into the artifact-and-run-architecture beads. Don't proceed past V5 if infra isn't stable enough to regenerate from prompt edits.
- **Scope creep** — 11+ visuals is a lot. If timelines compress, prioritize: principle composite (the metaphor renders; high-leverage), then user-workflow capstone (the coup de grace), then per-principle icons, then tool diagrams. Per-element principle graphics are nice-to-have if the composite reads well alone.

## Open questions for review

1. **DESIGN.md identification** — is this a known Anthropic/Google skill the user has used before, or does V1 actually need open-ended discovery?
2. **Per-principle vs composite-only** — do all 6 principle graphics ship, or does the composite alone carry the principles section if it reads well?
3. **Per-tool vs composite-only** — same question for tools.
4. **Workflow diagram style** — illustration (atmospheric) or diagram (explanatory)? This is the capstone; explanatory is probably right since it has to be readable. But it's the natural place for the most ambitious visual.
5. **Storage path** — `assets/visuals/` (repo-only) confirmed, or should some subset ship to user projects via `library/`?
6. **Sequencing** — strict (V1→V2→V3 gates everything) or can prompt drafting (V4, V7) run in parallel with style work to save calendar time, accepting some rework?
7. **Bead V1 fallback** — if no DESIGN.md skill exists, do we fall back to authoring the style brief from scratch, or pause and source a different design-support tool?

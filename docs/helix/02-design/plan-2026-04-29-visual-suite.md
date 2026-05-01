# Visual Suite — Principles, Tools, User Workflow

Date: 2026-04-29
Status: Draft v6 (locked; reviewers' Q1–Q6 resolved)

> **Note (2026-05-01):** the original plan placed DESIGN.md at the repo root.
> The canonical location is now `.stitch/DESIGN.md`. References below to
> `DESIGN.md` / `/DESIGN.md` / "repo root" should be read as `.stitch/DESIGN.md`.

## Summary

DDx ships a coordinated graphical suite covering three lenses on the product:
**principles**, **tools**, and **user workflow**. Each lens gets per-element
prompts plus a composite; the user-workflow lens is the capstone.

Visual style is established via Google's **DESIGN.md** format spec
(github.com/google-labs-code/design.md) — markdown + YAML frontmatter
describing tokens (colors, typography, spacing, components) plus prose
rationale. Comes with a `@google/design.md` npm CLI for `lint`, `diff`,
`export` (Tailwind/DTCG), and `spec`. **Status: alpha, version `0.1.0`.**

Project standardizes on **bun/bunx** for Node-ecosystem tooling, matching
existing precedent (the SvelteKit frontend at `cli/internal/server/frontend/`
already uses Bun per CLAUDE.md). `package.json` is the canonical declaration
surface for npm-delivered tools — no DDx wrapper, no FEAT-024 spec, no
DDx-owned lockfile.

All visuals are generated artifacts (per the artifact-and-run-architecture
plan), live in the helix phase tree alongside the docs they depict, with
sidecar metadata and `generated_by` provenance.

## Architecture

```
DESIGN.md authored at repo root
      ↓
DESIGN.md → website palette via Hextra theme overrides (spike — see V3.5)
DESIGN.md tokens → image-generation prompts (palette anchors)
      ↓
Visual generation (one at a time, $10 cap, pressure-test gated)
      ↓
┌────────────────────────────────┐
│ Principle prompts (×6)         │
│   ↓                            │
│ Principle composite (capstone) │
├────────────────────────────────┤
│ Tool prompts (×4)              │
│   ↓                            │
│ Tool graphics + composite      │
├────────────────────────────────┤
│ User-workflow diagram (capstone)
└────────────────────────────────┘
      ↓
Website redesign (beads to be created — 28, 29) consumes
```

## The three lenses

### Lens 1 — Principles (×6 prompts; composite ships; standalone optional)

One prompt per thesis principle. Lever/load/fulcrum metaphor is load-bearing.

| Principle | Visual concept |
|---|---|
| 1. Abstraction is the lever | The lever arm — multi-level stack with intent transmitting |
| 2. Software is iteration over tracked work | Cyclic motion of the lever — discrete tracked items |
| 3. Methodology is plural | Multiple lever handles / interchangeable grips |
| 4. LLMs are stochastic, unreliable, costly | The load — non-uniform, probabilistic mass |
| 5. Evidence provides memory | Trail / chain of receipts left by the moving lever |
| 6. Human-AI collaboration is the fulcrum | The pivot — humans and AI in contact at the seam |

All 6 prompts authored regardless of render decision. Composite ships first
(pressure-test the metaphor); standalone renders are optional follow-up if
the composite reads well alone.

### Lens 2 — Tools (×4 + composite — full per-element + composite)

| Tool | Visual concept |
|---|---|
| Bead tracker | DAG with priority queue + ready/blocked states |
| Document graph (artifacts) | Multi-node graph with `depends_on` and `generated_by` edges |
| Agentic execution (run/try/work) | Three-layer wrapping; worktree isolation visible |
| Plugins | Modular composition; HELIX/Dun snapping into shared core |

### Lens 3 — User workflow (capstone)

Iterative diagram showing how a user actually works with DDx over time:
author/refine artifact → graph synthesizes context → bead created → agent
runs (`ddx try` / `ddx work`) → evidence captured → human re-aligns → loop
closes. Explanatory register; load-bearing artifact.

## Storage — helix phase tree

Visuals live alongside the docs they depict. DESIGN.md is the cross-cutting
exception at repo root.

```
DESIGN.md                                          ← cross-cutting; repo root
package.json                                       ← repo root (bun-managed)
bun.lock                                           ← repo root (text lockfile)
docs/helix/00-discover/
├── product-vision.md
└── visuals/
    ├── principle-1-lever.{prompt.md,png,png.ddx.yaml}
    ├── ... (×6 principles)
    └── principles-composite.{prompt.md,png,png.ddx.yaml}
docs/helix/01-frame/
├── prd.md
└── visuals/
    ├── tool-tracker.{prompt.md,png,png.ddx.yaml}
    ├── tool-doc-graph.{prompt.md,png,png.ddx.yaml}
    ├── tool-execution.{prompt.md,png,png.ddx.yaml}
    ├── tool-plugins.{prompt.md,png,png.ddx.yaml}
    ├── tools-composite.{prompt.md,png,png.ddx.yaml}
    └── user-workflow.{prompt.md,png,png.ddx.yaml}
```

Each visual artifact has `depends_on` pointing to the source doc and
`generated_by` pointing to its prompt source. First real test of
`generated_by` edges across the helix tree.

## DESIGN.md is the visual style brief

DESIGN.md *is* the style brief — its prose sections (Overview, Do's/Don'ts)
carry metaphor system and composition rules; its YAML carries tokens. No
separate STYLE.md needed.

Two consumers:
- **Website (Hugo + Hextra):** spike palette injection via Hextra's
  `params.theme` / CSS variables (V3.5). Hextra ships precompiled CSS via
  Hugo Pipes; project-level Tailwind config consumption is unverified.
  Fall back to accept-divergence (DESIGN.md drives prompts only) if
  Hextra reach is insufficient.
- **Image prompts:** prompts reference DESIGN.md palette by hex and named
  tokens for cross-prompt consistency anchor.

Repo placement: `/DESIGN.md` (brand-level artifact for the whole product).

## Tool dependency declaration: `package.json` (no DDx wrapper)

Standard Node convention. No FEAT-024 spec, no DDx-owned lockfile, no
`ddx tool run` wrapper.

```bash
bun add --dev @google/design.md@0.1.0      # adds to package.json + bun.lock
bunx @google/design.md lint DESIGN.md      # standard invocation
```

`ddx doctor` extension scans known `package.json` locations (root,
`website/`, `cli/internal/server/frontend/`) and reports `bun install` if
`node_modules/` is missing or out of date. `ddx init` summary mentions
discovered `package.json` files.

DDx never wraps `bunx`; users invoke npm-ecosystem tools directly via
standard `bunx <pkg>` commands.

## Generation strategy

- **Generator:** `nano-banana-pro-openrouter` (Gemini 3 Pro Image preview)
  by default.
- **Each visual is a `generate-artifact` run** per the artifact-and-run plan.
- **`SYSTEM_TEMPLATE` override** for sober/utilitarian style (default
  pulls toward "dreamy illustration").
- **DESIGN.md tokens injected** into every prompt for palette/typography
  consistency.
- **Sequential rendering with $10 total budget cap and $1.50/image halt
  trigger.** Render one visual at a time; evaluate against acceptance
  criteria before generating the next. Halt on either cap. Per-image
  cap prevents one runaway prompt eating the budget.
- **First real dogfood** of the artifact infrastructure (sidecar I/O,
  `generated_by` edges, `ddx artifact regenerate` CLI). Bugs surfaced here
  feed back into beads `ddx-d5e71fb3` (FEAT-010), `ddx-5d92b873` (FEAT-007),
  and the artifact-regenerate implementation.
- **Regen policy:** every visual `depends_on` DESIGN.md and its prompt
  source. Either-source change marks the visual stale via the separate
  `generated_by` staleness rule. Regen is explicit (`ddx artifact regenerate`),
  not automatic — operator decides when to spend tokens.

## Acceptance criteria per visual

- Renders cleanly at intended placement (homepage hero, docs page, README)
- Contrast OK in light and dark mode
- Mobile crop preserves the message
- Alt text drafted and in sidecar
- File size budget under 200KB (compress / downsize as needed)
- Metaphor parses without explanatory copy
- **Brand-fit: sober/utilitarian register matching DDx's docs surface; no
  AI-gloss, no kitsch, no dreamy illustration**

## Risks

- **DESIGN.md is alpha** (`0.1.0`). Pin exact `0.1.0`; budget for both
  format spec AND `--format tailwind` exporter breaking on bumps (two
  alpha surfaces stack).
- **Hextra Tailwind path unverified.** V3.5 is a spike. Expected outcome:
  palette override via Hextra theme params works; if not, fall back to
  accept-divergence rather than fork Hextra.
- **Cross-prompt style consistency** on `gemini-3-pro-image-preview` is
  unproven. DESIGN.md tokens anchor palette but composition consistency
  across 6+ subsections is genuine risk. Composite-first sequencing
  (V6 before V7) limits exposure.
- **Metaphor strain when rendered.** Pressure-test in V6 before committing
  further. If lever/fulcrum/load doesn't carry, revise the visual concept
  (not principle wording).
- **Artifact-infra bugs** could block visual generation. Plan accepts
  iteration: bugs → fixes in artifact-and-run beads → resume.
- **Cost overrun.** $10 cap is for the full run; halt at cap and reassess.
  Sequential generation gives visibility; cache-on-prompt-hash prevents
  accidental re-spending.
- **Two alpha surfaces.** DESIGN.md format and exporter both alpha.
  Pinning `0.1.0` exactly mitigates but doesn't eliminate risk; budget
  ~half a day per bump for migration.

## Bead breakdown

### Tooling foundation (gates the rest)

- **V2.** Extend `ddx doctor` (and optionally `ddx init` summary) to detect
  `package.json` at known locations (root, `website/`, `cli/internal/server/frontend/`)
  and report `bun install` if `node_modules/` is missing or stale. ~100 LoC.

### DESIGN.md integration

- **V3.** Author `DESIGN.md` at repo root. Capture existing precedent
  (logo, current rgba triad, terminal-demo tone). Tokens (colors,
  typography, spacing, components); prose Overview, Do's/Don'ts, metaphor
  system. Run `bun add --dev @google/design.md@0.1.0` at repo root; lint
  with `bunx @google/design.md lint DESIGN.md`. **Treat as foundation,
  not throwaway spike** — DDx has no visual language today; embedding
  DESIGN.md is correct. Adapt as the format evolves.
- **V3.5.** Wire DESIGN.md → website (Hugo + Hextra) palette. Try Hextra
  `params.theme` / CSS-variable override path first. Fall back to
  accept-divergence (DESIGN.md drives image prompts only) if Hextra
  reach insufficient. Document outcome in bead resolution.
- **V3.6.** Wire DESIGN.md → frontend (SvelteKit + Tailwind v4) at
  `cli/internal/server/frontend/tailwind.config.js`. Use
  `bunx @google/design.md export --format tailwind` to produce a
  theme JSON the frontend's tailwind.config.js consumes. Easier path
  than Hextra (frontend already uses Tailwind); ensures DESIGN.md ROI
  even if V3.5 falls back to accept-divergence. **Goal: align on a new
  visual language across the entire product surface.**
- **V4.** Lefthook + CI lint integration: `bunx @google/design.md lint
  DESIGN.md` runs on commit if DESIGN.md is modified.

### Principles lens

- **V5.** Author all 6 principle-graphic prompts at
  `docs/helix/00-discover/visuals/principle-N-*.prompt.md`.
- **V6.** Author principle-composite prompt + generate **(first render —
  pressure-test gate; budget tracking starts here)**. If the lever/fulcrum/
  load metaphor doesn't render usefully, pause and revise before proceeding.
  **Hard-blocked on artifact infrastructure**: beads `ddx-d5e71fb3` (FEAT-010
  three-layer substrate), `ddx-5d92b873` (FEAT-007 sidecar + generated_by),
  and `ddx artifact regenerate` CLI implementation must be merged first.
  No manual prompt-file → image fallback; if those slip, V6 slips.
- **V7** *(optional, conditional on V6 outcome).* Render the 6 principle
  subsections standalone. Skip if composite suffices or if cross-prompt
  consistency fails.

### Tools lens

- **V8.** Author 4 tool-graphic prompts at `docs/helix/01-frame/visuals/`.
- **V9.** Generate 4 tool graphics + tool composite **(sequential, budget-tracked)**.

### Workflow capstone

- **V10.** Author user-workflow prompt + generate **(sequential,
  budget-tracked)**. Explanatory register; load-bearing.

### Website integration (downstream — beads not yet created)

- Replace 6 feature cards on landing with 6 principle cards.
- Add Why DDx top-nav page (exposition around principles).
- Concept "Why this exists" callouts.
- Replace README tagline with thesis.
- E2E test updates.
- Visual integration consumes V6/V9/V10 outputs.

## Reviewer ask

1. **Tool-declaration simplification** — `package.json` + `ddx doctor`
   detection only, no FEAT-024, no DDx wrapper. Right reduction, or did we
   lose something load-bearing?
2. **bun standardization** — repo already uses Bun for the frontend per
   CLAUDE.md. Any subdirectory or build step that breaks if root tooling
   moves to bun (e.g., CI assumed npm)?
3. **Multiple `package.json` scan in `ddx doctor`** — known set (root,
   `website/`, `cli/internal/server/frontend/`) vs glob discovery? Static
   list is simpler; glob risks scanning user output dirs.
4. **`package.json` placement for `@google/design.md`** — root chosen for
   cross-cutting linter. Concerns about polluting repo root with a
   `package.json`/`node_modules` when most of the project is Go?
5. **Hextra spike fallback (accept-divergence)** — if Hextra can't consume
   DESIGN.md tokens, website CSS stays Hextra-default and DESIGN.md drives
   image prompts only. Is image-prompt-only consumption sufficient ROI to
   justify the DESIGN.md effort?
6. **Cost cap $10 sequential** — realistic for 11+ Gemini-3-Pro-Image
   generations at 2K? Should we set a per-image cost ceiling too?
7. **Acceptance criteria** — anything missing? Brand-fit / kitsch judgment?
8. **Alpha-stack risk** — two alpha surfaces (DESIGN.md format + exporter).
   Sufficient to pin `0.1.0`, or should we wait until v1?
9. **Helix-tree storage of visuals** — `docs/helix/00-discover/visuals/`
   and `docs/helix/01-frame/visuals/`. Anti-patterns or convention violations?
10. **Anything missing.**

Codebase context:
- `/Users/erik/Projects/ddx/docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md`
- `/Users/erik/Projects/ddx/CLAUDE.md` (bun precedent)
- `/Users/erik/Projects/ddx/website/` (Hugo + Hextra; no Tailwind today)
- `/Users/erik/Projects/ddx/cli/internal/server/frontend/` (existing bun/SvelteKit setup)
- `/Users/erik/Projects/ddx/.agents/skills/nano-banana-pro-openrouter/`
- DESIGN.md spec: github.com/google-labs-code/design.md

Respond with sections (omit empty):
**Keep:** correct, load-bearing
**Cut:** remove
**Missing:** gaps
**Risks:** verification needed (cite file:line)
**Questions:** require USER input

Concise. 2 sentences per finding. Severity-ordered.

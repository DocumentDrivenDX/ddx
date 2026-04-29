# Visual Suite — Principles, Tools, User Workflow

Date: 2026-04-29
Status: Draft v2 (pending review)

## Summary

DDx ships a coordinated graphical suite covering three lenses on the product:
**principles**, **tools**, and **user workflow**. Each lens gets per-element
graphics plus a composite; the user-workflow lens is the capstone.

Visual style is established via Google's **DESIGN.md** format spec
(github.com/google-labs-code/design.md) — a markdown-with-YAML-frontmatter
format describing tokens (colors, typography, spacing, components) plus
prose rationale. Comes with a `@google/design.md` npm CLI for `lint`,
`diff`, `export` (Tailwind/DTCG), and `spec`. **Status: alpha; format under
active development.**

DDx integrates DESIGN.md as the first real user of a new **tools registry**
(FEAT-024) that declares external dependencies in `.ddx.yml` and installs
them via `ddx install`. This eliminates README-ritual installation
instructions for npx-delivered tools.

All visuals are generated artifacts (per the artifact-and-run-architecture
plan), live in the helix phase tree alongside the docs they depict, with
sidecar metadata and `generated_by` provenance.

## Architecture

```
FEAT-024 tools registry (extends existing skills-lock.json pattern)
      ↓
DESIGN.md integration (declared in .ddx.yml; installed via ddx install)
      ↓
DDx DESIGN.md authored at repo root
      ↓
DESIGN.md → Hugo/Hextra Tailwind theme (export)
DESIGN.md tokens → image-generation prompts (palette anchors)
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
Website redesign (existing beads 28, 29 to be created) consumes
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
| Document graph (artifacts) | Multi-node graph with depends_on and generated_by edges |
| Agentic execution (run/try/work) | Three-layer wrapping; worktree isolation visible |
| Plugins | Modular composition; HELIX/Dun snapping into shared core |

### Lens 3 — User workflow (capstone)

Iterative diagram showing how a user actually works with DDx over time:
author/refine artifact → graph synthesizes context → bead created → agent
runs (`ddx try` / `ddx work`) → evidence captured → human re-aligns → loop
closes. Explanatory register; load-bearing artifact.

## Storage — helix phase tree

Visuals live alongside the docs they depict, not in a separate `assets/`
directory. The DESIGN.md style guide is the one cross-cutting exception.

```
DESIGN.md                                          ← cross-cutting; repo root
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

Each visual artifact has `depends_on` pointing to the source doc (vision,
PRD, FEAT-006, etc.) and `generated_by` pointing to its prompt source.
First real test of `generated_by` edges across the helix tree.

## DESIGN.md is the visual style brief

DESIGN.md *is* the style brief — its prose sections (Overview, Do's/Don'ts)
carry metaphor system and composition rules; its YAML carries tokens. No
separate STYLE.md needed.

Two consumers:
- **Website (Hugo + Hextra):** `npx @google/design.md export --format tailwind`
  produces theme JSON the site can consume directly. Hextra is Tailwind-based.
- **Image prompts:** prompts reference DESIGN.md palette by hex and named
  tokens for cross-prompt consistency.

Repo placement: `/DESIGN.md` (brand-level artifact for the whole product).

## FEAT-024 — Tools Registry (new)

DDx needs a declarative way to specify external dependencies so installation
isn't a README ritual. Partial precedent exists: `skills-lock.json` already
provides reproducibility for npx-delivered Claude/Anthropic skills (recent
commit `619027bf`).

### Scope

**Declaration** in `.ddx.yml`:

```yaml
tools:
  npx:
    "@google/design.md": "^0.1.0"
  system:
    - mermaid-cli      # version optional; verify-only
  plugins:
    helix: "0.3.3"
```

**Lifecycle:**
- `ddx install` walks the declaration; resolves npx tools project-locally
  (extends or generalizes `skills-lock.json` pattern); verifies system
  binaries; reports missing.
- `ddx doctor` re-verifies tool availability with actionable install hints.
- `ddx tool run <name> [args…]` thin wrapper for invocation (replaces
  ad-hoc `npx @google/design.md` calls in scripts; gains version pinning
  and diagnostics).

**Out of scope initially:** pip/uv, cargo, brew (defer until needed).

### Lock-file integration — open question

- **(a) Generalize `skills-lock.json` → `tools-lock.json`.** No real distinction
  between "skill delivered via npx" and "CLI delivered via npx." Cleaner
  long-term; touches more existing code; needs migration.
- **(b) Parallel `tools-lock.json`.** Skills lock unchanged; new parallel
  file for tools. Smaller blast radius; consolidation pressure later.

Plan default: (a). Decision lives in V2a (FEAT-024 spec).

### Cross-cutting with FEAT-018

Plugins are already declared/installed via `ddx install <plugin>`. FEAT-024
generalizes the pattern to non-plugin tools. Standalone feature rather than
extending FEAT-018, since plugins have richer lifecycle (subtree, config
injection) than tools.

## Generation strategy

- **Generator:** `nano-banana-pro-openrouter` (Gemini 3 Pro Image) by default.
- **Each visual is a `generate-artifact` run** per the artifact-and-run plan.
- **`SYSTEM_TEMPLATE` override** for sober/utilitarian style (default
  pulls toward "dreamy illustration").
- **DESIGN.md tokens injected** into every prompt for palette/typography
  consistency.
- **First real dogfood** of the artifact infrastructure (sidecar I/O,
  `generated_by` edges, `ddx artifact regenerate` CLI). Bugs surfaced here
  feed back into beads `ddx-d5e71fb3` (FEAT-010), `ddx-5d92b873` (FEAT-007),
  and the artifact-regenerate implementation.

## Risks

- **DESIGN.md is alpha** — format may break. Pin a specific version of
  `@google/design.md`; budget for migration when it bumps.
- **Hextra Tailwind integration unverified** — V3.5 must confirm Hextra
  accepts a custom Tailwind config override before committing the export
  pipeline. If Hextra is opinionated, a theme overlay or fork is needed.
- **Cross-prompt style consistency** on `gemini-3-pro-image-preview` is
  unproven. DESIGN.md tokens anchor palette but composition consistency
  across 6+ subsections is a real risk. Composite-first sequencing
  (V6 before V7) limits exposure.
- **Metaphor strain when rendered.** Lever/fulcrum/load may not render
  cleanly. Pressure-test in V6 before committing further. If the metaphor
  doesn't carry, revise the visual concept (not principle wording).
- **Artifact-infra bugs** could block visual generation. Plan accepts
  iteration: bugs → fixes in artifact-and-run beads → resume.
- **Scope creep.** 11+ visuals plus FEAT-024 design + implementation is a
  lot. Prioritization order if compressed: FEAT-024 minimum (V2a/b/c) →
  DESIGN.md (V3) → principle composite (V6) → workflow capstone (V10) →
  tool composite (V9) → per-tool standalones (V8) → per-principle
  standalones (V7).

## Bead breakdown

### Discovery (done)

- ~~V1.~~ Identify DESIGN.md — done; it's a Google Labs format spec + npm CLI.

### Tools registry (FEAT-024) — gates everything below

- **V2a.** Author FEAT-024 spec at `docs/helix/01-frame/features/FEAT-024-tools-registry.md`.
  Address npx parity with skills-lock, system-binary declaration,
  `ddx tool run`, lifecycle, lockfile relationship (decide a vs b).
  Audit existing `skills-lock.json` implementation as input.
- **V2b.** Implement npx tools support: extend or generalize skills-lock;
  `ddx install` resolves `tools.npx` declarations; `ddx tool run` wrapper
  CLI; `ddx doctor` extension for tool availability.
- **V2c.** Declare `@google/design.md` in `.ddx.yml`; smoke test
  `ddx install` and `ddx tool run design.md lint`.

### DESIGN.md integration

- **V3.** Author `DESIGN.md` at repo root. Capture existing precedent
  (logo, current rgba triad, terminal-demo tone). Tokens (colors, typography,
  spacing, components); prose Overview, Do's/Don'ts, metaphor system. Lint
  via V2b before commit.
- **V3.5.** Wire DESIGN.md → Hugo/Hextra Tailwind theme.
  `npx @google/design.md export --format tailwind` into website build.
  **Verify Hextra accepts custom Tailwind config first**; if not, scope
  expands to theme overlay or fork.
- **V4.** Lint integration in lefthook + CI: `ddx tool run design.md lint`
  on commit if DESIGN.md is modified.

### Principles lens

- **V5.** Author all 6 principle-graphic prompts at
  `docs/helix/00-discover/visuals/principle-N-*.prompt.md`.
- **V6.** Author principle-composite prompt + generate. **Pressure-test
  gate** — if the lever/fulcrum/load metaphor doesn't render usefully,
  pause and revise before proceeding.
- **V7** *(optional)*. Render the 6 principle subsections standalone if
  composite reads well alone. Skip if composite suffices or if cross-prompt
  consistency fails.

### Tools lens

- **V8.** Author 4 tool-graphic prompts at `docs/helix/01-frame/visuals/`.
- **V9.** Generate 4 tool graphics + tool composite.

### Workflow capstone

- **V10.** Author user-workflow prompt + generate. Explanatory register;
  artifact in helix tree; load-bearing.

### Website integration (downstream — existing planned beads, not yet created)

- **28** website refactor — consumes V3.5 (Tailwind theme), V6 (principle
  composite), V9 (tool composite), V10 (workflow); replaces 6 feature cards
  with 6 principle cards; new Why DDx page.
- **29** README refactor — consumes V6/V10; replaces tagline with thesis.

## Reviewer ask

1. **FEAT-024 scope boundary.** Right shape — declaration in `.ddx.yml`,
   `ddx install` resolves, `ddx tool run` wraps invocation? Anything missing
   (caching, version-range resolution, security, supply-chain concerns)?
2. **Lock-file integration (a vs b).** Default leans (a) generalize. Push
   back if (b) parallel makes more sense given existing skills-lock semantics.
3. **Plugins inside or outside tools registry.** Plan keeps them outside
   (richer lifecycle). Reconsider?
4. **Helix-tree storage of visuals.** Visuals live in `docs/helix/00-discover/visuals/`
   and `docs/helix/01-frame/visuals/`. Anti-patterns? Helix convention violations?
5. **DESIGN.md alpha risk.** Pinning + migration budget; sufficient
   mitigation, or should we wait until v1?
6. **Hextra Tailwind override** — known behavior or unknown? V3.5 should
   verify before commit.
7. **Cross-prompt consistency** — DESIGN.md tokens enough? Or do we need
   reference-image compositing or a different generator for the principles
   and tools lenses?
8. **Sequencing realism.** V2a/b/c (FEAT-024 design + implementation) is
   serious work — does this plan budget it correctly, or is it under-scoped
   given DDx's "DDx provides primitives, opinions in plugins" stance?
9. **Anything missing.**

Codebase context:
- `/Users/erik/Projects/ddx/docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md` (artifact infra plan; visuals are first dogfood)
- `/Users/erik/Projects/ddx/skills-lock.json` (existing npx skills lock)
- `/Users/erik/Projects/ddx/.ddx.yml` (existing config)
- `/Users/erik/Projects/ddx/docs/helix/01-frame/features/FEAT-018-plugin-api.md`
- `/Users/erik/Projects/ddx/docs/helix/01-frame/features/FEAT-011-skills.md`
- `/Users/erik/Projects/ddx/website/` — Hugo + Hextra
- `/Users/erik/Projects/ddx/.agents/skills/nano-banana-pro-openrouter/`
- `/Users/erik/Projects/ddx/CLAUDE.md` (project-local install convention)
- DESIGN.md spec: github.com/google-labs-code/design.md

Respond with sections (omit empty):
**Keep:** correct, load-bearing
**Cut:** remove
**Missing:** gaps
**Risks:** verification needed (cite file:line)
**Questions:** require USER input

Concise. 2 sentences per finding. Severity-ordered.

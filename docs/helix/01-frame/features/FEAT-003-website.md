---
ddx:
  id: FEAT-003
  depends_on:
    - helix.prd
---
# Feature: DDx Website

**ID:** FEAT-003
**Status:** In Progress
**Priority:** P0
**Owner:** DDx Team

## Overview

`ddx.github.io` is the promotional and documentation website for DDx. It explains what document-driven development is, why DDx exists, how to get started, and how the ecosystem fits together. Hosted on GitHub Pages.

## Problem Statement

**Current situation:** DDx has a README but no dedicated website. New visitors must parse a GitHub repo to understand what DDx is and whether it's useful to them.

**Pain points:**
- No landing page that explains the concept to someone who hasn't seen it before
- No visual explanation of the ecosystem (DDx, workflow tools, agents)
- Installation and quick-start instructions buried in a README
- No SEO presence — developers searching for document management tooling won't find DDx

**Desired outcome:** A clean, fast website that explains DDx in under 2 minutes, gets a developer to installation in under 5 minutes, and serves as the canonical reference for the project.

## Requirements

### Functional

1. **Landing page** — hero section explaining DDx in one sentence, with the sub-claim **"A document-driven software factory."** Includes visual ecosystem diagram, call-to-action to install, and a link to the new `/docs/concepts/software-factory/` page.
2. **Quick start** — copy-pasteable installation command, 3-step getting started guide
3. **Concepts pages** — explain document-driven development, document types, how DDx fits with agents and workflow tools. Includes:
   - `/docs/concepts/software-factory.md` — DDx as a document-driven software factory (lineage, what is inherited, what is added, what is intentionally not done)
   - `/docs/concepts/run-architecture.md` — three-layer run model (`ddx run` / `ddx try` / `ddx work`); retires `execute-loop` / `execute-bead` from primary copy
   - Existing concept pages (architecture, document model, etc.) updated to use the new naming
4. **Why DDx (`/why`)** — top-level page rewritten as 10 sections, one per **domain principle**. Each section has a 50–80 word summary linking to a deep page at `/docs/principles/<slug>/`. The 10 domain principles (locked):
   1. Spec-first development
   2. Executable specifications
   3. Audit trail required
   4. Context is king
   5. Work is a DAG
   6. Right-size the model
   7. Avoid vendor lock-in
   8. Drift is debt
   9. Least privilege for agents
   10. Inspect and adapt
5. **Principle deep pages** — 10 pages at `/docs/principles/<slug>/index.md`, each rendered from the corresponding `RSCH-NNN` research artifact in `docs/helix/00-discover/research/`. Each deep page presents claim → evidence → DDx-feature mapping.
6. **CLI reference** — curated noun-verb command documentation matching the command overview in `CLAUDE.md`
7. **Ecosystem page** — shows DDx's position as infrastructure, lists workflow tools built on DDx, highlights HELIX as first plugin. **Deprecated peer tools are removed from all site copy** (no longer peers in the ecosystem).
8. **Server documentation** — how to run ddx-server, MCP endpoint reference
9. **Embedded terminal demos** — asciinema recordings of core workflows embedded in hero section and getting-started page:
   - Install DDx
   - `ddx init` + `ddx list` + `ddx doctor`
   - `ddx install helix` (plugin bootstrap)
   - One-shot project creation with HELIX
   - Feature evolution with HELIX
10. **README** — animated GIF/SVG demos, badge row, plugin quick start, link to microsite. The README is the GitHub-facing landing page and must sell at a glance.

### Content stance

- **Deprecated peer tools removed.** No references to deprecated peer tools anywhere in site copy, ecosystem diagrams, or glossary.
- **Quorum de-emphasized.** Multi-model review (quorum) is no longer a primary capability and is removed from the features list. It is documented only in skill-level pages and badged as "framing" (matches `FEAT-013` status).
- **Naming.** `ddx run` / `ddx try` / `ddx work` is the unified primary naming. `execute-loop` / `execute-bead` appear only inside the CLI reference, not in primary copy.
- **Software factory framing.** The homepage, `docs/helix/00-discover/product-vision.md` (new "Lineage" section), and `docs/helix/01-frame/principles.md` (new preface) all frame DDx as a document-driven software factory.

### Information architecture

```
/                                  → homepage (sub-claim: "A document-driven software factory.")
/why                               → 10 principle summaries (domain principles)
/docs/getting-started              → install + first steps
/docs/concepts/software-factory    → factory framing
/docs/concepts/run-architecture    → run/try/work three-layer model
/docs/concepts/<existing>          → architecture, document model, etc.
/docs/principles/<slug>/           → 10 deep pages, one per domain principle
/docs/cli/                         → curated noun-verb CLI reference
/docs/server/                      → ddx-server + MCP endpoint reference
/docs/ecosystem                    → DDx + HELIX positioning (deprecated peer tools removed)
```

The `/why` summaries and `/docs/principles/<slug>/` deep pages are the user-facing **domain principle taxonomy** (10 pages). The complementary 6 **engineering principles** are internal and live in `docs/dev/engineering-principles.md`, not on the site.

### Non-Functional

- **Performance:** Static site, loads in <2 seconds on 3G
- **Hosting:** GitHub Pages, no server infrastructure needed
- **Maintenance:** Content derived from repo docs where possible, minimal manual upkeep
- **Design:** Clean, developer-focused. No enterprise fluff. Code examples prominent.
- **SEO:** Targets "document-driven development", "AI agent document management", "MCP document server"

## User Stories

### US-020: Developer Discovers DDx
**As a** developer searching for tools to manage AI agent documents
**I want** to land on a page that explains what DDx does in 30 seconds
**So that** I can decide whether to try it

**Acceptance Criteria:**
- Given I visit ddx.github.io, when the page loads, then I see a clear one-sentence description and a visual diagram
- Given I'm interested, when I scroll down, then I see a quick-start section with a copy-pasteable install command
- Given I want details, when I click "Learn more", then I reach a concepts page explaining document-driven development

### US-021: Developer Installs DDx from Website
**As a** developer who decided to try DDx
**I want** to copy an install command and get started
**So that** I go from website to working tool in under 5 minutes

**Acceptance Criteria:**
- Given I'm on the quick-start section, when I click the install command, then it copies to my clipboard
- Given I've installed DDx, when I follow the "First steps" guide on the site, then I have a working document library in my project

### US-023: Developer Sees DDx in Action Before Installing
**As a** developer evaluating DDx
**I want** to watch a terminal recording of the core workflow
**So that** I can see what DDx actually does before committing to install it

**Acceptance Criteria:**
- Given I visit the landing page, when it loads, then I see an embedded terminal recording in or near the hero section
- Given I visit the getting-started page, then each step has a corresponding recording I can watch
- Given I visit the ecosystem page, then I see a recording of `ddx install helix` bootstrapping the workflow plugin

### US-024: Developer Evaluates DDx from GitHub README
**As a** developer who finds DDx on GitHub
**I want** the README to show me what DDx does in under 30 seconds
**So that** I decide whether to click through to the full site

**Acceptance Criteria:**
- Given I open the GitHub repo, when the README renders, then I see an animated terminal demo (GIF or SVG) of the init workflow
- Given I scroll the README, then I see a plugin install example and a link to the microsite

### US-022: Developer Understands the Ecosystem
**As a** developer evaluating DDx
**I want** to understand how DDx relates to workflow tools and agents
**So that** I know what DDx does vs what I need to get elsewhere

**Acceptance Criteria:**
- Given I visit the ecosystem page, when I read it, then I understand that DDx is infrastructure and workflow tools are separate
- Given I see the diagram, then I can identify what layer DDx occupies vs HELIX vs agents

## Edge Cases

- Visitor on mobile — site must be responsive
- Visitor with JavaScript disabled — core content must render without JS (demo recordings degrade to static screenshots or text)
- Stale documentation — establish process for regenerating CLI docs from source
- Stale demo recordings — CI regenerates recordings when CLI changes; PR opened if recordings differ

## Implementation

- **Static site generator:** Hugo with Hextra theme (Tailwind CSS, hero shortcodes, dark mode, search)
- **Location:** `/website/` directory in monorepo
- **Deployment:** GitHub Actions → GitHub Pages (`.github/workflows/pages.yml`)
- **Hugo module:** `github.com/imfing/hextra`

### Pages Built

- Landing page with hero, feature grid, CTAs, software-factory sub-claim
- Getting Started (install, init, first steps)
- `/why` — 10 domain principle summaries with links to deep pages
- `/docs/principles/<slug>/` — 10 deep pages backed by `RSCH-NNN` research artifacts
- `/docs/concepts/software-factory.md` — software-factory framing
- `/docs/concepts/run-architecture.md` — `ddx run` / `ddx try` / `ddx work` three-layer model
- Other concepts pages (document-driven development explanation, document model, architecture)
- CLI Reference (curated noun-verb reference)
- DDx Server (MCP endpoints, HTTP API — marked as under development)
- Ecosystem (stack diagram, what-goes-where table; deprecated peer tools removed)

### Demo Recording Pipeline

- Reproducible scripts in `scripts/demos/` drive asciinema recordings
- Each script produces a `.cast` file rendered to GIF/SVG via agg or svg-term
- Rendered assets live in `website/static/demos/` for Hugo embedding
- Hugo shortcode or partial wraps asciinema-player for `.cast` playback
- CI workflow (`.github/workflows/demos.yml`) regenerates on CLI changes

## Dependencies

- Hugo 0.159+ extended
- Hextra theme via Hugo modules
- GitHub Pages hosting
- Go toolchain (for Hugo modules)
- asciinema (recording) + agg or svg-term (rendering) for demo pipeline
- CI workflow for demo regeneration (`.github/workflows/demos.yml`)
- CI gate: pages deployment depends on CI passing

## Out of Scope

- Interactive document browser (P1 — would integrate with ddx-server)
- User accounts or login
- Blog or news feed
- Community forum (use GitHub Discussions)
- Localization / i18n

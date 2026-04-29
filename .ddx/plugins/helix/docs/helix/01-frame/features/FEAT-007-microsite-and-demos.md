---
dun:
  id: FEAT-007
  depends_on:
    - helix.prd
    - FEAT-006
---
# Feature Specification: FEAT-007 — Microsite and Demo Reels

**Feature ID**: FEAT-007
**Status**: Active
**Priority**: P1
**Owner**: HELIX maintainers

## Overview

HELIX needs a public-facing documentation site and scripted demo reels that
show users what HELIX does, how it works, and how to get started. The
microsite is the primary onboarding surface — it must take a new user from
"what is this?" to running `helix run` on their own project.

This feature covers both the Hugo+Hextra microsite (`website/`) and the
reproducible demo reel system (`docs/demos/`). It depends on FEAT-006
because the concern system (hugo-hextra, demo-asciinema) governs the
technology choices and practices for this work.

## Problem Statement

- **Current situation**: HELIX has comprehensive internal workflow docs
  (`workflows/`) but no public-facing site that explains the system to new
  users. The internal docs are written for agents, not humans.
- **Pain points**:
  1. No discoverable entry point — users must read AGENTS.md or workflows/
     to understand HELIX, which is too dense for first contact.
  2. No visual demonstration — text descriptions of the lifecycle are less
     convincing than watching it happen.
  3. No glossary — HELIX introduces many terms (beads, concerns, authority
     order, context digests) that need clear definitions in one place.
  4. No reference for artifact types — users building HELIX projects need
     to know what artifacts exist and when to create them.
- **Desired outcome**: A public site at `easel.github.io/helix/` that
  serves as the primary onboarding and reference surface for HELIX users.

## Functional Requirements

### FR-1: Site Structure

The microsite must include at minimum:

| Section | Purpose | Status |
|---------|---------|--------|
| **Home** | Hero, value prop, feature grid, embedded demo, CTA | Done |
| **Getting Started** | Install and first supervised build in under 10 minutes | Done |
| **Workflow** | Phases, authority order, supervisory loop, stopping conditions | Done |
| **CLI Reference** | Every command with options, env vars, and examples | Done |
| **Skills** | Agent skill listing, installation modes, naming conventions | Done |
| **Glossary** | Definitions for all HELIX terms, organized by category | Done |
| **Demos** | Embedded terminal recordings with run-it-yourself instructions | Done |

### FR-2: Glossary Coverage

The glossary must define every user-facing HELIX concept:

- **Phases**: All 6 lifecycle phases with key artifacts and commands
- **Artifacts**: Every artifact type by phase, with file locations and the
  authority order
- **Actions**: Every supervisory command with usage and behavior
- **Concerns**: The concern library, how filtering works, drift signals
- **Concepts**: Authority order, TDD, context digests, principles, ratchets,
  bounded execution, cross-model verification, epic focus, area taxonomy
- **Tracker**: Beads, labels, queue control, spec-id, dependencies

### FR-3: Demo Reels

Each demo reel must:

- Be fully scripted (no manual recording)
- Run in a Docker container for reproducibility
- Include a narrative structure: Setup → Core Workflow → Verification → Summary
- Produce an asciinema `.cast` file for microsite embedding
- Produce a GIF for README and social contexts
- Include a `README.md` explaining what it demonstrates and how to run it

#### Required Demos

| Demo | Scope | Status |
|------|-------|--------|
| **Quickstart** | Full lifecycle: install → frame → design → build → review | Done |
| **Concerns** | Concern selection during frame, drift detection during review | Done |
| **Evolve** | Threading a requirement change through the artifact stack | Done |
| **Experiment** | Metric-driven optimization loop | Done |

### FR-4: Technology Stack

The microsite uses:

- **Hugo** (extended edition, pinned version) as static site generator
- **Hextra** theme via Go Module for layout, search, navigation, dark mode
- **Asciinema-player** (v3.7.1) via CDN for terminal recording playback
- **GitHub Pages** for deployment via GitHub Actions
- **Playwright** for e2e testing with screenshot snapshots, video recording

These choices are declared in the `hugo-hextra` and `demo-asciinema`
concerns and must not drift without an ADR.

### FR-5: Content Quality

- Content must be readable as plain markdown on GitHub (not just in Hugo)
- Getting Started must get a user running in under 5 minutes of reading
- CLI Reference must document every command with at least one example
- Glossary definitions must be concise but complete — one table row or
  paragraph per term, not multi-page essays
- Demo pages must embed the recording AND provide run-it-yourself instructions

### FR-6: Deployment

- Site builds via `hugo --gc --minify` with zero errors and zero warnings
- GitHub Actions deploys on successful test + push to main
- Checkout uses `fetch-depth: 0` for git-based last-modified dates
- Concurrent deployment protection prevents race conditions

## Non-Functional Requirements

- **Performance**: Site must load in under 2 seconds on a 3G connection
  (Hugo static output achieves this by default)
- **Accessibility**: Hextra theme provides baseline a11y; custom shortcodes
  must not degrade it
- **SEO**: `enableRobotsTXT: true`, proper `<title>` tags, OpenGraph meta
  from Hextra defaults
- **Maintenance**: Content updates should not require Hugo or Go knowledge —
  just edit markdown and push

## User Stories

- US-020: As a developer evaluating HELIX, I want to watch a demo reel so
  I can understand the lifecycle without installing anything.
- US-021: As a new HELIX user, I want a getting-started guide so I can run
  my first supervised build in under 10 minutes.
- US-022: As a HELIX user, I want a glossary so I can look up unfamiliar
  terms without reading the full workflow docs.
- US-023: As a HELIX user building my own project, I want an artifact
  reference so I know what documents to create at each phase.
- US-024: As a HELIX contributor, I want the site to build and deploy
  automatically so I can focus on content, not infrastructure.

## Success Metrics

- Site is live at `easel.github.io/helix/`
- All required sections have substantive content (not stubs)
- Hugo build produces zero errors
- At least one demo reel is embedded and playable
- Glossary covers all terms from `workflows/REFERENCE.md`

## Constraints

- Site content lives in `website/` — not the repo root
- Hugo + Hextra are the only supported site tools (concern: `hugo-hextra`)
- Demo recordings use asciinema only (concern: `demo-asciinema`)
- No custom CSS unless Hextra genuinely cannot achieve the design goal
- No JavaScript beyond what Hextra and the asciinema shortcode provide

## Dependencies

- FEAT-006 (Concerns) — `hugo-hextra` and `demo-asciinema` concerns
  govern technology choices
- DDx (bead tracker) — demo scripts use `ddx bead` commands

## Out of Scope

- Blog or changelog section (may be added in a future iteration)
- Multi-language / i18n support
- Custom theme development beyond shortcodes
- Video (non-terminal) demos
- User accounts or interactive features

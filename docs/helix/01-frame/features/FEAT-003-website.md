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

1. **Landing page** — hero section explaining DDx in one sentence, visual ecosystem diagram, call-to-action to install
2. **Quick start** — copy-pasteable installation command, 3-step getting started guide
3. **Concepts page** — explains document-driven development, document types, how DDx fits with agents and workflow tools
4. **CLI reference** — command documentation (can be generated from CLI help text)
5. **Ecosystem page** — shows DDx's position as infrastructure, lists workflow tools built on DDx
6. **Server documentation** — how to run ddx-server, MCP endpoint reference

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

### US-022: Developer Understands the Ecosystem
**As a** developer evaluating DDx
**I want** to understand how DDx relates to workflow tools and agents
**So that** I know what DDx does vs what I need to get elsewhere

**Acceptance Criteria:**
- Given I visit the ecosystem page, when I read it, then I understand that DDx is infrastructure and workflow tools are separate
- Given I see the diagram, then I can identify what layer DDx occupies vs HELIX vs agents

## Edge Cases

- Visitor on mobile — site must be responsive
- Visitor with JavaScript disabled — core content must render without JS
- Stale documentation — establish process for regenerating CLI docs from source

## Implementation

- **Static site generator:** Hugo with Hextra theme (Tailwind CSS, hero shortcodes, dark mode, search)
- **Location:** `/website/` directory in monorepo
- **Deployment:** GitHub Actions → GitHub Pages (`.github/workflows/pages.yml`)
- **Hugo module:** `github.com/imfing/hextra`

### Pages Built

- Landing page with hero, feature grid, CTAs
- Getting Started (install, init, first steps)
- Concepts (document-driven development explanation)
- CLI Reference (all commands)
- DDx Server (MCP endpoints, HTTP API — marked as under development)
- Ecosystem (stack diagram, what-goes-where table)

## Dependencies

- Hugo 0.159+ extended
- Hextra theme via Hugo modules
- GitHub Pages hosting
- Go toolchain (for Hugo modules)

## Out of Scope

- Interactive document browser (P1 — would integrate with ddx-server)
- User accounts or login
- Blog or news feed
- Community forum (use GitHub Discussions)
- Localization / i18n

# Feature: DDx CLI

**ID:** FEAT-001
**Status:** In Progress
**Priority:** P0
**Owner:** DDx Team

## Overview

The `ddx` CLI is the core tool — a single Go binary that manages document libraries locally. It handles initialization, browsing, persona management, template application, git sync, meta-prompt injection, and MCP server configuration.

## Problem Statement

**Current situation:** Developers managing agent-facing documents have no tooling. Documents are ad-hoc files with no conventions, no composition, no cross-project reuse.

**Pain points:**
- No standard structure for document libraries
- Manual assembly of agent context from scattered files
- No way to sync proven patterns between projects
- Persona definitions duplicated and inconsistent across repos
- Template application requires manual copy-paste and find-replace

**Desired outcome:** A single CLI command manages the entire document library lifecycle — from initialization through daily use to cross-project sharing.

## Requirements

### Functional

1. **Library initialization** (`ddx init`) — create `.ddx/library/` structure, generate config, optionally sync from upstream
2. **Document browsing** (`ddx list`, `ddx prompts list/show`, `ddx templates list`) — discover and inspect documents in the library
3. **Persona system** (`ddx persona list/show/bind`) — manage persona definitions and role bindings
4. **Template application** (`ddx templates apply`) — apply project templates with variable substitution
5. **Git sync** (`ddx update`, `ddx contribute`) — bidirectional sync with upstream library via git subtree
6. **Meta-prompt injection** — inject system-level instructions into CLAUDE.md during init
7. **MCP management** (`ddx mcp list/install`) — install and configure MCP servers as project-local dependencies
8. **Health checks** (`ddx doctor`) — validate library structure, config, git setup, dependencies
9. **Self-upgrade** (`ddx upgrade`) — check for and install newer DDx versions
10. **Status and history** (`ddx status`, `ddx log`) — show sync state, local modifications, change history

### Non-Functional

- **Performance:** All local operations complete in <1 second
- **Portability:** Single binary, no runtime dependencies. Runs on macOS, Linux, Windows.
- **Reliability:** Git subtree operations handle conflicts gracefully with clear error messages
- **Installability:** Single curl command or `go install`

## User Stories

### US-001: Initialize Document Library
**As a** developer starting a new project
**I want** to run `ddx init` and get a structured document library
**So that** my agent-facing documents have a home from day one

**Acceptance Criteria:**
- Given I'm in a project directory without DDx, when I run `ddx init`, then a `.ddx/config.yaml` and `.ddx/library/` structure are created
- Given DDx is already initialized, when I run `ddx init`, then I get a clear message that it's already set up
- Given I run `ddx init --no-git`, then git subtree setup is skipped

### US-002: Browse Available Documents
**As a** developer looking for patterns
**I want** to list and search documents in my library
**So that** I can find what's available without digging through directories

**Acceptance Criteria:**
- Given a populated library, when I run `ddx list`, then I see all document categories with counts
- Given I run `ddx list prompts`, then I see only prompt documents
- Given I run `ddx prompts show <name>`, then I see the full prompt content

### US-003: Manage Personas
**As a** developer configuring agent behavior
**I want** to browse personas and bind them to roles
**So that** my agents behave consistently across the project

**Acceptance Criteria:**
- Given personas exist in the library, when I run `ddx persona list`, then I see available personas with descriptions
- Given I run `ddx persona bind code-reviewer strict-code-reviewer`, then the binding is saved to `.ddx.yml`

### US-004: Sync Document Library
**As a** developer on a team
**I want** to pull improvements from upstream and push my own back
**So that** proven patterns flow between projects

**Acceptance Criteria:**
- Given an upstream repo is configured, when I run `ddx update`, then the latest library changes are pulled
- Given I've improved a pattern, when I run `ddx contribute`, then my changes are pushed upstream

## Edge Cases

- Init in a directory without git — should work (skip subtree), warn about limited sync
- Sync when upstream has conflicts — provide clear merge conflict instructions
- List when library is empty — show helpful message about populating it
- Doctor when dependencies are missing — list what's missing and how to install

## Dependencies

- Git 2.0+ (for subtree sync)
- go-based build toolchain (development only)

## Out of Scope

- Workflow enforcement (that's HELIX, not DDx)
- Document editing UI (use your editor)
- Network-based document access (that's ddx-server)

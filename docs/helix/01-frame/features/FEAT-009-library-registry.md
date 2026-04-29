---
ddx:
  id: FEAT-009
  depends_on:
    - helix.prd
---
# Feature: Online Library & Plugin Registry

**ID:** FEAT-009
**Status:** Complete
**Priority:** P0
**Owner:** DDx Team

## Overview

DDx needs to discover, install, and manage resources from an online library. The library is a git repository (`ddx-library`) containing personas, prompts, templates, patterns, MCP configurations, and **package descriptors** for workflow tools like HELIX. DDx fetches resources on demand — no git subtree, no full repo clone.

This replaces the current git-subtree-based sync model with a lighter, more practical approach: DDx downloads what you ask for, caches it locally, and keeps track of what's installed.

## Problem Statement

**Current situation:**
- `ddx-library` exists as a git repo but DDx's sync mechanism (`ddx update`) is a stub
- Git subtree is complex, fragile, and overkill for distributing personas and templates
- HELIX publishes skills independently (`~/.agents/skills/`) with no DDx integration
- Dun expects plugins at `~/.cache/ddx/library/plugins/` but nothing populates this path
- There's no way to discover what's available or install a specific resource

**Desired outcome:** `ddx install helix` fetches and installs HELIX skills. `ddx install persona/strict-code-reviewer` fetches one persona. `ddx search testing` finds testing-related resources. Simple, practical, no git subtree.

## Architecture

### Registries

DDx supports multiple registries. Each registry is a git repository containing a `registry.yaml` index and installable resources. Registries are checked in order — first match wins.

```yaml
# .ddx/config.yaml
registries:
  - url: https://github.com/DocumentDrivenDX/ddx-library     # default, always present
    branch: main
  - url: https://github.com/mycompany/ddx-private  # company-private
    branch: main
```

The default registry (`https://github.com/DocumentDrivenDX/ddx-library`) is always included even if not explicitly listed. Additional registries are additive — they extend the default, not replace it.

### Registry Repository Structure

Each registry repo has:

```
ddx-library/
├── registry.yaml              # Index of all available packages
├── personas/
│   ├── strict-code-reviewer.md
│   ├── pragmatic-implementer.md
│   └── ...
├── prompts/
│   └── ...
├── templates/
│   └── ...
├── patterns/
│   └── ...
├── artifacts/                 # Artifact type resources
│   ├── adr/
│   │   ├── template.md
│   │   ├── create.md
│   │   ├── evolve.md
│   │   └── check.md
│   └── ...
├── mcp-servers/
│   └── registry.yml
├── workflows/
│   └── helix/
│       └── package.yaml       # HELIX package descriptor
└── plugins/
    └── helix/
        └── plugin.yaml        # Dun plugin for HELIX checks
```

### Package Descriptor

Workflow tools and plugins publish a `package.yaml` in the library:

```yaml
name: helix
version: 1.0.0
description: Structured development workflow with AI-assisted collaboration
type: workflow                  # workflow | plugin | persona-pack | template-pack
source: https://github.com/DocumentDrivenDX/helix
install:
  skills:
    source: .agents/skills/     # Path in source repo
    target: ~/.agents/skills/   # Install destination
  scripts:
    source: scripts/helix
    target: ~/.local/bin/helix
requires:
  - ddx >= 0.2.0
```

### Install Flow

> **Amended by FEAT-015 (2026-04-29):** `ddx install <plugin>` is now
> project-local. Skills land in `<projectRoot>/.agents/skills/` and
> `<projectRoot>/.claude/skills/` as real files (no symlinks, no home-
> directory writes). The `~/.agents/skills/` / `~/.local/bin/helix`
> targets and `~/.ddx/installed.yaml` flow described below are
> historical context from the pre-amendment global model. See FEAT-015
> for the authoritative install semantics and the no-`~` manifest
> invariant.

```bash
ddx install helix
```

1. Fetch `registry.yaml` from ddx-library (cached, refreshed on `ddx update`)
2. Find the `helix` entry → read `package.yaml`
3. Clone/download the source repo (shallow, to temp dir)
4. Copy skills to `~/.agents/skills/helix-*`
5. Copy scripts to `~/.local/bin/helix`
6. Record installation in `~/.ddx/installed.yaml`

For simple resources (individual personas, templates):

```bash
ddx install persona/strict-code-reviewer
```

1. Fetch the file directly from ddx-library (via GitHub raw URL or git archive)
2. Copy to `.ddx/library/personas/strict-code-reviewer.md`

### Cache and State

- **Registry cache:** `~/.cache/ddx/registries/<name>/registry.yaml` (one per registry, refreshed by `ddx update`)
- **Installation record:** `~/.ddx/installed.yaml` (what's installed, versions, timestamps, source registry)
- **Library cache:** `~/.cache/ddx/library/` (downloaded resources)
- **Plugin cache:** `~/.cache/ddx/library/plugins/` (populated for dun discovery)

## Requirements

### Functional

1. **Registry fetch** (`ddx update`) — download latest `registry.yaml` from ddx-library
2. **Search** (`ddx search <query>`) — search available resources by name, type, or keyword
3. **Install resource** (`ddx install <name>`) — download and install a specific resource or package
4. **Install workflow** (`ddx install helix`) — full workflow installation (skills, scripts, plugins)
5. **List installed** (`ddx installed`) — show what's installed locally
6. **Uninstall** (`ddx uninstall <name>`) — remove an installed resource
7. **Populate plugin cache** — on install, copy dun-compatible plugins to `~/.cache/ddx/library/plugins/`
8. **Version tracking** — record installed versions, detect available updates
9. **Update detection** (`ddx outdated`) — compare installed package versions
   against source repo tags (via `git ls-remote --tags`) to determine if
   updates are available. Output: package name, installed version, latest
   available, update available (yes/no).
10. **Package update** (`ddx upgrade <name>`) — re-install a package at the
    latest available version. Performs a fresh shallow clone at the latest tag,
    copies new files, updates `installed.yaml`. Safe to run repeatedly.
11. **Startup update check** — on `ddx` startup (async, non-blocking), check
    if installed packages have newer versions available. If so, print a
    one-line notice: `Plugin update available: helix 0.1.0 → 0.2.0 (run
    'ddx upgrade helix')`. Same pattern as the existing DDx binary update
    check.

### Non-Functional

- **No git subtree** — fetch individual files or shallow clones, not full repo history
- **Offline-safe** — work from cache when offline; warn but don't fail
- **Idempotent** — running `ddx install helix` twice is safe
- **Fast** — individual resource install <5s on broadband

## CLI Commands

```bash
ddx update                          # Refresh registry from upstream
ddx search <query>                  # Search available resources
ddx install <name>                  # Install a resource or package
ddx install helix                   # Install HELIX workflow
ddx install persona/strict-code-reviewer  # Install one persona
ddx installed                       # List what's installed
ddx outdated                        # Check for available updates
ddx upgrade <name>                  # Update a package to latest version
ddx uninstall <name>                # Remove an installed resource
```

## User Stories

### US-090: Developer Discovers Available Workflows
**As a** developer evaluating DDx
**I want** to search for available workflow tools
**So that** I can find and install HELIX or other methodologies

**Acceptance Criteria:**
- Given I run `ddx search workflow`, then I see HELIX and any other registered workflows with descriptions
- Given I run `ddx install helix`, then HELIX skills are installed to `~/.agents/skills/` and the CLI to `~/.local/bin/`

### US-091: Developer Installs Individual Resources
**As a** developer customizing my project
**I want** to install specific personas or templates from the library
**So that** I get exactly what I need without bulk downloading

**Acceptance Criteria:**
- Given I run `ddx install persona/strict-code-reviewer`, then the persona file is copied to `.ddx/library/personas/`
- Given I run `ddx installed`, then I see `persona/strict-code-reviewer` with version and install date

## Migration from Git Subtree

The current `ddx init` creates a git subtree for `.ddx/library/`. This should be replaced:

1. **New projects:** `ddx init` creates `.ddx/library/` as a local directory (no subtree)
2. **Library population:** `ddx install` fetches resources on demand
3. **Existing projects:** The git subtree continues to work but is no longer the recommended flow
4. **`ddx contribute`:** Remains as a way to push changes back (creates a PR against ddx-library)

## Dependencies

- `ddx-library` repo with `registry.yaml`
- GitHub API or git archive for fetching individual files
- Package descriptors in workflow tool repos (HELIX, etc.)

## Out of Scope

- Package signing or verification (v2)
- Automatic updates (manual `ddx update` + `ddx install` for now)
- Dependency resolution between packages

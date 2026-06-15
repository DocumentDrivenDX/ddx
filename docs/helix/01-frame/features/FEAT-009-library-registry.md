---
ddx:
  id: FEAT-009
  depends_on:
    - helix.prd
---
# Feature: Online Library & Plugin Registry

> **FEAT-015/ADR-027 amendment (2026-06-15):** Plugin lifecycle is
> project-local and lives under `ddx plugin *`. Registry plugins install like
> `npx` dependencies: the project records intent in `.ddx/plugins.lock.yaml`,
> payload bytes live under `${XDG_DATA_HOME}/ddx/cache/plugins/<name>/<version>/`,
> and `.agents/skills/*` plus `.claude/skills/*` are generated adapters that
> can be recreated with `ddx plugin sync`. Normal marketplace installs do not
> copy full plugin payloads into `<projectRoot>/.ddx/plugins/`. Local developer
> overlays remain the exception: `ddx plugin install <name> --local <path>`
> symlinks `.ddx/plugins/<name>` and generated skill targets to the checkout.
> `ddx upgrade` is reserved for the DDx binary. No plugin command writes
> home-directory DDx state.

**ID:** FEAT-009
**Status:** Complete
**Priority:** P0
**Owner:** DDx Team

## Overview

DDx needs to discover, install, and manage resources from an online library. The library is a git repository (`ddx-library`) containing personas, prompts, templates, patterns, MCP configurations, and **package descriptors** for workflow tools like HELIX. DDx fetches resources on demand — no full repo clone needed.

This is a lightweight, practical approach: DDx downloads what you ask for, caches it locally, and keeps track of what's installed.

## Problem Statement

**Current situation:**
- `ddx-library` exists as a git repo but DDx's sync mechanism (`ddx update`) is a stub
- There is no lightweight way to distribute personas and templates on demand
- HELIX publishes skills independently (global home paths) with no DDx integration
- External check runners expect plugins at `~/.cache/ddx/library/plugins/` but nothing populates this path
- There's no way to discover what's available or install a specific resource

**Desired outcome:** `ddx plugin install helix` fetches and installs HELIX skills. Resource-library installs, if exposed separately, must not reuse the plugin lifecycle commands. `ddx search testing` finds testing-related resources. Simple and practical.

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
        └── plugin.yaml        # Check-runner plugin for HELIX checks
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
    target: .agents/skills/     # Install destination (project-local; FEAT-015)
  scripts:
    source: scripts/helix
    target: .ddx/plugins/helix/scripts/helix
requires:
  - ddx >= 0.2.0
```

### Install Flow

> **Amended by FEAT-015/ADR-027 (2026-06-15):** `ddx plugin install <plugin>`
> is the forward plugin install command. Registry installs write project lock
> metadata, resolve payloads into the XDG plugin cache, and generate
> agent-facing adapters under `.agents/skills/` and `.claude/skills/`. Local
> overlays use symlinks for `--local`. Project plugin state lives in
> `.ddx/plugins.lock.yaml`; no home plugin state exists.

```bash
ddx plugin install helix
```

1. Fetch `registry.yaml` from ddx-library
2. Find the `helix` entry → read `package.yaml`
3. Clone/download the source repo (shallow, to temp dir)
4. Copy the package payload into
   `${XDG_DATA_HOME}/ddx/cache/plugins/helix/<version>/`
5. Generate adapter links under `<projectRoot>/.agents/skills/` and
   `<projectRoot>/.claude/skills/`
6. Record registry plugin state in `.ddx/plugins.lock.yaml`

For simple resources (individual personas, templates):

The forward resource command is separate from plugin lifecycle work and must not
reuse the retired top-level `ddx install` surface.

1. Resolve the resource through the plugin/library registry
2. Materialize the persona into the project library area or configured persona
   target

### Cache and State

- **Registry cache:** `.ddx/cache/registries/<name>/registry.yaml` (one per registry)
- **Plugin state:** `.ddx/plugins.lock.yaml` (project plugins, versions, cache
  paths, timestamps, source registry)
- **Library cache:** `~/.cache/ddx/library/` (downloaded resources)
- **Plugin cache:** `~/.cache/ddx/library/plugins/` (populated for dun discovery)

## Requirements

### Functional

1. **Registry fetch** — download latest `registry.yaml` from ddx-library as part of plugin install/list/upgrade
2. **Search** (`ddx search <query>`) — search available resources by name, type, or keyword
3. **Install plugin** (`ddx plugin install <name>`) — download and cache a
   workflow/plugin package, write the project lock, and generate adapters
4. **Install local plugin overlay** (`ddx plugin install <name> --local <path>`) — symlink a project to a local checkout
5. **List installed plugins** (`ddx plugin list`) — show project plugins and local overlays
6. **Uninstall plugin** (`ddx plugin uninstall <name>`) — remove an installed plugin
7. **Populate plugin cache** — on install, copy plugin payloads to
   `${XDG_DATA_HOME}/ddx/cache/plugins/<name>/<version>/`
8. **Version tracking** — record installed versions, detect available updates
9. **Update detection** (`ddx plugin upgrade --check`, or the equivalent
   staleness surface) — compare installed package versions
   against source repo tags (via `git ls-remote --tags`) to determine if
   updates are available. Output: package name, installed version, latest
   available, update available (yes/no).
10. **Plugin upgrade** (`ddx plugin upgrade <name>`) — re-install a plugin at the
    latest available version. Performs a fresh shallow clone at the latest tag,
    refreshes the cache, updates `.ddx/plugins.lock.yaml`, and regenerates
    adapters. Safe to run repeatedly.
11. **Startup update check** — on `ddx` startup (async, non-blocking), check
    if installed packages have newer versions available. If so, print a
    one-line notice: `Plugin update available: helix 0.1.0 → 0.2.0 (run
    'ddx plugin upgrade helix')`. Same pattern as the existing DDx binary update
    check.

### Non-Functional

- **On-demand fetch** — fetch individual files or shallow clones, not full repo history
- **Offline-safe** — work from cache when offline; warn but don't fail
- **Idempotent** — running `ddx plugin install helix` twice is safe
- **Fast** — individual resource install <5s on broadband

## CLI Commands

```bash
ddx search <query>                  # Search available resources
ddx plugin install helix            # Install HELIX workflow/plugin
ddx plugin install helix --local ../helix  # Link local checkout for development
ddx plugin list                     # List project plugins
ddx plugin upgrade                  # Check/apply available plugin updates
ddx plugin upgrade <name>           # Update a registry plugin to latest version
ddx plugin uninstall <name>         # Remove an installed plugin
```

## User Stories

### US-090: Developer Discovers Available Workflows
**As a** developer evaluating DDx
**I want** to search for available workflow tools
**So that** I can find and install HELIX or other methodologies

**Acceptance Criteria:**
- Given I run `ddx search workflow`, then I see HELIX and any other registered workflows with descriptions
- Given I run `ddx plugin install helix`, then HELIX payloads are cached under
  XDG and HELIX skills are exposed through generated adapters in
  `.agents/skills/` and `.claude/skills/`

### US-091: Developer Installs Individual Resources
**As a** developer customizing my project
**I want** to install specific personas or templates from the library
**So that** I get exactly what I need without bulk downloading

**Acceptance Criteria:**
- Given I install an individual persona resource, then the persona file is
  copied to `.ddx/library/personas/`
- Given I run `ddx plugin list`, then I see installed project plugins with version and install date

## Dependencies

- `ddx-library` repo with `registry.yaml`
- GitHub API or git archive for fetching individual files
- Package descriptors in workflow tool repos (HELIX, etc.)

## Out of Scope

- Package signing or verification (v2)
- Automatic updates (manual `ddx plugin upgrade` for now)
- Dependency resolution between packages

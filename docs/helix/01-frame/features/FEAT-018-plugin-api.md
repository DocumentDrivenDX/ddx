---
ddx:
  id: FEAT-018
  depends_on:
    - helix.prd
    - FEAT-009
    - FEAT-015
---
# Feature: Plugin API Stability

**ID:** FEAT-018
**Status:** Not Started
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx plugins extend the platform with methodology-specific capabilities. The
plugin API is the set of contracts that plugin authors depend on: package
descriptors, directory layout, skill format, hook scripts, and bead
conventions. This feature documents the existing surfaces, adds schema
versioning, and commits to backward compatibility.

## Problem Statement

**Current situation:** The plugin extension surface exists but is implicit.
Package descriptors are embedded in Go code (BuiltinRegistry), not declared by
plugins. Skill format (SKILL.md) follows conventions but has no formal schema.
Hook scripts work but their invocation contract is undocumented.

**Pain points:**
- Plugin authors must read DDx source code to understand what's expected
- No versioning — DDx can change any surface without warning
- No validation — malformed plugins fail at runtime with unclear errors
- Package descriptors live in DDx Go code, not in plugin repos

**Desired outcome:** Plugin authors can read a single reference document,
validate their plugin against a schema, and trust that documented surfaces
won't break without a major version bump.

## Extension Surfaces

### 1. Package Descriptor

Currently embedded in `cli/internal/registry/registry.go` as Go structs.
Should move to a `package.yaml` in each plugin repo.

**Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Package identifier (e.g., `helix`) |
| `version` | string | yes | Semantic version |
| `description` | string | yes | One-line purpose |
| `type` | enum | yes | `workflow`, `plugin`, `persona-pack`, `template-pack` |
| `source` | string | yes | Repository URL |
| `api_version` | string | yes | DDx plugin API version (e.g., `1`) |
| `install.root` | mapping | no | Copy entire plugin to target |
| `install.skills` | []mapping | no | Skill symlink targets |
| `install.scripts` | mapping | no | CLI entrypoint symlink |
| `install.executable` | []string | no | Paths needing execute bit |
| `requires` | []string | no | DDx version constraints |
| `keywords` | []string | no | Search/discovery tags |

### 2. Plugin Directory Layout

```
<plugin-root>/
  package.yaml              # Package descriptor (new — replaces Go embedding)
  skills/                   # Canonical skill source (SKILL.md per skill)
  .agents/skills/           # Symlinks to skills/ for agent discovery
  .claude/skills/           # Symlinks to skills/ for Claude discovery
  workflows/                # Shared workflow library (optional)
  scripts/                  # CLI entrypoints (optional)
  bin/                      # Binary wrappers (optional)
  docs/                     # Plugin documentation (optional)
```

### 3. SKILL.md Format

**Frontmatter (YAML):**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Skill identifier (e.g., `ddx-bead`) |
| `description` | string | yes | One-line purpose |
| `argument-hint` | string | no | Optional shorthand usage hint for help or prompt flows |

**Body:** Markdown with sections describing when to use, steps, constraints,
and cross-references to shared workflow resources.

The bundled DDx skills already use this top-level frontmatter shape, and the
stable contract must preserve it so install and doctor validation accept the
shipped skills without migration. `argument-hint` is advisory only; it does not
change skill execution semantics.

### 4. Hook Scripts

Hooks are executable scripts in `.ddx/hooks/` invoked by DDx at specific
lifecycle points.

**Current hooks:**

| Hook | Trigger | Input | Expected behavior |
|------|---------|-------|-------------------|
| `validate-bead-create` | `ddx bead create` | Bead JSON on stdin | Exit 0 to allow, non-zero to reject with stderr message |

**Invocation contract:**
- DDx calls the hook with the operation data on stdin as JSON
- Hook stdout is ignored; stderr is shown to the user on failure
- Exit code 0 = allow, non-zero = reject
- Hooks must complete within 10 seconds
- `HELIX_SKIP_TRIAGE=1` bypasses validation hooks (for automation)

### 5. Bead Conventions

Plugins may define label and field conventions that their hooks enforce:

| Convention | Example | Enforced by |
|-----------|---------|-------------|
| Required labels | `helix`, `phase:build` | validate-bead-create hook |
| spec-id field | `FEAT-001` | validate-bead-create hook |
| acceptance field | Deterministic criteria | validate-bead-create hook |
| Phase labels | `phase:frame`, `phase:build`, etc. | validate-bead-create hook |
| Kind labels | `kind:implementation`, `kind:review` | Advisory (warning only) |

## Requirements

### Functional

1. **package.yaml support** — `ddx install` reads `package.yaml` from the
   plugin repo as an alternative to the built-in registry. Built-in registry
   entries serve as fallback when no `package.yaml` exists.
2. **API version field** — `package.yaml` declares `api_version: 1`. DDx
   validates compatibility on install.
3. **Plugin validation** (`ddx doctor --plugins`) — check installed plugins
   for structural issues: missing SKILL.md, broken symlinks, missing
   required fields.
4. **Extension surface documentation** — ship a reference document with DDx
   describing all surfaces, field types, and compatibility guarantees.
5. **Backward compatibility** — changes to documented surfaces follow semver:
   additions in minor versions, removals only in major versions.

### Non-Functional

- **Minimal surface** — expose only what plugins need. Don't add extension
  points speculatively.
- **Validation over convention** — prefer schema validation over naming
  conventions where possible.

## User Stories

### US-180: Plugin Author Creates a New Plugin
**As a** developer creating a DDx plugin
**I want** to read a reference document describing the plugin API
**So that** I can create a valid plugin without reading DDx source code

**Acceptance Criteria:**
- Given I read the plugin API reference, when I create a package.yaml and
  skills directory, then `ddx install --local /my/plugin` installs it
- Given my package.yaml has `api_version: 1`, when DDx is at a compatible
  version, then install succeeds

### US-181: Plugin Author Validates Their Plugin
**As a** plugin author checking my work
**I want** to run `ddx doctor --plugins`
**So that** I see structural issues before publishing

**Acceptance Criteria:**
- Given a plugin with a missing SKILL.md, when I run doctor, then it reports
  the missing file
- Given a plugin with broken skill symlinks, when I run doctor, then it
  reports the broken links

## Out of Scope

- Go-level plugin interfaces (plugins are file-based, not compiled)
- Plugin marketplace or hosting
- Plugin dependency resolution (plugins don't depend on other plugins)
- Runtime plugin loading (plugins are installed at setup time)

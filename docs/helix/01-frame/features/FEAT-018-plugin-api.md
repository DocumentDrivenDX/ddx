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
| `artifact_type_roots` | []string | no | Glob roots for artifact-type definitions when the plugin does not ship a `workflows/` tree (see Surface 6) |

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

### 6. Artifact Type Definitions

Plugins declare the artifact types they govern by shipping per-type
definition directories. DDx discovers these on install and at `ddx doc`
time; the discovered set is the authoritative source for prefix-to-type
resolution (see FEAT-005).

**Default discovery layout (mandated for new plugins):**

```
<plugin-root>/
  workflows/**/artifacts/<typeId>/
    meta.yml         # required — type metadata (see frontmatter shape below)
    template.md      # required — structural template / sidecar template
    prompt.md        # required — generation/evolution prompt
    example.md       # optional — worked example
```

`<typeId>` is the directory name and is the canonical type identifier
(e.g. `feature-specification`, `adr`, `solution-design`). The `**` glob
allows plugins to nest types under workflow phases or other organizational
subdirectories — DDx walks `workflows/` recursively and collects every
`artifacts/<typeId>/` folder it finds.

**Opt-in for plugins without a `workflows/` tree:**

Plugins that don't model a workflow tree (persona packs, template packs,
single-purpose plugins) declare additional roots in `package.yaml`:

```yaml
artifact_type_roots:
  - artifacts/        # scanned recursively; same <typeId>/{meta.yml,...} shape
  - extras/types/
```

Each entry is a path (relative to plugin root) that DDx scans for
`<typeId>/` subdirectories using the same shape. Globs (`**`) are
permitted. If `artifact_type_roots` is unset, DDx scans `workflows/**`
only.

**`meta.yml` frontmatter shape:**

```yaml
artifact:
  name: <Human-readable name>
  id: <typeId>            # required; must match parent directory name
  type: document           # required; document | sidecar
  prefix: <PREFIX>         # required; ID prefix this type owns (e.g. FEAT, ADR, MET)
  phase: <phase-id>        # optional; advisory grouping (e.g. frame, design)
  optional: <bool>         # optional; whether the artifact is required by its workflow

description: |
  One- or multi-line summary of the artifact's purpose.

output:
  location: <path-or-glob> # optional; default location new instances land at
  format: markdown | sidecar
  naming: <pattern>        # optional; filename convention

prompts:
  generation: prompt.md     # required; relative path within the type dir
  review: <inline-or-path>  # optional

template:
  file: template.md         # required

examples:
  - file: example.md        # optional
    description: <summary>
```

Unknown top-level keys are preserved on round-trip; type-specific
extensions (validation, variables, workflow hints) are pass-through and
do not affect DDx graph semantics.

**Discovery and conflict resolution:**

1. DDx walks every installed plugin's discovery roots and indexes every
   `<typeId>` it finds along with the `prefix` declared in `meta.yml`.
2. A given `prefix` may be declared by at most one type across all
   installed plugins. Duplicate prefixes are a `ddx doctor --plugins`
   error.
3. Prefix resolution at `ddx doc` time first consults the plugin index;
   when no plugin claims a prefix, DDx falls back to the conventional
   prefix table in FEAT-005.
4. The legacy `.ddx/library/artifacts/<type>/{template,create,evolve,check}.md`
   shape (pre-FEAT-018 HELIX layout) is recognized as a compat fallback:
   types found there are indexed with `id` and `prefix` inferred from the
   directory name, and resolution prefers the new-shape entry when both
   exist for the same prefix.

## Requirements

### Functional

1. **package.yaml support** — `ddx install` reads `package.yaml` from the
   plugin repo as an alternative to the built-in registry. Built-in registry
   entries serve as fallback when no `package.yaml` exists.
2. **API version field** — `package.yaml` declares `api_version: 1`. DDx
   validates compatibility on install.
3. **Plugin validation** (`ddx doctor --plugins`) — check installed plugins
   for structural issues: missing SKILL.md, broken symlinks, missing
   required fields, duplicate artifact-type prefixes, malformed `meta.yml`.
4. **Extension surface documentation** — ship a reference document with DDx
   describing all surfaces, field types, and compatibility guarantees.
5. **Backward compatibility** — changes to documented surfaces follow semver:
   additions in minor versions, removals only in major versions.
6. **Artifact-type discovery** — DDx discovers artifact types per the
   contract in Surface 6. The new shape is mandated for plugins authored
   against `api_version: 1`; the legacy
   `.ddx/library/artifacts/<type>/{template,create,evolve,check}.md`
   layout is supported as a compat path so the bundled HELIX library
   continues to work without migration.

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

---
ddx:
  id: SD-018
  depends_on:
    - FEAT-018
    - FEAT-004
    - FEAT-009
    - FEAT-011
    - FEAT-015
---
# Solution Design: Plugin API Stability

## Overview

This design defines the stable, file-based plugin API that DDx exposes to
plugin authors. The contract spans:

- the package manifest (`package.yaml`)
- the plugin directory layout
- skill packaging and `SKILL.md` frontmatter
- hook invocation contracts
- install-time and doctor-time validation

The goal is to make plugin authoring possible without reading DDx source code
while keeping the surface minimal and backward-compatible.

## Design Principles

- Prefer validation over convention where a machine-checkable rule exists.
- Keep the plugin contract file-based; do not add compiled plugin interfaces.
- Treat manifests as authoritative and preserve unknown fields for forward
  compatibility.
- Keep the discovery surface explicit: DDx only recognizes paths declared by
  the manifest or the documented directory layout.
- Make `ddx install` and `ddx doctor --plugins` use the same validator so the
  install path and the audit path cannot drift.

## Package Manifest

Each plugin repository or local plugin root contains a `package.yaml` at the
repository root.

### Required Fields

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `name` | string | yes | Stable package identifier |
| `version` | string | yes | Package release version |
| `description` | string | yes | One-line summary |
| `type` | enum | yes | `workflow`, `plugin`, `persona-pack`, or `template-pack` |
| `source` | string | yes | Canonical origin URL or local source identifier |
| `api_version` | scalar | yes | Plugin API schema version, canonical value `1` |

### Optional Fields

| Field | Type | Meaning |
|-------|------|---------|
| `install.root` | mapping | Copy or materialize the package root into the install target |
| `install.skills` | []mapping | Skill source/target mappings |
| `install.scripts` | mapping | CLI entrypoint source/target mapping |
| `install.executable` | []string | Relative paths that must retain execute permission |
| `requires` | []string | DDx version constraints |
| `keywords` | []string | Search and discovery tags |

### Schema Rules

- `api_version` gates the contract, not the package release version.
- DDx accepts the first API generation only: `api_version: 1`.
- The manifest parser preserves unknown fields so future additions do not
  break older tooling.
- `name` must be stable enough to use as the install directory name.
- `source` is provenance metadata, not a resolution hint; install behavior
  comes from the declared install mappings.

## Plugin Layout

The canonical plugin source tree is file-based and explicit:

```text
<plugin-root>/
  package.yaml
  skills/
  .agents/skills/
  .claude/skills/
  workflows/
  scripts/
  bin/
  docs/
```

### Layout Rules

- `skills/` is the canonical skill source owned by the plugin.
- `.agents/skills/` and `.claude/skills/` are discovery mirrors created from
  the canonical skill source.
- `workflows/` holds shared workflow resources when a plugin ships a workflow
  bundle.
- `scripts/` holds CLI entrypoints that DDx may symlink into the user's PATH.
- `bin/` is for wrapper binaries and shims.
- `docs/` is optional and does not affect validation unless referenced by the
  manifest.

The installed plugin root lives under the project-local DDx workspace, not in
the active work queue. The install target is:

```text
.ddx/plugins/<name>/
```

## Skill Contract

Each skill is a directory containing exactly one `SKILL.md` file.

### `SKILL.md` Frontmatter

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `skill.name` | string | yes | Skill identifier |
| `skill.description` | string | yes | One-line summary |
| `skill.args` | []mapping | no | Short usage hints for help text or prompts |

### Skill Rules

- The skill directory name must match the `skill.name` frontmatter value.
- `SKILL.md` must parse as YAML frontmatter plus Markdown body.
- The body is free-form Markdown and may document steps, constraints, and
  cross-references to shared workflow resources.
- `skill.args` is advisory only; it does not change execution semantics.

This contract keeps the existing nested `skill:` schema used by the bundled
DDx skills as the stable form. Install-time and doctor-time validation must
accept that shape so built-in skills are valid without any migration step.

The canonical skill body may reference shared HELIX or DDx workflow material,
but the manifest and frontmatter are the stable contract. Everything else is
documentation.

## Hook Contract

DDx hooks remain file-based executables under `.ddx/hooks/` and related
documented hook locations. FEAT-018 does not introduce a new compiled plugin
hook system.

### Execution Contract

- DDx passes one JSON document on stdin.
- Hook stdout is ignored.
- Hook stderr is shown only on failure or warning.
- Exit code `0` allows the operation.
- Exit code `1` blocks the operation with a hard error.
- Exit code `2` emits a warning and continues.
- Hooks must complete within 10 seconds.
- `HELIX_SKIP_TRIAGE=1` bypasses validation hooks for automation paths that
  need to avoid interactive guardrails.

This contract is shared by all documented hooks so install-time validation and
workflow validation can reuse the same execution model.

## Install Flow

Plugin installation follows one validator-backed path regardless of source.

1. Resolve the package source.
   - local filesystem path
   - git repository / release artifact
   - builtin registry fallback for legacy packages
2. Read `package.yaml` from the source root.
3. Validate `api_version` and required manifest fields.
4. Materialize the package into `.ddx/plugins/<name>/`.
5. Create relative symlinks for the declared discovery surfaces.
6. Ensure any declared executable paths preserve the execute bit.
7. Record the installed package for later doctor and update checks.

### Compatibility and Legacy Fallback

- If a source repository provides `package.yaml`, that manifest is authoritative.
- If a plugin has no manifest yet, DDx may fall back to the built-in registry
  entry for compatibility.
- The fallback path is migration-only and should not become a second source of
  truth.

## Validation Model

### Install-Time Validation

`ddx install` validates:

- required manifest fields are present
- `api_version` is supported
- declared source and target paths are structurally valid
- skill directories contain `SKILL.md`
- skill frontmatter includes `skill.name` and `skill.description`
- `SKILL.md` names match their directory names via `skill.name`
- executable targets exist and are executable when required
- symlink targets stay within the installed plugin root

Install validation is fail-fast. If the package is malformed, DDx stops before
writing a partial install record.

### Doctor-Time Validation

`ddx doctor --plugins` performs a read-only audit of installed plugins:

- missing `package.yaml`
- unsupported `api_version`
- broken symlinks
- missing `SKILL.md`
- malformed skill frontmatter
- mismatched `skill.name` values
- declared executable paths that lost the execute bit

Doctor reports structural issues only. It does not mutate the install.

## Compatibility Rules

### Manifest Versioning

- `api_version` is the compatibility gate.
- Additive fields are allowed within a given `api_version`.
- Removing or renaming fields requires a new `api_version`.
- Older DDx binaries should reject manifests whose `api_version` they do not
  understand rather than guessing.

### Skill Frontmatter Compatibility

- `name` and `description` are required for all stable skills.
- New optional frontmatter fields may be added without breaking older DDx
  builds.
- Unknown frontmatter fields are preserved in the parsed model for tooling
  that wants to inspect them later.

### Behavioral Compatibility

- Plugin authors should be able to add docs, extra skill metadata, or new
  manifest keys without affecting install behavior.
- Structural changes to the documented layout require a major API version
  bump.

## Migration Strategy

The current builtin registry remains a bridge, not the long-term contract.

- Existing built-in packages continue to work through registry fallback.
- New plugin packages should ship `package.yaml` and the documented directory
  layout from day one.
- Over time, the builtin registry should shrink to compatibility shims and
  bootstrap packages only.

This keeps the implementation incremental while moving the source of truth
into the plugin repository itself.

## Non-Goals

- Compiled plugin interfaces
- Runtime plugin loading
- Plugin dependency resolution
- Marketplace discovery or hosting
- Validation of arbitrary user scripts outside the documented hook contract

## Validation Matrix

The implementation that follows this design should be covered by tests that
exercise:

- manifest parsing and `api_version` rejection
- local-path plugin installation
- builtin-registry fallback for legacy packages
- skill frontmatter validation
- broken-symlink detection in `ddx doctor --plugins`
- execute-bit validation for declared scripts
- unknown-field preservation for manifests and skill frontmatter

Expected outcome: plugin authors can create a valid package from the docs,
DDx can validate it consistently on install and doctor, and old packages can
still be consumed during the migration period.

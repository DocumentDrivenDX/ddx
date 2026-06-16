---
title: Plugins
weight: 6
---

{{< maturity "beta" >}}

DDx plugins extend the platform with workflow methodologies, agent skills,
templates, prompts, checks, and MCP server definitions. HELIX is the reference
workflow plugin.

The forward install model is cache-backed, similar to `npx`: a project records
which plugin it wants, DDx resolves the payload into the shared XDG cache, and
DDx generates the small agent adapter paths needed by local harnesses.
Repositories do not need to check in full plugin payloads.

## Install Model

`ddx plugin install <name>` writes three kinds of state:

| State | Path | Commit? |
|-------|------|---------|
| Project intent and version pin | `.ddx/plugins.lock.yaml` | Yes |
| Plugin payload | `${XDG_DATA_HOME}/ddx/cache/plugins/<name>/<version>/` | No |
| Agent adapters | `.agents/skills/<skill>/`, `.claude/skills/<skill>/` | No |

Generated adapter paths are recreated by `ddx plugin sync`, `ddx init`, and
plugin-aware commands when they need a missing adapter. They are intentionally
ignored by git.

`.ddx/plugins/<name>` is reserved for local developer overlays created by
`ddx plugin install <name> --local <path>`. Normal registry installs do not
copy the plugin payload into `.ddx/plugins/`.

## Commands

```bash
ddx plugin install helix          # Pin HELIX and generate local adapters
ddx plugin list                   # Show project plugins and overlay state
ddx plugin show helix             # Show one plugin and sync missing adapters
ddx plugin sync                   # Recreate generated adapters from the lock/cache
ddx plugin install helix --local ../helix --force
ddx doctor --plugins              # Check lock/cache/adapter health
```

The retired top-level commands `ddx install`, `ddx installed`,
`ddx uninstall`, `ddx outdated`, and `ddx verify` are compatibility shims or
removed surfaces. Use the `ddx plugin` namespace for plugin lifecycle work.

## Package Layout

A plugin should keep source assets in normal repository paths and declare the
agent-facing install mappings in `package.yaml`:

```yaml
name: my-plugin
version: 1.0.0
description: A workflow plugin for DDx
type: workflow
source: https://github.com/you/my-plugin
install:
  root:
    source: "."
    target: ".ddx/plugins/my-plugin" # local-overlay target only
  skills:
    - source: "skills/"
      target: ".agents/skills/"
    - source: "skills/"
      target: ".claude/skills/"
```

For registry installs, `install.root` describes the package root to cache; it
does not cause a project-local payload copy. For local overlays, DDx links
`.ddx/plugins/<name>` and the skill adapters directly to the checkout.

## Skill References

Skills should reference resources relative to their package layout, not assume
that a full plugin tree exists inside the project. For files that must be read
at runtime, keep them inside the cached package and reference them from the
skill using stable package-relative instructions.

Avoid writing instructions that require users to commit generated adapters or
cached payloads. Clone portability comes from `.ddx/plugins.lock.yaml` plus
lazy sync.

## Local Development

For live plugin development:

```bash
ddx plugin install my-plugin --local ../my-plugin --force
ddx plugin list
ddx skills check .agents/skills/my-skill .claude/skills/my-skill
```

Local overlays are machine-local symlinks. They do not update
`.ddx/plugins.lock.yaml` and are not auto-committed.

## Publishing

DDx currently resolves registry packages from the built-in registry and tagged
source archives. A plugin is ready to publish when it has:

- a valid `package.yaml`
- at least one tagged release
- skills with top-level `name` and `description` frontmatter
- no manifest targets that escape the project root
- docs that teach `ddx plugin install`, not retired top-level install commands

---
ddx:
  id: ADR-027
  depends_on:
    - FEAT-011
    - FEAT-015
    - ADR-003
---
# ADR-027: Skill Install Topology and Resolution Precedence

**Status:** Accepted
**Date:** 2026-06-01
**Amended:** 2026-06-15

## Context

DDx ships the `ddx` skill as a package, and operators need to know which
install layer they are actually using. Before this decision, the install
topology was spread across implementation details and a handful of docs:

- the project plugin lock under the resolved DDx root;
- registry payloads under `${XDG_DATA_HOME}/ddx/cache/plugins/`;
- the baked-in default package embedded in the binary;
- the agent-facing skill outputs under `.agents/skills/` and
  `.claude/skills/`.

HELIX is now distributed as a marketplace plugin. That changes the practical
cost of the older "copy real files into every repository" model: installing
workflow plugins by copying full skill/template/artifact trees into each
project causes repository bloat and stale checked-in assets. DDx needs the
same operator experience as `npx`: declare the dependency, resolve it from an
XDG cache, and materialize only the local adapter files needed by the current
agent.

Without one documented precedence rule, `ddx doctor` could not tell whether a
project had a valid plugin lock, a missing cache payload, or missing generated
adapters, and the agent docs had no single place to describe where the skill
surfaces live.

## Decision

DDx resolves the default `ddx` package in this order:

1. project plugin lock entry plus cache payload
2. built-in cache payload materialized from the binary under
   `${XDG_DATA_HOME}/ddx/cache/plugins/ddx/<version>/`
3. baked-in package embedded in the binary

For registry/marketplace plugins, "project-local" means the project has a
durable dependency and lock entry, not necessarily a copied payload tree.
Payloads resolve from the XDG plugin cache; the baked-in layer exists only for
the default `ddx` package so the binary remains usable offline.

The built-in `ddx` package follows the same adapter shape as marketplace
plugins without becoming a normal project dependency: `ddx init` and
`ddx plugin sync` may materialize the embedded default package into the XDG
plugin cache, then create `.agents/skills/ddx` and `.claude/skills/ddx` as
generated shims/links to `skills/ddx` in that cache. They do not create
`.ddx/plugins/ddx`, do not write a project plugin-lock entry for `ddx`, and do
not require network access. If the cache is missing, the embedded package is the
fallback source used to recreate it.

DDx supports one forward registry install mode with generated agent-tier
outputs:

**Project dependency (default, `ddx plugin install <name>`):**
- Project metadata: a plugin intent/lock entry under the resolved DDx root.
- Plugin payload: resolved into `${XDG_DATA_HOME}/ddx/cache/plugins/<name>/<version>/`
  not copied into the repository.
- Agent-tier skill outputs: generated shims/links under
  `<project>/.agents/skills/<name>/` and `<project>/.claude/skills/<name>/`.
  These are generated files and should be ignored by git.

The top-level plugin lifecycle verbs (`ddx install`, `ddx installed`,
`ddx uninstall`, `ddx outdated`, and `ddx verify`) are retired compatibility
surfaces. New operator guidance must use `ddx plugin install`, `ddx plugin
list`, `ddx plugin show`, `ddx plugin sync`, and `ddx doctor --plugins`.

Global/home-directory plugin installs are not the forward model. Sharing
downloaded payloads across projects happens through the XDG cache; each project
records its own plugin intent and can regenerate its agent adapters from that
lock.

Unmanaged legacy home-directory skill installs are retired and should not be
created manually.

Local developer overlays remain the exception: `ddx plugin install <name>
--local <path>` may symlink directly to a checkout because the operator has
explicitly chosen machine-local development state.

## Consequences

- `ddx doctor` can report project plugin lock, cache, and generated adapter
  states distinctly.
- Operators can distinguish `ok`, `cache-missing`, and `shims-missing` for the
  project plugin lock.
- The docs can point agents and operators to the same project-local surfaces
  instead of describing competing home-directory and project-directory
  locations.
- The default package remains available offline through the embedded layer.
- The default package no longer needs a full copied skill tree in every project;
  projects get generated adapters that can be recreated from the XDG cache or
  the binary.
- Registry plugin payloads no longer need to be checked into every project.
  Clone portability comes from project metadata plus lazy materialization.
- Agent skill directories become generated adapter state. They may be removed
  and recreated by `ddx plugin sync`, `ddx init`, or the first command that
  needs the plugin.

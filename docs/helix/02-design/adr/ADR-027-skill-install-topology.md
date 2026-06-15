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

- the project-local package install under `<project>/.ddx/plugins/ddx/`;
- the global fallback under `${XDG_DATA_HOME}/ddx/global/plugins/ddx/`;
- the baked-in default package embedded in the binary;
- the agent-facing skill outputs under `.agents/skills/` and
  `.claude/skills/`.

HELIX is now distributed as a marketplace plugin. That changes the practical
cost of the older "copy real files into every repository" model: installing
workflow plugins by copying full skill/template/artifact trees into each
project causes repository bloat and stale checked-in assets. DDx needs the
same operator experience as `npx`: declare the dependency, resolve it from a
cache/global layer, and materialize only the local adapter files needed by the
current agent.

Without one documented precedence rule, `ddx doctor` could not tell whether a
project was using its own package copy or falling through to the global layer,
and the agent docs had no single place to describe where the skill surfaces
live.

## Decision

DDx resolves the default `ddx` package in this order:

1. project-local `<project>/.ddx/plugins/ddx/`
2. global `${XDG_DATA_HOME}/ddx/global/plugins/ddx/`
3. baked-in package embedded in the binary

For registry/marketplace plugins, "project-local" means the project has a
durable dependency and lock entry, not necessarily a copied payload tree.
Payloads resolve from the project cache, global install/cache, or the baked-in
default package where applicable. The baked-in layer exists only for the
default `ddx` package so the binary remains usable offline.

DDx supports two install modes with distinct agent-tier outputs:

**Project dependency (default, `ddx plugin install <name>`):**
- Project metadata: a plugin intent/lock entry under the resolved DDx root.
- Plugin payload: resolved into `${XDG_DATA_HOME}/ddx/cache/plugins/<name>/<version>/`
  or a compatible global install/cache entry, not copied into the repository.
- Agent-tier skill outputs: generated shims/links under
  `<project>/.agents/skills/<name>/` and `<project>/.claude/skills/<name>/`.
  These are generated files and should be ignored by git.

**Global (`ddx install <name> --global`):**
- Plugin content: `${XDG_DATA_HOME}/ddx/global/plugins/<name>/`
- Agent-tier skill outputs: `~/.agents/skills/<name>/` and
  `~/.claude/skills/<name>/`

The `--global` surface enables machine-wide installs so operators can share a
single skill across every project on the machine without per-project setup.
State for global installs is recorded in a separate global state file
(`${XDG_DATA_HOME}/ddx/global/installed.json`) and does not pollute per-project
state. The `ddx plugin list --global` and `ddx plugin show <name> --global`
commands enumerate from the global state.

Unmanaged legacy home-directory skill installs (those not created by DDx's
`--global` surface) are retired and should not be created manually.

Local developer overlays remain the exception: `ddx plugin install <name>
--local <path>` may symlink directly to a checkout because the operator has
explicitly chosen machine-local development state.

## Consequences

- `ddx doctor` can report the project install and the global fallback as
  distinct states.
- Operators can distinguish `ok`, `missing`, and
  `lazy-resolves-to-global` for the project layer.
- The docs can point agents and operators to the same project-local surfaces
  instead of describing competing home-directory and project-directory
  locations.
- The default package remains available offline through the embedded layer.
- `ddx install --global` satisfies the common case where an operator wants a
  skill on every project without repeating the install per repository.
- Registry plugin payloads no longer need to be checked into every project.
  Clone portability comes from project metadata plus lazy materialization.
- Agent skill directories become generated adapter state. They may be removed
  and recreated by `ddx plugin sync`, `ddx init`, or the first command that
  needs the plugin.

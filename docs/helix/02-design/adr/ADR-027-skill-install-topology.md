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

## Context

DDx ships the `ddx` skill as a package, and operators need to know which
install layer they are actually using. Before this decision, the install
topology was spread across implementation details and a handful of docs:

- the project-local package install under `<project>/.ddx/plugins/ddx/`;
- the global fallback under `${XDG_DATA_HOME}/ddx/global/plugins/ddx/`;
- the baked-in default package embedded in the binary;
- the agent-facing skill outputs under `.agents/skills/` and
  `.claude/skills/`.

Without one documented precedence rule, `ddx doctor` could not tell whether a
project was using its own package copy or falling through to the global layer,
and the agent docs had no single place to describe where the skill surfaces
live.

## Decision

DDx resolves the default `ddx` package in this order:

1. project-local `<project>/.ddx/plugins/ddx/`
2. global `${XDG_DATA_HOME}/ddx/global/plugins/ddx/`
3. baked-in package embedded in the binary

The project-local package is the authoritative install for a repository. The
global layer is a fallback and may be used when the project copy is absent.
The baked-in layer exists only for the default `ddx` package so the binary
remains usable offline.

DDx supports two install modes with distinct agent-tier outputs:

**Project-local (default, `ddx install <name>`):**
- Plugin content: `<project>/.ddx/plugins/<name>/` (in-tree) or
  `${XDG_DATA_HOME}/ddx/projects/<identity>/plugins/<name>/` (convention mode)
- Agent-tier skill outputs: `<project>/.agents/skills/<name>/` and
  `<project>/.claude/skills/<name>/`

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


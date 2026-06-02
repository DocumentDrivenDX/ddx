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

The agent-facing skill outputs are project-local and live in:

- `<project>/.agents/skills/ddx/`
- `<project>/.claude/skills/ddx/`

Home-directory skill installs are retired. DDx does not add new state under
`~/.agents/skills` or `~/.claude/skills`.

## Consequences

- `ddx doctor` can report the project install and the global fallback as
  distinct states.
- Operators can distinguish `ok`, `missing`, and
  `lazy-resolves-to-global` for the project layer.
- The docs can point agents and operators to the same project-local surfaces
  instead of describing competing home-directory and project-directory
  locations.
- The default package remains available offline through the embedded layer.


---
ddx:
  id: SD-011
  depends_on:
    - FEAT-011
    - FEAT-001
    - FEAT-015
    - ADR-001
---
# Solution Design: DDx Agent Skills

> **Updated 2026-04-20.** FEAT-011 consolidated the earlier 4-skill layout
> (`ddx-bead`, `ddx-agent`, `ddx-install`, `ddx-status`) into a single
> `ddx` skill with an intent router and per-topic reference files.

## Overview

DDx ships a single agent-facing skill — `ddx` — that provides guidance
for operating every DDx CLI surface: beads, the queue, executions,
agents, harnesses, personas, reviews, and installation. The skill body
is an intent router; the real domain guidance lives under
`reference/*.md` files loaded on demand.

Skills are plain-Markdown guidance wrappers over DDx CLI commands. They
carry no compiled code or runtime dependencies — an agent reads the
skill and follows its instructions by invoking `ddx` CLI commands
directly.

## Skill Format

> **FEAT-015 amendment (2026-05-12):** Skill directories are project-local under
> `<projectRoot>/.agents/skills/` and `<projectRoot>/.claude/skills/` as installer
> outputs from the package-owned `library/skills/ddx/`. Home-directory skill paths
> are retired. The layout below uses the current project-local model.

```
library/skills/ddx/       # package-owned canonical source
├── SKILL.md
├── evals/
│   └── routing.jsonl
└── reference/
    ├── beads.md
    ├── agents.md
    ├── executions.md
    ├── personas.md
    └── ...

.agents/skills/ddx/       # project-local (installed by package installer)
.claude/skills/ddx/       # project-local (installed by package installer)
```

### SKILL.md Frontmatter

The skill uses the top-level frontmatter schema enforced by
`ddx skills check` (AGENTS.md §Skill Policy):

```yaml
---
name: ddx
description: Operates the DDx toolkit for document-driven development. ...
---
```

- `name` — exactly matches the directory name (`ddx`).
- `description` — intent triggers keyed to user phrasing ("drain the
  queue", "run a bead", "create a bead", etc.). The description is
  load-bearing for router selection by skills-aware agents.
- `argument-hint` — optional; used only when the skill takes a
  trailing positional or shorthand invocation hint.
- **Nested `skill:` metadata is rejected.** The DDx skill uses
  top-level fields only.

### SKILL.md Body

The body opens with an overview and then an **intent router** — a
table mapping user phrasing to the matching `reference/<topic>.md`
file. The directive to the agent is strict: load the matching
reference file before responding to a DDx-related request.

Reference files cover:

- `reference/beads.md` — bead CRUD, dependencies, claims, evidence
- `reference/agents.md` — power-bound dispatch, passthrough constraints,
  `ddx run`, `ddx try`, and `ddx work`
- `reference/executions.md` — execution definitions and immutable run
  history (`ddx metric` / `ddx exec`)
- `reference/personas.md` — persona listing, show, binding
- `reference/install.md` — plugin and skills install flows
- additional topics as DDx surfaces grow

## Installation Mechanism

### Package-Owned Source

Canonical skill source lives in `library/skills/ddx/`. The binary embeds
the entire default package library via `//go:embed` at
`cli/internal/registry/defaultplugin/library/` (FEAT-011, plan-2026-05-13)
so the skill ships with every DDx release and never requires a separate
download.

### Project-Local Install (`ddx init`)

`ddx init` installs the default `ddx` plugin through the embedded package
installer, writing project-local real files (not symlinks) to
`.agents/skills/ddx/` and `.claude/skills/ddx/` for the two major skill
runtimes. Real files are installed (not symlinked to global) so project
worktrees can evolve independently and remain git-portable.

### Local Development Overlay

For development, `ddx plugin install ddx --local library --force` creates
project-local symlinks to the source for live editing without auto-committing.

### Plugin-Declared Skills (`ddx plugin install <plugin>`)

Plugins may declare additional skills in their `package.yaml`. The
registry installer materializes real files in `.agents/skills/` and
`.claude/skills/`, while `ddx plugin install <plugin> --local <path>`
materializes direct symlinks to the local checkout for developer overlays.
Both modes prune stale plugin-owned skill entries from prior plugin versions
(FEAT-015 AC-004 / AC-013, tracked by `ddx-20fe27c7`).

### Manual Management

Users may edit or replace the skill files directly. `ddx init` does
not overwrite manually modified files unless `--force` is passed.

## CLI Invocation Pattern

Reference files invoke the `ddx` binary on `$PATH`. They do not
shell-expand or hard-code paths. If `ddx` is absent, the agent emits a
clear error and halts. All CLI calls use structured flags — no
positional argument guessing.

## Validation

- `ddx skills check [path ...]` validates SKILL.md frontmatter for any
  skill tree: top-level `name`, top-level `description`, optional
  `argument-hint`, rejects nested `skill:` metadata, requires a
  non-empty body.
- `make skill-schema` (at `cli/Makefile:82`) runs `ddx skills check`
  against the canonical source (`library/skills/ddx`) and the embedded
  default-package copy (`cli/internal/registry/defaultplugin/library/skills/ddx`).
  Pre-commit and CI both enforce this gate.
- Unit tests verify that the embedded default package tree parses cleanly.

## Testing Strategy

- Static validation of every bundled `SKILL.md` via
  `ddx skills check` against the source and embedded copy.
- Router evals: `library/skills/ddx/evals/routing.jsonl` contains labelled
  user phrasings and expected reference-file selections. The eval is
  the regression harness for router drift.
- Integration tests for `ddx init` assert the skill directories exist
  at `.agents/skills/ddx/` and `.claude/skills/ddx/` and contain readable
  `SKILL.md` files after initialization.
- No end-to-end agent execution tests — skill correctness is
  validated by inspecting the skill content and router evals, not by
  running an agent.

## Non-Goals

- Workflow-specific skills (HELIX provides those under its own
  install path; FEAT-011 stays platform-agnostic).
- Skills for commands that need no guidance (`ddx version`,
  `ddx upgrade`).
- Interactive TUI or GUI — skills are agent-facing Markdown.
- Compiled skill logic — all intelligence lives in CLI commands, not
  skill files.

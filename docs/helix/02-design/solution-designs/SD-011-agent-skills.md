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

> **FEAT-015 amendment:** Skill directories are project-local under
> `<projectRoot>/.agents/skills/`. Home-directory skill paths are retired.
> The layout below uses the current project-local model.

```
.agents/skills/ddx/   # project-local (FEAT-015)
├── SKILL.md
├── evals/
│   └── routing.jsonl
└── reference/
    ├── beads.md
    ├── agents.md
    ├── executions.md
    ├── personas.md
    └── ...
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
- `reference/agents.md` — harness dispatch, profiles, `ddx agent run`,
  execute-bead / execute-loop (alias `ddx work`)
- `reference/executions.md` — execution definitions and immutable run
  history (`ddx metric` / `ddx exec`)
- `reference/personas.md` — persona listing, show, binding
- `reference/install.md` — plugin and skills install flows
- additional topics as DDx surfaces grow

## Installation Mechanism

### Embedded Source

Skill source lives in `cli/internal/skills/ddx/`. The binary embeds
the tree via `//go:embed` (FEAT-011) so the skill ships with every
DDx release and never requires a separate download.

### Project-Local Install (`ddx init`)

`ddx init` writes a project-local copy into `.ddx/skills/ddx/` and
registers skill symlinks under `.agents/skills/` and `.claude/skills/`
for the two major skill runtimes. Real files are copied (not
symlinked to global) so project worktrees can evolve independently.

### Global Install (`ddx install --global`)

> **Retired (FEAT-015):** `ddx install --global` has been removed. Skills are
> installed project-locally via `ddx init`. No home-directory writes occur.

### Plugin-Declared Skills (`ddx install <plugin>`)

Plugins may declare additional skills in their `package.yaml`. The
installer materializes relative symlinks from `.agents/skills/` and
`.claude/skills/` into the plugin's skill directories and prunes
stale links from prior plugin versions (FEAT-015 AC-004 / AC-013,
tracked by `ddx-20fe27c7`).

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
  against both the canonical source (`skills/ddx`) and the embedded
  copy (`cli/internal/skills/ddx`). Pre-commit and CI both enforce
  this gate.
- Unit tests in `cli/internal/skills/` verify the embedded tree
  parses cleanly.

## Testing Strategy

- Static validation of every bundled `SKILL.md` via
  `ddx skills check`.
- Router evals: `skills/ddx/evals/routing.jsonl` contains labelled
  user phrasings and expected reference-file selections. The eval is
  the regression harness for router drift.
- Integration tests for `ddx init` assert the skill directory exists
  and contains a readable `SKILL.md` after initialization.
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

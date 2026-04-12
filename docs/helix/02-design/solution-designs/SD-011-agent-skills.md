---
ddx:
  id: SD-011
  depends_on:
    - FEAT-011
    - FEAT-001
    - ADR-001
---
# Solution Design: DDx Agent Skills

## Overview

This design specifies the four core DDx skills (`ddx-bead`, `ddx-agent`,
`ddx-install`, `ddx-status`), their installation mechanism, the SKILL.md
format, and how `ddx init` registers them automatically.

Skills are plain-Markdown guidance wrappers over DDx CLI commands. They carry
no compiled code or runtime dependencies — an agent reads the SKILL.md and
follows its instructions by invoking `ddx` CLI commands directly.

## Skill Format

Each skill is a directory with a single `SKILL.md` file:

```
~/.agents/skills/
├── ddx-bead/SKILL.md
├── ddx-agent/SKILL.md
├── ddx-install/SKILL.md
└── ddx-status/SKILL.md
```

### SKILL.md Frontmatter

```yaml
---
skill:
  name: ddx-bead
  description: Guided bead creation and triage
  args:
    - name: title
      description: Short title for the new bead
      required: false
---
```

The `skill` key is the conventional frontmatter block. `name` matches the
directory name. `description` is shown in skill listings. `args` declares
argument hints that the agent uses to prompt for missing inputs.

### SKILL.md Body

The body provides step-by-step guidance the agent follows. It references DDx
CLI commands by exact invocation. The body may include:

- Ordered steps the agent executes
- Validation checks before running commands
- Fallback behavior when `ddx` is not on `$PATH` (emit a clear error and stop)
- Context hints such as where to find governing documents

## Core Skills

### `ddx-bead`

Wraps `ddx bead create`, `ddx bead update`, and `ddx bead dep add`.

The skill prompts for title, type, description, and acceptance criteria. It
reads `.ddx/hooks/` to determine which labels are required by the project's
hook configuration and includes them automatically. It suggests a `--spec-id`
by scanning nearby governing documents. It assembles and runs the final
`ddx bead create` command with all required flags.

### `ddx-agent`

Wraps `ddx agent run` and `ddx agent log`.

The skill calls `ddx agent list` to enumerate available harnesses, then
`ddx agent capabilities <harness>` to show model and effort options. It guides
model and effort selection based on task complexity, assembles the full
`ddx agent run` command, and surfaces the result via `ddx agent log`.

### `ddx-install`

Wraps `ddx search` and `ddx install`.

The skill runs `ddx search <query>` to surface matching packages, shows the
package description and install targets, waits for confirmation, then runs
`ddx install <package>`. It verifies post-install health with `ddx doctor`.

### `ddx-status`

Wraps `ddx status`, `ddx doctor`, and `ddx bead list`.

The skill runs all three commands in sequence and presents a unified project
health summary: DDx version and sync state, health check results, and a count
of open, ready, and blocked beads.

## Installation Mechanism

### Embedded Source

Skill source files are embedded in the DDx binary under
`cli/internal/skills/`. Each skill is a directory containing its `SKILL.md`.
The binary writes them via the existing embedded-resource mechanism used for
other bundled assets.

### `ddx init` Auto-Registration

When `ddx init` runs, it creates `~/.agents/skills/` if absent and writes each
bundled skill as a directory with its `SKILL.md`. Existing skill files are not
overwritten unless the installed version predates the binary's bundled version
(compared by a `version` field in the frontmatter).

### `ddx install ddx-skills`

`ddx install ddx-skills` (via FEAT-009) fetches the latest skill versions from
the registry and writes them to `~/.agents/skills/`. This is the upgrade path
for projects that do not want to upgrade the full DDx binary.

### Manual Management

Users may edit or replace skill files directly. `ddx init` does not overwrite
manually modified files unless the `--force` flag is passed.

## CLI Invocation Pattern

Skills invoke the `ddx` binary on `$PATH`. They do not shell-expand or
hard-code paths. If `ddx` is absent, the skill emits:

```
ddx is not installed or not on $PATH. Install from https://github.com/org/ddx.
```

and halts. All CLI calls use structured flags — no positional argument guessing.

## Integration with `ddx init`

`ddx init` gains a `skills` step after the existing initialization steps:

1. Detect `~/.agents/skills/` (create if absent).
2. For each bundled skill, write `~/.agents/skills/<name>/SKILL.md`.
3. Report which skills were written or skipped.

The step is non-fatal: if `~/.agents/` is not writable, `ddx init` logs a
warning and continues.

## Testing Strategy

- Unit tests verify that each bundled SKILL.md parses valid frontmatter
  (`skill.name`, `skill.description` present; `args` is a list if provided).
- Integration tests for `ddx init` assert that skill directories exist and
  contain a readable `SKILL.md` after initialization.
- Acceptance test for `ddx install ddx-skills` verifies that skills are written
  to `~/.agents/skills/` and match the registry manifest.
- No end-to-end agent execution tests — skill correctness is validated by
  inspecting the SKILL.md content, not by running an agent.

## Non-Goals

- Workflow-specific skills (HELIX provides those under its own install path).
- Skills for commands that need no guidance (`ddx version`, `ddx upgrade`).
- Interactive TUI or GUI — skills are agent-facing Markdown.
- Compiled skill logic — all intelligence lives in CLI commands, not skill files.

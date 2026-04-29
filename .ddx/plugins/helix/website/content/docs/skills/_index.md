---
title: Skills
weight: 4
prev: /docs/cli
next: /docs/glossary
---

HELIX publishes agent skills that mirror the CLI commands. Skills are the
interface between AI agents and the HELIX workflow — each skill describes
when to activate and what to do.

## Installed Skills

| Skill | CLI Command | Purpose |
|-------|-------------|---------|
| `helix-run` | `helix run` | Supervisory autopilot |
| `helix-build` | `helix build` | One bounded implementation pass |
| `helix-check` | `helix check` | Queue-health decision |
| `helix-frame` | `helix frame` | Requirements and feature specs |
| `helix-design` | `helix design` | Design document creation |
| `helix-evolve` | `helix evolve` | Thread changes through artifact stack |
| `helix-review` | `helix review` | Fresh-eyes post-implementation review |
| `helix-align` | `helix align` | Top-down reconciliation audit |
| `helix-polish` | `helix polish` | Issue refinement |
| `helix-triage` | `helix triage` | Create well-structured issues |
| `helix-next` | `helix next` | Show recommended next issue |
| `helix-experiment` | `helix experiment` | Metric-driven optimization |
| `helix-commit` | `helix commit` | Commit with build gate |
| `helix-worker` | `helix run` (background) | Launch and monitor background runs |
| `helix-backfill` | `helix backfill` | Reconstruct missing docs |

### Aliases

- `helix-implement` -> `helix-build`
- `helix-plan` -> `helix-design`

## Skill Structure

Each skill lives in `skills/<name>/SKILL.md` with YAML frontmatter:

```yaml
---
name: helix-build
description: 'Execute one bounded HELIX build pass.'
argument-hint: "[issue-id|scope]"
---
```

Skills reference shared workflow resources from `workflows/` for action
prompts, templates, and conventions.

## Installation Modes

### Plugin Mode (recommended)

```bash
claude --plugin-dir /path/to/helix
```

Skills are auto-discovered from `skills/`. No manual steps needed.

### Symlink Mode (legacy)

```bash
scripts/install-local-skills.sh
```

Creates symlinks in `~/.claude/skills/` and `~/.agents/skills/`.

## Naming Convention

- Public skill names are `helix-<command>`
- `<command>` must match the CLI subcommand exactly
- Every published SKILL.md declares `name` and `description`
- Skills accepting a trailing argument declare `argument-hint`

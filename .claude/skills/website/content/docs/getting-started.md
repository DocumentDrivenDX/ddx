---
title: Getting Started
weight: 1
prev: /docs
next: /docs/concepts
---

Get DDx installed and start tracking work in under 5 minutes.

{{< asciinema src="07-quickstart" cols="100" rows="30" >}}

## Install

Run the install script to set up DDx globally:

```bash
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
```

This installs:
- `ddx` CLI binary to `~/.local/bin/ddx`
- DDx skills to `~/.ddx/skills/`
- Symlinks in `~/.agents/skills/` and `~/.claude/skills/` for Claude Code

Verify the installation:

```bash
ddx version
ddx doctor
```

## Initialize a Project

In your project directory, run:

```bash
ddx init
```

This creates:
- `.ddx/` - DDx configuration and project-local skills
- `.ddx/skills/` - Bootstrap skills (ddx-doctor, ddx-run)
- `.agents/skills` → `.ddx/skills` - Symlink for Claude Code

## Install HELIX Workflow

```bash
ddx install helix
```

This installs HELIX to `~/.ddx/plugins/helix/` and adds its skills to your skill search path.

## Track Work

```bash
ddx bead create "Build login page" --type task
ddx bead create "Add auth middleware" --type task
ddx bead list
ddx bead ready
```

## Run Agents

```bash
ddx agent run --harness claude --prompt task.md
ddx agent usage
```

## Update

Check for updates:

```bash
ddx update --check     # Check all
ddx update ddx         # Update DDx CLI
ddx update helix      # Update HELIX plugin
```

## Next Steps

- [CLI reference](../cli) — all commands
- [Ecosystem](../ecosystem) — how DDx fits with HELIX and other tools
- [Creating plugins](../plugins) — add your own workflow to the registry

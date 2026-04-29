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

This installs the `ddx` CLI binary to `~/.local/bin/ddx`. DDx skills
are installed per-project by `ddx init` and `ddx install <plugin>` —
nothing is written under `~/` outside the binary itself.

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
- `.ddx/` - DDx configuration and project-local plugin tree
- `.agents/skills/` and `.claude/skills/` - copied skill files for
  Claude Code (real files, no symlinks)

## Install HELIX Workflow

```bash
ddx install helix
```

This installs HELIX to `.ddx/plugins/helix/` and copies its skills
into the project's `.agents/skills/` and `.claude/skills/` trees.

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

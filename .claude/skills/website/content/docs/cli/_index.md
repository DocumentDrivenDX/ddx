---
title: CLI Reference
weight: 3
---

For the full auto-generated reference — one page per command with all flags and
subcommands — see the [Command Reference](/docs/cli/commands/).

## Quick Reference

### Setup

```bash
ddx init              # Initialize DDx in your project
ddx install helix     # Install a workflow plugin
ddx doctor            # Check installation health
ddx upgrade           # Upgrade DDx binary
```

### Beads (Work Tracker)

The bead tracker is the core of DDx. Beads are work items with dependencies,
claims, and status. Workflow tools like HELIX use beads to drive execution.

```bash
ddx bead create "Title" --type task
ddx bead list
ddx bead show <id>
ddx bead ready              # unblocked beads
ddx bead blocked            # beads waiting on deps
ddx bead update <id> --claim
ddx bead close <id>
ddx bead dep add <id> <dep>
ddx bead dep tree <id>
```

### Execution Engine

Define reusable execution definitions and run them with recorded evidence.

```bash
ddx exec define <name> --artifact <id> --command "go test ./..."
ddx exec run <id>
ddx exec list
ddx exec history --artifact <id>
ddx exec result <run-id>
ddx exec log <run-id>
```

### Agent Dispatch

```bash
ddx agent run --harness claude --prompt file.md
ddx agent run --quorum majority --harnesses codex,claude --text "Review this"
ddx agent list
ddx agent usage
ddx agent capabilities claude
```

### Package Registry

```bash
ddx search <query>
ddx install <name>
ddx installed
ddx uninstall <name>
```

### Documents

```bash
ddx doc history <id>
ddx doc changed --since HEAD~5
ddx checkpoint <name>
ddx list
```

### Configuration

```yaml
# .ddx/config.yaml
agent:
  harness: claude
  permissions: safe         # safe | supervised | unrestricted
git:
  auto_commit: never        # always | prompt | never
```

### Global Flags

| Flag | Description |
|------|------------|
| `-v` | Verbose output |
| `--config` | Config file path |
| `--help` | Show help |

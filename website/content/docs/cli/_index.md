---
title: CLI Reference
weight: 3
---

Complete command reference for the `ddx` CLI.

## Setup

```bash
ddx init                    # Initialize DDx in your project
ddx doctor                  # Validate installation health
ddx upgrade                 # Upgrade DDx binary to latest
ddx status                  # Show version and project info
```

## Package Registry

```bash
ddx search <query>          # Search available packages
ddx install <name>          # Install a package
ddx installed               # List installed packages
ddx uninstall <name>        # Remove a package
```

## Beads (Work Tracker)

```bash
ddx bead create "Title" --type task    # Create a work item
ddx bead list               # List all beads
ddx bead show <id>          # Show bead details
ddx bead ready              # Show unblocked beads
ddx bead close <id>         # Close a completed bead
ddx bead dep add <id> <dep> # Add dependency
ddx bead dep tree <id>      # Show dependency tree
```

## Agent Dispatch

```bash
ddx agent run --harness claude --prompt file.md   # Run an agent
ddx agent run --quorum majority --harnesses codex,claude --text "Review this"
ddx agent list              # Available harnesses
ddx agent capabilities claude  # Model and effort options
ddx agent usage             # Token consumption summary
ddx agent log               # Session history
```

## Documents

```bash
ddx doc graph               # Show dependency graph
ddx doc stale               # List stale documents
ddx doc stamp <path>        # Mark as reviewed
ddx doc history <id>        # Git history for an artifact
ddx doc diff <id>           # Diff since last stamp
ddx doc changed --since HEAD~5  # Recently changed artifacts
ddx checkpoint <name>       # Create a named checkpoint
```

## Library

```bash
ddx list                    # Browse library contents
ddx prompts list            # Browse prompts
ddx persona list            # Browse personas
ddx persona bind <role> <name>  # Bind persona to role
```

## Configuration

```bash
ddx config set <key> <val>
ddx config get <key>
```

### Agent Config (`.ddx/config.yaml`)

```yaml
agent:
  harness: claude              # default harness
  permissions: safe            # safe | supervised | unrestricted
  models:
    codex: o3-mini
    claude: claude-sonnet-4-20250514
git:
  auto_commit: never           # always | prompt | never
  checkpoint_prefix: ddx/
```

## Global Flags

| Flag | Description |
|------|------------|
| `-v`, `--verbose` | Verbose output |
| `--config` | Config file path |
| `--help` | Show help |

---
title: CLI Reference
weight: 3
# hand-curated; do not auto-regenerate
# This page mirrors the CLI Command Overview in CLAUDE.md. The auto-generated
# per-command pages live under /docs/cli/commands/ and are regenerated from the
# Cobra command tree — this index is maintained by hand and must not be
# overwritten by that regeneration process.
---

<!-- hand-curated; do not auto-regenerate -->

This is a curated, noun-verb tour of the `ddx` CLI. For the full
auto-generated reference — one page per command with every flag and
subcommand — see the [Complete command reference](/docs/cli/commands/).

## Core

Lifecycle and health of a DDx-enabled project.

```bash
ddx init              # Initialize DDx in a project
ddx doctor            # Check installation health and diagnose issues
ddx upgrade           # Upgrade DDx binary to latest release version
ddx update            # Update toolkit resources from master repository
ddx contribute        # Share improvements back to community
ddx status            # Show version and sync status
```

## Bead Tracker

Beads are work items with dependencies, claims, and status. The tracker is
the substrate workflow tools (like HELIX) drive execution against.

```bash
# CRUD
ddx bead create "Title" --type task
ddx bead show <id>
ddx bead update <id> --claim
ddx bead close <id>

# Query and filter
ddx bead list
ddx bead ready              # unblocked beads
ddx bead blocked            # beads waiting on deps
ddx bead status

# Dependency DAG
ddx bead dep add <id> <dep>
ddx bead dep remove <id> <dep>
ddx bead dep tree <id>

# JSONL interchange with bd / br
ddx bead import <file>
ddx bead export
```

## Queue Work

Drain the bead execution queue. `ddx work` is an alias for
`ddx agent execute-loop`; all flags pass through.

```bash
ddx work                    # drain the bead execution queue
```

## Agent Service

Invoke AI agents against prompts, run multi-agent consensus, or execute a
bead end-to-end in an isolated worktree.

```bash
ddx agent run --harness=<name> --prompt <file>
ddx agent run --quorum=majority --harnesses=a,b
ddx agent execute-bead <id> [--from <rev>] [--no-merge]
ddx agent execute-loop          # drain bead queue (prefer `ddx work`)
ddx agent list                  # available harnesses
ddx agent doctor                # harness health check
ddx agent log                   # session history
```

## Resource Commands

Browse and apply the document-library resources DDx ships and syncs.

```bash
ddx prompts list
ddx prompts show <name>

ddx templates list
ddx templates apply <name>

ddx persona list
ddx persona show <name>
ddx persona bind <role> <persona>

ddx mcp list
ddx mcp install <name>
```

## Embedded Utilities

Utilities that ship inside the `ddx` binary so projects don't need extra
tooling on PATH.

```bash
ddx jq <filter> [file...]   # embedded jq processor (via gojq)
```

## Global Flags

| Flag | Description |
|------|------------|
| `-v` | Verbose output |
| `--config` | Config file path |
| `--help` | Show help |

For every command, subcommand, and flag, see the
[Complete command reference](/docs/cli/commands/).

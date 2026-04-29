---
title: DDx Skills
weight: 7
---

DDx ships 6 built-in skills that agents discover as slash commands. These
guide agents through DDx operations instead of requiring them to memorize
flag combinations.

## Available Skills

| Skill | Description |
|-------|-------------|
| `/ddx-bead` | Create and manage beads with proper metadata |
| `/ddx-agent` | Select harness, model, and effort for agent dispatch |
| `/ddx-install` | Search, preview, and install packages |
| `/ddx-status` | Project health overview |
| `/ddx-review` | Multi-agent quorum and fresh-eyes code review |
| `/ddx-run` | Execute a bead end-to-end: claim → build → verify → close |

Skills are installed automatically by `ddx init` to the project's
`.agents/skills/` and `.claude/skills/` directories as real files.

## `/ddx-bead` — Create Work Items

Instead of remembering flags, invoke the skill:

```
/ddx-bead "Implement user login"
```

The skill guides the agent through:
- Choosing type (task, epic, bug, chore)
- Setting labels and spec-id
- Writing acceptance criteria
- Wiring dependencies

## `/ddx-review` — Multi-Agent Review

Run a structured code review across multiple harnesses:

```
/ddx-review
```

The skill:
1. Checks available harnesses via `ddx agent list`
2. Assembles a review prompt with relevant context
3. Dispatches via `ddx agent run --quorum majority --harnesses codex,claude`
4. Reports where models agree and disagree

### Example: Review a bead's implementation

```
/ddx-review bead ddx-abc123
```

The skill reads the bead's spec-id and acceptance criteria, then reviews
the implementation against them.

## `/ddx-run` — Execute a Bead

The full lifecycle for implementing a work item:

```
/ddx-run ddx-abc123
```

The skill:
1. Reads the bead's spec-id and acceptance criteria
2. Loads the governing artifact
3. Claims the bead (`ddx bead update <id> --claim`)
4. Dispatches an agent with full context
5. Verifies tests pass after implementation
6. Closes the bead if acceptance criteria are met

This prevents agents from skipping verification or claiming completion
without running tests.

## `/ddx-agent` — Guided Dispatch

When you need to run an agent but aren't sure which model or effort level:

```
/ddx-agent
```

Shows available harnesses, their capabilities, and helps select the right
configuration before dispatching.

## `/ddx-status` — Health Check

Quick overview of project state:

```
/ddx-status
```

Runs `ddx doctor`, `ddx bead list`, and `ddx bead ready` and summarizes
the findings.

## Creating Custom Skills

See [Creating Plugins](../plugins) for how to add your own skills to DDx
or distribute them as a plugin.

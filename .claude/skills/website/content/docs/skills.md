---
title: DDx Skills
weight: 7
---

DDx ships workflow skills that agents discover as slash commands. These
guide agents through DDx operations instead of requiring them to memorize
flag combinations.

## Available Skills

| Skill | Description |
|-------|-------------|
| `/ddx-bead` | Create and manage beads with proper metadata |
| `/ddx-install` | Search, preview, and install packages |
| `/ddx-status` | Project health overview |
| `/ddx-review` | Fresh-eyes and adversarial code review workflow |

Skills are installed automatically by `ddx init` to `~/.agents/skills/ddx-*/`.

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

## `/ddx-review` — Review Workflow

Run a structured code review workflow:

```
/ddx-review
```

The skill:
1. Loads the review prompt and any attached context
2. Assembles a review prompt with relevant context
3. Dispatches the review workflow against the selected harnesses
4. Reports findings and evidence against the requested scope

### Example: Review a bead's implementation

```
/ddx-review bead ddx-abc123
```

The skill reads the bead's spec-id and acceptance criteria, then reviews
the implementation against them.

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

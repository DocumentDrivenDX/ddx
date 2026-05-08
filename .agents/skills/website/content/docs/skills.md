---
title: DDx Skill
weight: 7
---

DDx ships a single built-in skill, `ddx`, that any
[agentskills.io](https://agentskills.io)-compatible coding agent (Claude Code,
OpenAI Codex, Gemini CLI, etc.) discovers and uses to operate the DDx surface
correctly. The skill is installed automatically by `ddx init` to the project's
`.agents/skills/` and `.claude/skills/` directories as real files.

## Structure

The skill follows the standard SKILL.md + `reference/` layout. The top-level
`SKILL.md` is an overview and intent router; the real domain guidance lives in
`reference/*.md` files that the agent reads on demand.

```
ddx/
  SKILL.md          # overview, vocabulary, intent router
  reference/
    beads.md        # work-item authoring
    work.md         # draining the queue, executing beads
    review.md       # bead review, quorum/adversarial review
    agents.md       # agent dispatch, harness/power passthrough, personas
    status.md       # queue health, ddx doctor, sync status
```

## Reference areas

The skill routes the agent to one of these reference files based on the user's
intent:

### beads

Authoring and managing work items: `ddx bead create`, metadata, labels,
spec-id, acceptance criteria, dependency edges.

### work

Draining the queue end-to-end: `ddx work` to pick ready beads, `ddx try` to
attempt one bead in an isolated worktree, claim → build → verify → close.

### review

Two concepts under one roof. *Bead review* (`ddx bead review <id>`) grades a
completed bead against its acceptance criteria. *Comparison/adversarial
review* composes multiple `ddx run` invocations to cross-check work.

### agents

Dispatching agents via `ddx run`: power bounds (`--min-power` / `--max-power`),
harness/provider/model passthrough constraints, and persona bindings (`ddx
persona list/show/bind`).

### status

Queue and project health: `ddx doctor`, `ddx bead ready`, `ddx bead blocked`,
sync status, and "how am I doing" overviews.

## Creating Custom Skills

Plugins can ship additional skills alongside the built-in `ddx` skill. See
[Creating Plugins](../plugins) for how to add your own skills or distribute
them as a plugin.

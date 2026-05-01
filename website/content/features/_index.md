---
title: Features
description: "DDx platform capabilities — stable, beta, and planned."
---

DDx provides a focused set of platform primitives for AI-assisted development. Each capability is listed with its current maturity level.

## Core Platform

### Bead Tracker {{< maturity "stable" >}}

Beads are the atomic unit of work in DDx. Each bead carries a title, description, acceptance criteria, dependency graph, and full state history. The `ddx bead` subcommands cover the complete CRUD lifecycle.

```
ddx bead create "Add login endpoint" --ac "returns 200 on valid credentials"
ddx bead list --status ready
ddx bead close <id>
```

### Agent Dispatch {{< maturity "stable" >}}

Invoke AI agents through a single interface. DDx selects the configured harness, passes the prompt and context, and records token usage and cost per run.

```
ddx agent run --harness claude --prompt prompts/implement.md
ddx agent execute-bead <id>
```

### Execution Engine {{< maturity "stable" >}}

Each bead executes in an isolated git worktree. Evidence is captured after every run. Passing beads are merged back; timed-out or failing runs are preserved for inspection.

```
ddx agent execute-loop          # drain the queue
ddx agent execute-bead <id> --from HEAD
```

### Plugin System {{< maturity "stable" >}}

Workflow methodologies are packaged as plugins. One command installs a plugin under the project root — no global state, no machine-wide side effects.

```
ddx install helix               # install HELIX workflow
ddx install ddx                 # update DDx default resources
```

### Document Library {{< maturity "stable" >}}

Prompts, patterns, templates, and personas live in versioned library directories. The library ships with DDx and is extended by plugins.

```
ddx prompts list
ddx templates apply scaffold
ddx persona list
```

### Persona System {{< maturity "stable" >}}

Personas are reusable AI personality templates — code-reviewer, test-engineer, architect. Projects bind personas to roles in `.ddx.yml`. Every agent run picks up the bound persona automatically.

```
ddx persona show code-reviewer
ddx persona bind code-reviewer --role reviewer
```

### Git Sync {{< maturity "stable" >}}

DDx resources are versioned via git subtree. `ddx update` pulls the latest library from the master repository; `ddx contribute` pushes improvements back.

```
ddx update
ddx contribute
```

## Extended Capabilities

### MCP Server {{< maturity "beta" >}}

DDx can expose beads, documents, and execution history over the Model Context Protocol (MCP) and HTTP. Remote supervisors — including Claude Desktop and compatible clients — can query work state and steer execution.

```
ddx server start
```

### Web UI {{< maturity "beta" >}}

A browser interface for the bead tracker and document library. Built with SvelteKit and embedded in the `ddx-server` binary. Useful for reviewing bead state, execution history, and document relationships without leaving the browser.

### Cost-Tiered Routing {{< maturity "beta" >}}

Configure multiple model endpoints with tier labels. DDx routes beads to the cheapest model that meets the complexity threshold, escalating on failure. Optimize closed-work-per-dollar across Claude, Codex, Gemini, and local models.

## Planned

### Remote Execution {{< maturity "planned" >}}

Run beads on remote compute — a cloud VM, a CI runner, or a dedicated agent host — without changing the local workflow. The bead queue and merge result flow stay identical.

### Team Sync {{< maturity "planned" >}}

Shared bead queues across a team. Agents on multiple machines claim, execute, and close beads from the same queue without coordination overhead.

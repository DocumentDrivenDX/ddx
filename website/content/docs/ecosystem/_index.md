---
title: Ecosystem
weight: 5
---

DDx ships in two layers. The platform provides primitives every document-driven
project needs; workflow methodologies layer opinions on top. Knowing which layer
owns which job tells you where to look — and what you should not expect from
DDx itself.

## Two Layers

```
┌──────────────────────────────────────────────────────┐
│  Workflow Methodology                                │
│  HELIX, your team's process, or a custom workflow    │
│  Opinions: phases, gates, supervisory loops, review  │
├──────────────────────────────────────────────────────┤
│  DDx Platform  ← primitives                          │
│  Document library, bead tracker, plugin registry,    │
│  agent dispatch, MCP server, git sync                │
└──────────────────────────────────────────────────────┘
```

DDx is unopinionated about *how* you build software. It gives you the artifacts
agents read, the work items they claim, and the dispatch surface that runs
them. A workflow methodology decides what comes first, when a phase is
complete, and which review gates apply.

## What Goes Where

| DDx platform owns | A workflow methodology owns |
|-------------------|-----------------------------|
| Document library and templates | Which documents are required, and when |
| Bead tracker (work items, dependencies, claims) | Phase gates and definition-of-done |
| Plugin registry (`ddx install`) | Methodology-specific skills and prompts |
| Agent dispatch and harness routing | Supervisory loops and planning |
| MCP server for documents and beads | Story shape, review checklists, ceremonies |
| Git sync and execution evidence | Release process and deployment cadence |

If a question is "*how* should this team build?", it belongs in a workflow
methodology. If it is "*what* tools do I need to coordinate documents, work,
and agents?", it belongs in DDx.

## DDx Platform Artifacts

DDx produces two artifacts from a single repository:

| Artifact | What It Does |
|----------|-------------|
| **`ddx` CLI** | Document management, bead tracker, agent dispatch, plugin registry |
| **`ddx-server`** | Serve documents, beads, and agent logs over HTTP and MCP |

Both artifacts are workflow-agnostic. You can use them with HELIX, with another
methodology, or with no methodology at all.

## Plugin Registry

Plugins are how the ecosystem extends DDx. A plugin is a git repository that
ships any mix of skills, templates, prompts, patterns, MCP servers, or tool
configs. Installing one is a single command:

```bash
ddx install <plugin>
```

`ddx install` writes only inside the project — under `.ddx/plugins/<name>/`,
`.agents/skills/`, and `.claude/skills/`. The tree is committed to git, so
teammates and CI get the same plugin set on clone. Nothing is installed under
`~/`.

A plugin can be as small as a single skill or as large as a full methodology.
HELIX is the reference example, but the same mechanism distributes:

- **Workflow methodologies** — phase definitions, gates, supervisory skills
- **Skill packs** — agent slash-commands grouped by domain (review, testing, infra)
- **Template libraries** — opinionated starting points for new projects
- **MCP server bundles** — pre-wired integrations for agents to call

The registry is decentralized: any GitHub repo with a tagged release can act as
a plugin source. See the [plugins documentation](../plugins/) for layout and
authoring details.

## Agent Landscape

DDx is the dispatch and coordination layer for AI agents — it does not ship a
model. Beads, documents, and skills are consumed by agents you bring.

| Agent class | Examples | How DDx talks to it |
|-------------|----------|--------------------|
| Hosted frontier | Claude (Anthropic), GPT (OpenAI), Gemini (Google) | Provider SDKs via `ddx agent run` |
| Coding harnesses | Claude Code, Codex, Cursor agents | Harness adapters configured per project |
| Local / self-hosted | Ollama, llama.cpp, vLLM-served models | OpenAI-compatible endpoints |
| MCP clients | Any MCP-aware host | `ddx-server` exposes documents and beads |

Routing is configured per project in `.ddx/config.yaml`. The same bead can be
executed by a cheap local model, escalated to a frontier model on retry, or
reviewed by a different harness entirely — the bead tracker and document
library are stable across all of them.

## Workflow Methodologies

Workflow methodologies are layered on top of DDx as plugins. They turn the
platform's primitives into a development process.

### HELIX

HELIX is the reference workflow methodology: a six-phase structured process
(Frame, Design, Test, Build, Deploy, Iterate) with phase gates, supervisory
dispatch, and bounded actions. It uses DDx for document management, bead
tracking, and agent dispatch — and adds the opinions about *how* those
primitives are sequenced.

```bash
ddx install helix
```

{{< asciinema src="03-plugin-install" >}}

Watch the full DDx + HELIX journey — from init to a working application:

{{< asciinema src="06-full-journey" cols="100" rows="30" >}}

[HELIX on GitHub →](https://github.com/easel/helix)

### Your Methodology

Because DDx is workflow-agnostic, you can build your own methodology on its
primitives, package it as a plugin, and ship it through the same registry.
You can also use DDx without any workflow methodology at all — beads,
documents, and agents work fine on their own.

## For Workflow Authors

If you are building a workflow methodology on DDx, you get for free:

- **Document library management** — structured docs your users already have
- **Bead tracker** — shared work-item storage with dependencies and claims
- **Agent dispatch** — invoke any supported AI agent through one interface
- **Plugin registry** — distribute your methodology with `ddx install`
- **MCP access** — agents discover and read documents programmatically
- **Execution evidence** — every agent run is recorded for audit and replay

Focus your methodology on what makes it unique. Let DDx handle the platform.

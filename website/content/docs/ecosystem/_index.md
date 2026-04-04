---
title: Ecosystem
weight: 5
---

DDx is one layer in a stack. Understanding where it fits helps you know what DDx does — and what to get elsewhere.

## The Stack

```
┌─────���──────────────────��────────────────────────────┐
│  Workflow Tools                                      │
│  HELIX, your team's methodology, or custom workflows ��
│  Opinionated: phases, gates, enforcement, practices  │
├─────────────────────────────────────────────────────┤
│  DDx  ← you are here                                │
│  Shared infrastructure: document libraries, personas,│
│  bead tracker, agent dispatch, MCP server            │
├────────────��───────────────��────────────────────────┤
│  AI Agents                                           │
│  Claude, GPT, Gemini, local models                   │
│  Consume documents, produce implementations          │
└────────────────────────────���────────────────────────┘
```

## What Goes Where

| Belongs in DDx | Belongs in a Workflow Tool |
|---------------|--------------------------|
| Document library structure | Development phases and gates |
| Persona definitions and bindings | Phase enforcement rules |
| Bead tracker (work items) | Workflow-specific validation |
| Agent dispatch and session logging | Supervisory loops and planning |
| Plugin registry (`ddx install`) | Methodology-specific practices |
| MCP server for documents | Story/issue management beyond beads |

## DDx Artifacts

DDx produces two artifacts from a single repository:

| Artifact | What It Does |
|----------|-------------|
| **`ddx` CLI** | Document management, bead tracker, agent dispatch, plugin registry |
| **`ddx-server`** | Serve documents, beads, and agent logs over HTTP and MCP |

## Workflow Tools

Workflow tools build on DDx's infrastructure to provide opinionated development practices.

### HELIX

A six-phase structured development methodology (Frame, Design, Test, Build, Deploy, Iterate) that uses DDx for document management, bead tracking, and agent dispatch.

```bash
# Install HELIX with one command
ddx install helix
```

{{< asciinema src="03-plugin-install" >}}

Watch the full DDx + HELIX journey — from init to a working application:

{{< asciinema src="06-full-journey" cols="100" rows="30" >}}

[HELIX on GitHub →](https://github.com/easel/helix)

### Your Methodology

DDx is workflow-agnostic. You can build your own methodology on DDx's primitives, or use DDx without any workflow tool at all.

## For Workflow Tool Authors

If you're building a workflow tool on DDx, you get for free:

- **Document library management** — your users already have structured docs
- **Bead tracker** — shared work-item storage with dependencies and coordination
- **Persona system** — bind agents to roles with predefined behavior
- **Agent dispatch** — invoke any supported AI agent through one interface
- **Plugin registry** — distribute your tool as a DDx plugin
- **MCP access** — agents can discover and read documents programmatically

Focus your tool on what makes your methodology unique. Let DDx handle the infrastructure.

---
title: Ecosystem
weight: 5
---

DDx is one layer in a stack. Understanding where it fits helps you know what DDx does — and what to get elsewhere.

## The Stack

```
┌─────────────────────────────────────────────────────┐
│  Workflow Tools                                      │
│  HELIX, your team's methodology, or custom workflows │
│  Opinionated: phases, gates, enforcement, practices  │
├─────────────────────────────────────────────────────┤
│  DDx  ← you are here                                │
│  Shared infrastructure: document libraries, personas,│
│  templates, git sync, meta-prompts, MCP server       │
├─────────────────────────────────────────────────────┤
│  AI Agents                                           │
│  Claude, GPT, Gemini, local models                   │
│  Consume documents, produce implementations          │
└─────────────────────────────────────────────────────┘
```

## What Goes Where

| Belongs in DDx | Belongs in a Workflow Tool |
|---------------|--------------------------|
| Document library structure | Development phases and gates |
| Persona definitions and bindings | Phase enforcement rules |
| Template application | Workflow-specific CLI commands |
| Git subtree sync | Progress tracking |
| Meta-prompt injection | Methodology-specific practices |
| MCP server for documents | Story/issue management |

## DDx Artifacts

DDx produces three artifacts from a single repository:

| Artifact | What It Does |
|----------|-------------|
| **`ddx` CLI** | Local document management — init, list, sync, personas, templates |
| **`ddx-server`** | Serve documents over HTTP and MCP for agent consumption |
| **This website** | Documentation and promotion |

## Workflow Tools

Workflow tools build on DDx's infrastructure to provide opinionated development practices:

### HELIX

A six-phase structured development methodology (Frame, Design, Test, Build, Deploy, Iterate) that uses DDx for document management, persona composition, and template application.

[HELIX on GitHub →](https://github.com/easel/helix)

### Your Methodology

DDx is workflow-agnostic. You can build your own methodology on DDx's primitives, or use DDx without any workflow tool at all.

## For Workflow Tool Authors

If you're building a workflow tool on DDx, you get for free:

- **Document library management** — your users already have structured docs
- **Persona system** — bind agents to roles with predefined behavior
- **Template engine** — generate project structures with variable substitution
- **Git sync** — share patterns across projects
- **MCP access** — agents can discover and read documents programmatically

Focus your tool on what makes your methodology unique. Let DDx handle the document plumbing.

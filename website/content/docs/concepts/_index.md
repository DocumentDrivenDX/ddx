---
title: Concepts
weight: 2
---

The ideas behind DDx and document-driven development.

## The Three-Layer Stack

DDx is one layer in a three-project stack. The platform stays general; opinions live in workflow tools; quality checks live in their own runner.

![DDx / HELIX / Dun three-layer stack](/diagrams/three-layer-stack.svg)

- **DDx** — platform services: document library, bead tracker, agent dispatch, personas, templates, git sync, MCP.
- **HELIX** — a workflow methodology built on DDx. Phases, gates, supervisory dispatch, methodology-specific validation.
- **Dun** — a quality runner. Discovers and runs checks; emits agent-friendly output.

DDx provides primitives. HELIX (and others) provide opinions. Dun verifies the result.

## Documents Are the Product

The fundamental shift: **you maintain documents, agents produce code.**

In traditional development, code is the primary artifact. In document-driven development, the primary artifacts are the documents that tell agents what to build and how to build it:

- **Prompts** — instructions that direct agent behavior for specific tasks
- **Personas** — behavioral definitions that shape how agents approach work
- **Patterns** — proven solutions to recurring problems, written for agent consumption
- **Templates** — project and file blueprints with variable substitution
- **Specs** — requirements and designs that define what to build

The quality of agent output follows directly from the quality of these documents. Better documents produce better code, every time.

## The Document Library

DDx organizes agent-facing documents into a structured library:

```
.ddx/library/
├── prompts/        # Agent instructions
├── personas/       # Behavioral definitions
├── patterns/       # Reusable solutions
├── templates/      # Project blueprints
├── configs/        # Tool configurations
├── mcp-servers/    # MCP server registry
└── environments/   # Environment-specific docs
```

These are plain Markdown and YAML files. Any agent can read them. Any developer can edit them. Git tracks every change.

## Composition Over Monoliths

Instead of maintaining one giant instruction set, DDx encourages **small, focused documents combined on demand**:

- A persona defines behavior ("be a strict code reviewer")
- A pattern defines an approach ("handle errors this way")
- A spec defines requirements ("build this feature")

Composed together, they give an agent everything it needs for a specific task. Each piece is independently maintainable and reusable.

## Personas

A persona is a document that shapes how an agent behaves. DDx ships with personas like:

| Persona | Behavior |
|---------|----------|
| `code-reviewer` | Pedantic about quality, catches edge cases, demands tests |
| `implementer` | Ships working code fast, avoids over-engineering |
| `test-engineer` | Writes tests first, validates thoroughly |
| `architect` | Chooses the simplest design that works |

You **bind** personas to **roles** in your project configuration:

```yaml
# .ddx.yml
persona_bindings:
  code-reviewer: code-reviewer
  architect: architect
```

When an agent is assigned to a role, it picks up the bound persona and adjusts its approach.

![Persona / role binding](/diagrams/persona-binding.svg)

## Beads: the Unit of Work

Every task is a **bead**. Beads carry a description, acceptance criteria, dependencies, and a status. Agents claim ready beads, do the work, and close them. The queue drives what happens next.

![Bead lifecycle](/diagrams/bead-lifecycle.svg)

The execute-loop drains the queue end-to-end: claim a ready bead, render its prompt, run the agent in an isolated worktree, review the result, merge or push (with auto-recovery on race), and close.

![Execute-loop flow](/diagrams/execute-loop.svg)

Cheap models do the work; stronger models review; deterministic fixes sit at the top of the ladder. The metric is closed-bead throughput per dollar.

## Project-Local Everything

`ddx init` and `ddx install` only touch the project root. There is no shared mutable state outside the repo. The single global artifact is `ddx-server` — a long-running daemon — and it is optional.

![Project-local install model](/diagrams/project-local-install.svg)

## Git-Native Sync

DDx uses git subtree to synchronize document libraries:

- **Pull** community improvements into your project
- **Push** your improvements back to the upstream library
- **Override** specific documents locally without losing sync

No external services. No lock-in. Standard git workflows.

## Infrastructure, Not Methodology

DDx is deliberately **general and abstract**. It provides primitives:

- Document library management
- Persona system with role bindings
- Template engine with variables
- Git-based sync
- Meta-prompt injection
- MCP server management

Specific development methodologies — how to structure your development process, what phases to follow, what gates to enforce — are built as **separate tools on top of DDx**. DDx provides the foundation; workflow tools provide the opinions.

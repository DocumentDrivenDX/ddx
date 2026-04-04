---
title: Concepts
weight: 2
---

The ideas behind DDx and document-driven development.

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
| `strict-code-reviewer` | Pedantic about quality, catches edge cases, demands tests |
| `pragmatic-implementer` | Ships working code fast, avoids over-engineering |
| `test-engineer-tdd` | Writes tests first, validates thoroughly |
| `simplicity-architect` | Chooses the simplest design that works |

You **bind** personas to **roles** in your project configuration:

```yaml
# .ddx.yml
persona_bindings:
  code-reviewer: strict-code-reviewer
  architect: simplicity-architect
```

When an agent is assigned to a role, it picks up the bound persona and adjusts its approach.

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

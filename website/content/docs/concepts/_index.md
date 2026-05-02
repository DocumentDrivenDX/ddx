---
title: Concepts
weight: 2
---

The ideas behind DDx and document-driven development.

## What DDx Is

DDx (Document-Driven Development eXperience) is the **shared infrastructure
platform for document-driven development**. It provides the primitives that
developers and workflow tools use to maintain, compose, and deliver the
documents AI agents consume to build software.

The core thesis: **document quality, not agent capability, is the bottleneck.**
Well-maintained abstractions — requirements, architecture, design, tests —
produce better software than working at the code level alone. DDx encodes that
insight into infrastructure.

```mermaid
flowchart LR
    Dev["Developer<br/>(maintains documents)"]
    subgraph Docs["Documents (the product)"]
        direction TB
        Specs["Specs"]
        Personas["Personas"]
        Patterns["Patterns"]
        Templates["Templates"]
        Prompts["Prompts"]
    end
    Agent["AI Agent<br/>(harness)"]
    Code["Code,<br/>tests, artifacts"]

    Dev -->|edits| Docs
    Docs -->|composed into prompt| Agent
    Agent -->|produces| Code
    Code -.->|feedback informs| Docs

    classDef product fill:#fef3c7,stroke:#b45309,color:#1f2937;
    class Docs product;
```

## The Three-Layer Stack

DDx is one layer in a three-project stack with explicit boundaries. Each layer
is independently useful and replaceable.

| Layer | Project | Owns |
|-------|---------|------|
| **Platform** | [DDx](https://github.com/DocumentDrivenDX/ddx) | Document library, bead tracker, agent dispatch, personas, templates, git sync |
| **Workflow** | [HELIX](https://github.com/DocumentDrivenDX/helix) | Phases, gates, supervisory dispatch, methodology |
| **Quality** | [Dun](https://github.com/DocumentDrivenDX/dun) | Check discovery, execution, agent-friendly output |

DDx provides primitives. HELIX and others provide opinions. Dun verifies the
result.

```mermaid
flowchart TB
    subgraph Workflow["Workflow layer — opinions"]
        HELIX["HELIX<br/>phases · gates · supervisory dispatch"]
        Other["...alternative<br/>workflows"]
    end
    subgraph Platform["Platform layer — primitives"]
        DDx["DDx<br/>document library · bead tracker ·<br/>agent dispatch · personas · git sync"]
    end
    subgraph Quality["Quality layer — verification"]
        Dun["Dun<br/>check discovery · execution ·<br/>agent-friendly output"]
    end

    HELIX -->|consumes primitives| DDx
    Other -->|consumes primitives| DDx
    DDx -->|invokes checks| Dun
    Dun -->|results| DDx

    classDef platform fill:#dbeafe,stroke:#1d4ed8,color:#1e293b;
    classDef workflow fill:#dcfce7,stroke:#15803d,color:#1e293b;
    classDef quality fill:#fae8ff,stroke:#a21caf,color:#1e293b;
    class DDx platform;
    class HELIX,Other workflow;
    class Dun quality;
```

{{< callout type="info" >}}
**Why three layers?** Mixing methodology into platform code locks teams into
one way of working. Keeping the platform opinion-free lets HELIX evolve, lets
alternative workflows exist, and lets DDx serve both fully-autonomous and
human-driven teams from the same primitives.
{{< /callout >}}

## Documents Are the Product

The fundamental shift: **you maintain documents, agents produce code.**

In traditional development, code is the primary artifact. In document-driven
development, the primary artifacts are the documents that tell agents what to
build and how to build it:

- **Prompts** — instructions that direct agent behavior for specific tasks
- **Personas** — behavioral definitions that shape how agents approach work
- **Patterns** — proven solutions to recurring problems, written for agent
  consumption
- **Templates** — project and file blueprints with variable substitution
- **Specs** — requirements and designs that define what to build

The quality of agent output follows directly from the quality of these
documents. Better documents produce better code, every time.

## What DDx Provides

- **Document library** — structured, versioned, agent-discoverable artifacts
  in the repository.
- **Bead tracker** — work items with a dependency DAG, ready/blocked queues,
  and JSONL interchange. Beads are the unit of work agents execute.
- **Agent service** — unified harness dispatch (Claude, Codex, Gemini, local
  models) with quorum review, session logging, and a single prompt envelope.
- **Execute-loop** — drain the bead queue with isolated worktrees, automatic
  review, and recovery. Cheap models do, strong models review.
- **Project-local install** — `ddx init` and `ddx install <plugin>` only touch
  `<projectRoot>`. The only global artifact is `ddx-server`.
- **Single `ddx` skill** — one consolidated skill, not a fleet. One surface
  for agents to learn.

## What DDx Is Not

- **Not a methodology.** No phases, no gates, no prescribed artifact types —
  workflow tools own those.
- **Not a storage system.** Files in Git. No proprietary backend.
- **Not an editor or IDE.** Editing happens wherever you already work.
- **Not opinionated about agents.** Any harness with a prompt-in/output-out
  contract plugs in.

## Read Next

- [Principles](principles/) — the load-bearing decisions behind DDx.
- [Architecture](architecture/) — how beads, personas, and the
  project-local install model fit together.
- [Run Architecture](run-architecture/) — the layered `ddx run` /
  `ddx try` / `ddx work` model that drains the bead queue.
- [Glossary](glossary/) — quick definitions for the terms used across the
  docs.

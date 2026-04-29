---
ddx:
  id: product-vision
---
# Product Vision: DDx

**Version:** 2.0.0
**Date:** 2026-04-06
**Status:** Active

## Core Thesis

Well-maintained abstractions — requirements, architecture, design, tests —
produce better software than working at the code level alone. The agentic era
amplifies this: document quality, not agent capability, is the bottleneck.

DDx encodes that insight into infrastructure. Documents become first-class,
agent-consumable artifacts with identity, relationships, and lifecycle. Beads
turn intent into self-contained work items. Agents drain the queue.

## The Three-Layer Stack

DDx is one layer in a three-project stack with explicit boundaries:

| Layer | Project | Owns |
|-------|---------|------|
| Platform | **DDx** (this repo) | Document library, bead tracker, agent dispatch, personas, templates, git sync |
| Workflow | **HELIX** | Phases, gates, supervisory dispatch, bounded actions, methodology |
| Quality | **Dun** | Check discovery, execution, agent-friendly output |

DDx provides primitives. HELIX and others provide opinions. Dun verifies the
result. Each layer is independently useful and replaceable.

## Mission

Make documents the unit of software development. Provide the shared
infrastructure that developers and workflow tools use to maintain, compose, and
deliver the documents agents consume to build software.

## What DDx Provides

- **Document library** — structured, versioned, agent-discoverable artifacts in
  the repository. Personas, patterns, templates, prompts, MCP server registry.
- **Bead tracker** — work items with a dependency DAG, ready/blocked queues,
  and JSONL interchange. Beads are the unit of work agents execute.
- **Agent service** — unified harness dispatch (claude, codex, gemini, local
  models) with quorum review, session logging, and a single prompt envelope.
- **Execute-loop** — drain the bead queue with isolated worktrees, automatic
  review, and recovery. Cheap models do, strong models review.
- **Project-local install** — `ddx init` and `ddx install <plugin>` only touch
  `<projectRoot>`. The only global artifact is `ddx-server`. No home-directory
  state, no machine-wide opinions.
- **Single `ddx` skill** — DDx ships one consolidated skill, not a fleet. One
  surface for agents to learn.

## What DDx Is Not

- **Not a methodology.** No phases, no gates, no prescribed artifact types.
  Workflow tools (HELIX) own those.
- **Not a storage system.** Files in Git. No proprietary backend.
- **Not an editor or IDE.** Editing happens wherever the user works.
- **Not opinionated about agents.** Any harness with a prompt-in/output-out
  contract plugs in.

## Key Differentiators

### vs. Ad-Hoc Agentic Coding

Code-as-system-of-record produces point changes without coupling awareness, no
structured intent, and constant context re-explanation. DDx makes documents the
system of record for intent and architecture; code remains the system of record
for implementation.

### vs. Code-Only Agent Tools

Without an abstraction hierarchy, functionality accumulates in non-orthogonal
interfaces and agents cannot reason about cross-cutting impact. DDx gives every
change a defined place and a known impact boundary.

### vs. Traditional Documentation-Driven Development

Prior DDD failed because documents went stale. DDx counters that with
relationships, content hashing, beads as reconciliation tasks, and adversarial
review.

## Design Philosophy

### Platform vs Workflow Boundary

The hardest discipline is keeping platform code free of methodology opinions.
The boundary is concrete:

| DDx (Platform) | Workflow Tool (e.g., HELIX) |
|----------------|------------------------------|
| Document storage, versioning, sync | Artifact types and templates |
| Bead CRUD, dependency DAG, ready queue | Phase definitions, gates, ratchets |
| Agent dispatch, quorum, session logging | Methodology prompts, supervisory loops |
| Persona binding mechanism | Which personas fulfill which roles |
| Execute-loop runtime | When to run it, what to do with results |

If a feature request would push opinions into the CLI, it belongs in a plugin.

### Multi-Directional Iteration

The artifact hierarchy is a set of lenses, not a pipeline. Vision changes
propagate down; implementation discoveries feed up; spec refinements ripple
both ways. DDx's primitives (beads, document relationships, content hashing)
support all three directions equally.

### Cost-Tiered Execution

Closed-bead throughput per dollar is the optimization target. Cheap models do
the work; stronger models review; deterministic checks (Dun) sit at the top of
the ladder catching what review missed. The agent service routes by capability,
not by name — endpoints with live model discovery, not hardcoded providers.

### Human-Agent Control Slider

DDx supports a continuum from full autonomy (drain the queue overnight) to
human-driven (agent assists, human commits). The same primitives serve both;
the workflow layer chooses where to sit.

## Strategic Fit

**Why us:** DDx grew out of daily agent-driven development. The problems we
solve are the problems we hit shipping software with agents.

**Why now:** Document quality is the bottleneck. Every team is independently
rebuilding the same document infrastructure. No shared layer exists.

**North Star:** Every developer working with AI agents uses DDx, or tools built
on DDx, to manage the documents that drive those agents.

## Principles

See `docs/helix/01-frame/principles.md`. The load-bearing ones:

1. **Platform, not methodology** — DDx provides primitives; workflow tools provide opinions.
2. **Project-local by default** — install touches the project, not the machine.
3. **Documents are the product** — code is output; documents are what you maintain.
4. **Bead-driven work** — beads are the unit of work; the queue is the interface.
5. **Cost-tiered throughput** — optimize closed beads per dollar, not raw capability.
6. **Git-native, file-first** — plain files, standard git, no lock-in.
7. **Agent-agnostic** — any capable harness plugs in via the prompt envelope.

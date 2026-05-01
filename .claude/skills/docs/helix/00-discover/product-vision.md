---
ddx:
  id: product-vision
---
# Product Vision: DDx

**Version:** 2.0.0
**Date:** 2026-04-06
**Status:** Active

## Core Thesis

Fifty years of software engineering has produced a reliable insight:
well-maintained abstractions at multiple levels — requirements, architecture,
design, tests — produce better software than working at the code level alone.
The agentic era does not invalidate this; it amplifies it.

Creating documentation and using it as an abstraction — then refining the
details around that abstraction — is the best way to enable agents to write
correct software. DDx encodes that insight into infrastructure, making
documents first-class, agent-consumable artifacts with identity, relationships,
and lifecycle tracking.

## Mission Statement

DDx makes documents the unit of software development — providing the shared infrastructure that developers and workflow tools use to maintain, compose, and deliver the documents AI agents consume to build software.

## What DDx Is

DDx (Document-Driven Development Experience) is a **toolkit and platform for
document-driven agentic software development**. It is unopinionated about
methodology — all methodology opinions live in plugins (e.g., HELIX).

DDx provides:

- **Artifact management** — define, version, and manage structured documents
  (visions, specs, designs, ADRs, etc.) within a repository
- **Plugin infrastructure** — `ddx init`, `ddx install <plugin>` to add
  methodology-specific capabilities. DDx stays lean; plugins bring opinions.
- **Bead-based issue tracker** — ephemeral implementation tasks that synthesize
  context from project documents into self-contained work items agents can
  execute without loading additional context
- **Agent execution infrastructure** — tools for running agents against beads,
  including adversarial review workflows
- **Artifact templates with props** — documents templated with typed properties
  and relationships, forming a graph of interconnected artifacts

## What DDx Is Not

- **Not a methodology.** DDx does not prescribe phases, artifact types, or
  workflows. Those come from plugins.
- **Not a storage system.** Artifacts are versioned in Git. Future backends are
  possible but not DDx's concern.
- **Not an IDE or editor.** DDx manages documents and tasks; editing happens in
  whatever tools the user prefers.

## Key Differentiators

### vs. Ad-Hoc Agentic Coding (Vibe Coding)

The mainstream approach — agents making changes with code as the system of
record — leads to point changes without understanding coupling, no structured
way to communicate intent, and constant context re-explanation. DDx makes
documentation the system of record for intent and architecture, while code
remains the system of record for implementation.

### vs. Code-Only Agent Tools

Systems treating code as the sole system of record accumulate functionality
without abstraction hierarchy, producing non-orthogonal interfaces and no way
for agents to understand cross-cutting impact. DDx's progressive abstraction
layers give every change a defined place and known impact boundary.

### vs. Traditional Documentation-Driven Development

Prior art in DDD fails because documentation gets stale. DDx mitigates this by
making documents first-class artifacts with relationships, using beads to
create reconciliation tasks, supporting adversarial review for consistency
checking, and detecting staleness automatically.

## Design Philosophy

### Multi-Directional Iteration

DDx supports iteration in all directions through the artifact hierarchy:

- **Top-down** — vision changes propagate through PRD, specs, tests, code
- **Bottom-up** — implementation discoveries feed back up to specs or vision
- **Middle-out** — spec refinements trigger updates both above and below

The artifact hierarchy is a set of lenses at different zoom levels, not a
linear pipeline.

### Human-Agent Control Slider

DDx supports a continuum of human involvement:

- **Full agent autonomy** — one-shot prompt, agent runs the entire pipeline
- **Guided autonomy** — human reviews at abstraction boundaries
- **Collaborative** — human and agent co-author at every level
- **Human-driven** — agent assists with research, drafting, review; human decides

### Self-Documenting Workflows

After `ddx init` and plugin install, the resulting project should explain
itself well enough that a new team member — human or agent — can orient
quickly.

### Platform Services, Not Opinions

| DDx (Platform) | Plugin (e.g., HELIX) |
|----------------|---------------------|
| Artifact storage and versioning | Artifact templates and types |
| Bead creation, assignment, lifecycle | Phase definitions |
| Agent execution and orchestration | Cross-cutting concern definitions |
| Adversarial review infrastructure | Methodology-specific prompts |
| Metric collection hooks | Metric definitions |
| Feedback loop infrastructure | Feedback analysis and action |

## 3-5 Year Vision

| Timeframe | Market Position | Key Milestones |
|-----------|----------------|----------------|
| Year 1 | Established open-source toolkit used by early adopters building agent-driven workflows | CLI stable, server shipping, 3+ workflow tools building on DDx, active community library |
| Year 3 | Standard infrastructure layer for document-driven development, analogous to what npm is for packages | Ecosystem of workflow tools, enterprise adoption, rich MCP integration, document analytics |
| Year 5 | Foundational platform for the agent-driven development era — documents-as-code is the default practice | Industry-standard document formats, broad IDE integration, self-improving document libraries |

**North Star:** Every developer working with AI agents uses DDx (or tools built on DDx) to manage the documents that drive those agents.

## Target Market

| Segment | Primary: Agent-First Developers | Secondary: Team Leads & Architects |
|---------|-------------------------------|-------------------------------------|
| Size | Millions of developers using AI coding agents daily | Hundreds of thousands managing teams using agents |
| Pain | Documents scattered, no composition, no reuse — agents get bad context and produce bad output | No standardization across teams, no way to share what works, knowledge trapped in individual repos |
| Current Solution | Ad-hoc markdown files, copy-paste between projects, manual context assembly | Internal wikis, tribal knowledge, per-project CLAUDE.md files maintained by hand |

## Key Value Propositions

| Capability | Benefit |
|-----------|---------|
| Structured document library | Agent-facing documents stay organized, discoverable, and current instead of rotting in random files |
| Git-native sync | Proven patterns flow between projects and teams without reinventing sharing infrastructure |
| Persona composition | Consistent agent behavior across projects — bind "strict-code-reviewer" once, get it everywhere |
| Meta-prompt injection | Right baseline context injected into agents automatically, no manual assembly |
| MCP server for document access | Agents can programmatically browse and consume document libraries |
| Template engine | New projects start with proven document structures, not blank files |
| Bead tracker | Portable work items with dependency DAG, ready queue, and import/export — shared across workflow tools |
| Agent service | Unified harness dispatch (codex, claude, gemini, etc.) with quorum, session logging, and prompt envelope format |
| Document dependency graph | Track which docs depend on which, detect staleness via content hashing, cascade invalidation when upstream docs change |
| Workflow-agnostic primitives | Any methodology (HELIX, custom, etc.) can build on DDx without reimplementing infrastructure |

## Success Definition

| Metric | Target | Timeline |
|--------|--------|----------|
| Projects using DDx document libraries | 500+ | Year 1 |
| Workflow tools built on DDx | 3+ | Year 1 |
| Community-contributed personas/patterns | 100+ | Year 1 |
| Document reuse rate across projects | >40% | Year 1 |
| DDx server MCP endpoints adopted by agent tools | 2+ integrations | Year 1 |

## Strategic Fit

**Why us:** DDx grew out of real agent-driven development practice. The document management problems we're solving are ones we hit daily building software with AI agents.

**Why now:** AI agents crossed the capability threshold where document quality — not agent capability — is the bottleneck. Every team is independently discovering they need document infrastructure. No standard exists yet.

**Resources:** Open source, community-driven. Go CLI (proven tech), static website (low maintenance), server component (Go, leveraging CLI internals). Single repo, three focused outputs.

## Principles

1. **Documents are the product** — code is output, documents are what you maintain
2. **Infrastructure, not methodology** — DDx provides primitives, workflow tools provide opinions
3. **Git-native, file-first** — plain files, standard git, no lock-in
4. **Composition over monoliths** — small focused documents combined on demand
5. **Agent-agnostic** — documents work with any capable agent

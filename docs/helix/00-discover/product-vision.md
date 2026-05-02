---
ddx:
  id: product-vision
---
# Product Vision: DDx

**Version:** 2.1.0
**Date:** 2026-04-29
**Status:** Active

## Core Thesis

> Software is leverage. Fifty years of practice has shown that there are physics of software that must be respected to produce and maintain systems at scale. The agentic era allows every team to produce systems at scale and makes this even more important.
>
> Generative AI brings its own physics. DDx exists at the seam where these two meet — the fulcrum that lets software's lever do work. Without it, shipping software with agents quietly degrades into prompt-and-pray. The six principles below are the load-bearing claims on both sides.

## The Productivity Shift

Agentic AI compresses what used to be person-weeks of engineering into hours. The bottleneck moves: writing code is no longer the binding constraint — keeping intent, artifacts, and evidence coherent across many parallel agent invocations is. Teams that hit this new ceiling keep hitting the same six pain points, and every team independently rebuilds the same infrastructure to climb past them. DDx ships that infrastructure once.

Each pain point below maps to a specific DDx capability — the list is bounded by what the platform actually addresses, not an open-ended catalog.

1. **Context assembly is manual.** Composing the right persona + pattern + spec + prior evidence into agent context is ad hoc and slow. *(DDx: artifact library + composition.)*
2. **Documents drift silently.** When an upstream document changes, dependent artifacts go stale with no detection or reconciliation. *(DDx: artifact graph + staleness + reconciliation beads.)*
3. **Work tracking is reinvented.** Every workflow tool reimplements issue storage, dependency DAGs, and ready/blocked queues. *(DDx: bead tracker + JSONL interchange.)*
4. **Agent runs leave no shared trace.** Each tool grows its own dispatch, logging, and evidence shape; nothing carries between invocations. *(DDx: three-layer run architecture on a unified on-disk substrate.)*
5. **Cost of agentic work is invisible.** Token spend is a first-order constraint with no shared signal on the cheapest model that reliably closes beads. *(DDx: cost-tier routing + token awareness.)*
6. **Skills and plugins don't compose.** Each project reinvents its agent instructions, skills, and supporting mechanics from scratch. *(DDx: single `ddx` skill + project-local plugin install.)*

### Physics of Software

1. **Abstraction is the lever.** Multi-level artifact stacks (vision → spec → test → code) with maintained relationships are how intent propagates without being lost. True for human teams; load-bearing for agents because they don't carry tacit knowledge between invocations.
2. **Software is iteration over tracked work.** Repeated trials over an explicit work substrate — beads, queues, dependency DAGs — is how software gets built. Pre-existed agents; agents make the substrate non-optional.
3. **Methodology is plural.** Different teams, projects, and problem domains demand different workflows — waterfall, agile, kanban, HELIX, ad-hoc. No tool that bakes one in survives the rest. DDx provides primitives (artifacts, runs, beads) that any methodology composes.

### Physics of Generative AI

4. **LLMs are stochastic, unreliable, and costly.** Cost-tier ladders, ensemble verification, and 'cheapest model that works' are the operating shape of agentic work, not optimizations. Quality degrades as the context window fills.
5. **Evidence provides memory.** Agents carry no state between invocations and outputs aren't bit-reproducible. The only thing that survives a run is what we captured as it happened. That captured evidence is the substrate for evaluation, trust, debugging, and learning — without it, every other principle degrades to anecdote.
6. **Context rot is real; bounded context execution is the response.** LLM output quality decays as a single context window fills — well before the hard token limit. Long-running agent sessions accumulate transcript, tool noise, and failed attempts that compete with the original instructions; the agent at hour one is not the agent at minute one. The structural fix is **bounded context execution** — also known as the **Ralph loop**: every unit of work runs in a fresh, narrowly-scoped context against an explicit contract (a bead with acceptance criteria), with persistent state landing on disk as evidence rather than carried forward as transcript. DDx implements this with `ddx agent execute-bead` (single bounded attempt in an isolated worktree) and `ddx work` (queue drain that re-enters the loop with a fresh context per bead). See `website/content/docs/concepts/bounded-context-execution.md` for the full treatment.

### The Intersection — DDx's Reason to Exist

6. **Human-AI collaboration is the fulcrum.** Abstraction levers intent across the artifact stack, but only collaboration converts leverage into shipped software. Humans supply intent and accountability; AI supplies volume and execution. DDx is the toolkit at the seam — handoffs in both directions, at every level.

## Platform and Workflow

DDx pairs with workflow tools across explicit boundaries:

| Layer | Project | Owns |
|-------|---------|------|
| Platform | **DDx** (this repo) | Document library, bead tracker, agent dispatch, personas, templates, git sync |
| Workflow | **HELIX** | Phases, gates, supervisory dispatch, bounded actions, methodology |

DDx provides primitives. HELIX and others provide opinions. Each layer is
independently useful and replaceable.

## Mission

Make artifacts the unit of software development. Provide the shared
infrastructure that developers and workflow tools use to maintain, compose, and
deliver the artifacts agents produce and consume to build software.

## What DDx Provides

- **Artifact library** — structured, versioned, agent-discoverable artifacts in
  the repository. Documents, diagrams, wireframes, prompts, images, and other
  media; generators checked into git with source-hash-driven regeneration.
  Personas, patterns, templates, MCP server registry. Artifacts may be produced
  or consumed by humans and agents alike; DDx manages identity and provenance
  across all four combinations.
- **Multi-media artifact graph** — artifact identity, relationships, and content
  hashing across document and non-document media. Any file with a sidecar
  `.ddx.yaml` is a first-class artifact.
- **Bead tracker** — work items with a dependency DAG, ready/blocked queues,
  and JSONL interchange. Beads are the unit of work agents execute.
- **Three-layer run architecture** — `ddx run` (single agent invocation), `ddx
  try` (bead attempt in isolated worktree), `ddx work` (queue drain). Agent
  invocation is upstream; DDx owns worktree lifecycle, evidence bundling, and
  drain.
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
the work; stronger models review; deterministic checks sit at the top of the
ladder catching what review missed. The agent service routes by capability,
not by name — endpoints with live model discovery, not hardcoded providers.

### Three-Layer Run Architecture

DDx owns three explicit layered primitives:

| Layer | CLI | Owns |
|-------|-----|------|
| 1 | `ddx run` | Single agent invocation; structured output, tokens, model, exit metadata |
| 2 | `ddx try <bead>` | Bead attempt in isolated worktree; merge or preserve result |
| 3 | `ddx work` | Mechanical queue drain; no-progress detection and stop conditions |

`ddx try` wraps `ddx run`. `ddx work` iterates `ddx try`. One on-disk
substrate; layer metadata distinguishes records. Agent invocation is upstream
(DDx-agent module via CONTRACT-003); DDx owns worktree lifecycle, evidence
bundling, and drain loop.

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

Operating principles are the choices DDx makes in response to the physics above.

1. **Platform, not methodology** — DDx provides primitives; workflow tools provide opinions.
2. **Project-local by default** — install touches the project, not the machine.
3. **Artifacts are the product (documents primary, other media supported)** — code is output; artifacts are what you maintain.
4. **Bead-driven work** — beads are the unit of work; the queue is the interface.
5. **Cost-tiered throughput** — optimize closed beads per dollar, not raw capability.
6. **Git-native, file-first** — plain files, standard git, no lock-in.
7. **Agent-agnostic** — any capable harness plugs in via the prompt envelope.

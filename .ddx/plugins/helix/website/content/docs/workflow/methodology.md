---
title: Methodology
weight: 1
---

HELIX is an opinionated methodology for agentic software development. Named
after the double helix of DNA, it encodes the idea that **planning and
execution are two complementary strands** that happen simultaneously and feed
back into each other. Neither is primary. Neither is sequential. They
intertwine.

## Why HELIX Exists

Three approaches to software development each make a different mistake:

- **Waterfall** assumes you can plan completely, then execute.
- **Agile** (in many implementations) assumes you can execute without planning.
- **Vibe coding / ad-hoc agentic development** assumes agents can infer intent
  from code alone.

HELIX says: **plan and execute simultaneously, at multiple levels of
abstraction, with documentation as the shared context layer.**

## The Double Helix

HELIX operates as two interleaved cycles:

- **Planning helix**: review, plan, validate — creates and refines beads
  (tracked work items)
- **Execution helix**: execute, measure, report — claims beads, does work,
  records results, creates follow-on beads

These cycles run concurrently. A human can refine specs while an agent builds
features. New requirements flow through the planning helix while implementation
advances through the execution helix. The tracker is the shared state between
them.

## Progressive Abstraction Layers

Each layer is a lens at a different zoom level. Changes can enter at any layer
and propagate up, down, or sideways.

| Layer | Artifact | Purpose |
|-------|----------|---------|
| **Discovery** | Research, competitive analysis | Understand the problem space |
| **Vision** | Product Vision | What is the thing? Why does it exist? |
| **Requirements** | PRD | High-level features and constraints |
| **Specification** | Feature Specs + Acceptance Criteria | What each feature does, how we know it works |
| **Design** | Technical Designs, Solution Designs, ADRs | How to build it, what trade-offs we make |
| **Test** | Test suites (from acceptance criteria) | Executable assertions proving requirements are met |
| **Code** | Implementation | The software itself |

Higher layers govern lower layers. Source code reflects what exists, not what
should exist. When layers disagree, HELIX resolves by escalating to the
governing source — not by guessing from code alone.

## Phases

Work moves through five phases. These are not a sequential pipeline — teams
regularly loop back as they learn.

| Phase | Description | Key Activities |
|-------|-------------|----------------|
| **Discover** | Explore the problem space | Market analysis, competitive review, stakeholder research |
| **Frame** | Decompose the problem into structured artifacts | Vision, PRD, Feature Specs, Acceptance Criteria |
| **Plan** | Encode requirements as testable assertions | Convert acceptance criteria into executable tests |
| **Build / Iterate** | Red-green-refactor against tests | Implement code to pass tests, respecting cross-cutting concerns |
| **Polish** | Systematic optimization against metrics | Hypothesis-driven improvement, adversarial review, gap analysis |

## Multi-Directional Workflow

HELIX is **not top-down**. Changes can enter at any layer:

- **Top-down**: Update the vision, and HELIX propagates through specs, designs,
  and implementation.
- **Bottom-up**: See the output of a build, provide feedback, and HELIX routes
  corrections to the right layer.
- **Middle-out**: Refine a feature spec because implementation revealed a
  missing requirement.
- **From running systems**: Interact with the working software, provide
  unstructured feedback ("I don't like how this looks"), and HELIX determines
  which abstraction layer the feedback applies to.

All of these flows use the same commands: `helix evolve` threads changes
through the artifact stack, `helix align` reconciles drift, and `helix run`
picks up the implementation work.

## Artifact Graph

HELIX artifacts form a directed graph of relationships:

```
Vision ──→ PRD ──→ Feature Specs ──→ Acceptance Criteria
                        │
                        ├──→ Technical Designs
                        │
                        └──→ Solution Designs
                                    │
ADRs ←── Cross-cutting Concerns ────┘
  │
  └──→ Context Digests ──→ Execution Beads
```

This graph enables:

- **Impact analysis** — when an artifact changes, identify which others are
  affected
- **Reconciliation** — verify that dependent artifacts remain consistent
- **Context synthesis** — a bead can pull in its full governance chain
  automatically

## Cross-Cutting Concerns

Cross-cutting concerns are first-class artifacts in HELIX. They capture
technology choices, quality attributes, and conventions that apply across
multiple features:

- **Technology stacks**: language, runtime, package manager, test framework
- **Quality attributes**: accessibility, observability, internationalization
- **Conventions**: linting, formatting, naming, error handling

Concerns are declared in the project and folded into every execution bead via
context digests, so agents respect them without being reminded.

## Adversarial Review

HELIX encourages adversarial review as a core practice:

- After an agent completes work, a different agent (or different AI model)
  examines it against the artifact hierarchy
- Reviews check: Does the implementation match the spec? Does the spec still
  align with the PRD? Are cross-cutting concerns respected?
- Review findings become new beads in the tracker, feeding back into the
  planning helix

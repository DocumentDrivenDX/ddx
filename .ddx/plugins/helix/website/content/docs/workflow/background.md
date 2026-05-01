---
title: Theoretical Background
weight: 2
---

HELIX draws on decades of software engineering methodology, combining lessons
from structured planning, iterative delivery, and emerging agentic
development patterns into a unified framework. This page explains the
theoretical foundations and the design choices they motivate.

## The Double Helix Metaphor

HELIX is named after the double helix of DNA. The metaphor encodes a core
thesis: **planning and execution are two complementary strands** that happen
simultaneously and feed back into each other. Neither is primary. Neither is
sequential. They intertwine.

- **Planning strand**: review, plan, validate — creates and refines
  governing artifacts
- **Execution strand**: execute, measure, report — claims work items, does
  the work, records results, creates follow-on work

This structure mirrors how effective teams actually work. A developer
discovers a design flaw during implementation. A review surfaces a missing
requirement. A metric reveals that a feature assumption was wrong. In each
case, the execution strand feeds back into the planning strand, and the
planning strand adjusts what happens next.

## Rejection of False Dichotomies

Three approaches to software development each make a different mistake:

**Waterfall** assumed you could plan completely, then execute. In practice,
requirements change, designs prove infeasible, and the gap between plan and
reality grows until the plan is fiction.

**Agile** (in many implementations) assumed you could execute without deep
planning. Sprints and standups replaced design documents, but the
institutional knowledge moved into people's heads instead of durable
artifacts. When team members leave, the knowledge leaves with them.

**Vibe coding and ad-hoc agentic development** assumes agents can infer
intent from code alone. This works for small tasks but breaks down as
codebases grow — agents loop without progress, specs drift from code, and
the human becomes a full-time dispatcher instead of a decision-maker.

HELIX says: **plan and execute simultaneously, at multiple levels of
abstraction, with documentation as the shared context layer.** This is not a
compromise between the three — it is a different model entirely.

## Progressive Abstraction Layers

HELIX organizes artifacts into layers at different zoom levels. Each layer
answers a different question:

| Layer | Question | Artifact |
|-------|----------|----------|
| Discovery | What problem space are we in? | Research, competitive analysis |
| Vision | What is this and why does it exist? | Product Vision |
| Requirements | What must it do? | PRD |
| Specification | What exactly does this feature do? | Feature Specs, Acceptance Criteria |
| Design | How do we build it? What trade-offs do we make? | Technical Designs, ADRs |
| Test | How do we know it works? | Test suites |
| Code | What exists right now? | Implementation |

The key insight: **changes can enter at any layer and propagate in any
direction.** A bug report enters at the code layer and may propagate up to
the specification layer. A business pivot enters at the vision layer and
cascades down through every layer below. A design review reveals a missing
requirement, which propagates up to the PRD and back down through tests.

This is what makes HELIX fundamentally different from waterfall: the layers
are not sequential stages. They are lenses at different zoom levels, all
active simultaneously.

## The Authority Order

When artifacts at different layers disagree, HELIX resolves the conflict by
deferring to the higher layer. This is the **authority order**:

1. Product Vision
2. Product Requirements (PRD)
3. Feature Specs / Acceptance Criteria
4. Architecture / ADRs
5. Solution / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code

Source code reflects what exists, not what should exist. If a higher-layer
artifact says one thing and the code says another, the code is wrong.

This principle prevents a common failure mode in agentic development: agents
that infer requirements from existing code and propagate implementation
accidents into specifications.

## Adversarial Review

HELIX treats adversarial review as a core practice, not an optional
add-on. After an agent completes work, a different agent (or the same agent
with a review prompt) examines it against the artifact hierarchy:

- Does the implementation match the spec?
- Does the spec still align with the PRD?
- Are cross-cutting concerns respected?
- What drift signals are present?

Review findings become new work items in the tracker. This creates a
feedback loop: execution produces artifacts, review produces findings,
findings become new work, and the cycle continues.

When multiple AI models are available, HELIX alternates them for
adversarial review. Different models have different blind spots; rotating
reviewers catches errors that self-review misses.

## The Artifact Graph

HELIX artifacts form a directed graph of relationships:

```
Vision
  └── PRD
       └── Feature Specs
            ├── Acceptance Criteria
            ├── Technical Designs
            │    └── ADRs
            └── Beads (work items)
                 └── Context Digests ← Cross-cutting Concerns
```

This graph enables:

- **Impact analysis** — what does a change affect?
- **Reconciliation** — are dependent artifacts still consistent?
- **Context synthesis** — a work item can pull in its full governance chain
- **Traceability** — every implementation traces back to governing intent

## Cross-Cutting Concerns

Technology choices, quality requirements, and conventions that apply across
multiple work items and phases are expressed as **cross-cutting concerns**.
Each concern declares what areas it applies to, what practices it requires,
and what drift signals indicate the project is straying from its declared
choices.

Concerns connect to the artifact graph through a knowledge chain:

```
Spike/POC → ADR → Concern → Context Digest → Bead
```

This means a decision made during exploration (a spike) gets recorded (in an
ADR), indexed (as a concern), and automatically injected into every
relevant work item (via context digests). See
[Cross-Cutting Concerns](/docs/workflow/concerns) for the full mechanism.

## The Bounded Execution Loop

HELIX's autopilot (`helix run`) is deliberately bounded. It continuously:

1. Checks the tracker for ready work
2. Selects the highest-leverage next action
3. Executes one bounded pass
4. Reviews the result
5. Repeats until human input is needed or no work remains

The loop follows the **principle of least power**: prefer the smallest
sufficient action. Refine a spec before redesigning a system. Sharpen
issues before implementing. Reconcile artifacts before inventing new ones.

When the loop cannot make safe forward progress — because authority is
missing, ambiguity cannot be resolved, or product judgment is needed — it
stops and tells the user exactly what decision is needed and why.

## Inspirations and Influences

HELIX builds on ideas from many sources:

- **Agile methodologies** (Scrum, XP, Kanban) — iterative delivery,
  test-driven development, continuous integration, and the insight that
  working software is the primary measure of progress
- **Specification-driven development** (SpecKit, formal methods) — the
  discipline of writing specifications before code, and using specs as the
  source of truth
- **Document-driven development** (Gastown, RALPH) — treating documents as
  first-class engineering artifacts that drive execution, not just records
  of what happened
- **Agentic development patterns** (AutoResearch, Jeffrey Emmanuel's
  Agentic Flywheel) — the emerging practice of using AI agents as
  first-class participants in the development loop, with structured
  feedback cycles that compound agent effectiveness over time
- **Test-driven development** (Beck, Astels) — red-green-refactor as a
  discipline, not just a technique; the idea that tests define behavior
  before implementation exists

The synthesis is HELIX's own: a supervisory control system that combines
structured planning with bounded autonomous execution, where documents
drive agents and agents produce documents, in a continuous intertwined loop.

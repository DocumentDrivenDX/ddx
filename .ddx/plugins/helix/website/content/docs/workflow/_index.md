---
title: Workflow
weight: 2
prev: /docs/getting-started
next: /docs/cli
---

HELIX organizes software delivery into six phases, governed by an authority
hierarchy that ensures upstream artifacts always take precedence.

## Phases

| Phase | Purpose | Key Commands |
|-------|---------|-------------|
| **Frame** | Define the problem — vision, PRD, feature specs, user stories | `helix frame` |
| **Design** | Architect the solution — ADRs, solution designs, technical designs | `helix design` |
| **Test** | Write failing tests that define behavior | (implicit in build) |
| **Build** | Implement code to make tests pass | `helix build`, `helix run` |
| **Deploy** | Release to production with monitoring | (future) |
| **Iterate** | Learn, review, align, and improve | `helix review`, `helix align` |

## Authority Order

When artifacts disagree, HELIX uses this hierarchy:

1. **Product Vision** — what the product should become
2. **Product Requirements** — what must be built
3. **Feature Specs / User Stories** — detailed behavior
4. **Architecture / ADRs** — structural decisions
5. **Solution / Technical Designs** — how to build it
6. **Test Plans / Tests** — verification criteria
7. **Implementation Plans** — build sequencing
8. **Source Code** — current state (not source of truth)

Higher layers govern lower layers. Source code reflects what exists, not what
should exist. If a higher layer is missing, HELIX does not infer intent from
lower layers.

## The Supervisory Loop

`helix run` is the supervisory autopilot. It continuously:

1. **Checks** the tracker for ready work
2. **Selects** the highest-leverage bounded next action
3. **Executes** one pass (build, design, polish, align, etc.)
4. **Reviews** the result with cross-model verification
5. **Repeats** until human input is needed or no work remains

The autopilot uses the principle of least power — prefer refining existing
artifacts over creating new ones, prefer polishing issues over implementing
from incomplete specs.

### When HELIX Stops

HELIX stops and asks for human input when:

- Authority is missing (no governing spec for the work)
- Ambiguity cannot be resolved from existing artifacts
- Product judgment or prioritization is needed
- A decision requires stakeholder approval
- No actionable work remains in the tracker

### Trigger Rules

| Condition | HELIX Action |
|-----------|-------------|
| Ready governed work exists | `build` — bounded implementation |
| Specs changed with open issues | `polish` — refine issues first |
| Requirement change detected | `evolve` or `design` before build |
| Queue drained | `check` — decide next action |
| Periodic interval | `align` — reconciliation audit |
| Build complete | `review` — fresh-eyes cross-model review |

## Tracker-First Execution

The tracker (`ddx bead`) is the steering wheel. Users direct agents by:

- Creating issues with priorities and acceptance criteria
- Setting dependencies between work items
- Approving or blocking work through tracker state
- Closing completed work

Agents read tracker state and execute. The mental model:

```
User <-> Tracker <-> helix run (background)
```

## Epic Focus Mode

When `helix run` picks an epic, it stays focused:

1. Decompose the epic into child tasks
2. Implement children one at a time
3. Review the epic when children complete
4. Only move to the next scope after the epic finishes or is blocked

## Cross-Model Verification

When configured, HELIX alternates AI models for adversarial review:

- **Primary agent** handles reasoning-heavy work (frame, design, evolve, align, review)
- **Alternate agent** handles mechanical work (build, polish, check)

This ensures different models review each other's output, catching blind spots
that self-review misses.

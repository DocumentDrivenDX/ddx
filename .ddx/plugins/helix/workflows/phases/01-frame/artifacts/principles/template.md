---
ddx:
  id: "[artifact-id]"
---

# Project Principles

These principles guide judgment calls across all HELIX phases. They are not
requirements, concerns, ADRs, workflow rules, or process enforcement. They are
lenses applied when choosing between two valid options.

This document was bootstrapped from HELIX defaults. You own it now — add,
modify, reorder, or remove any principle. The only constraint: principles
cannot negate HELIX mechanics (artifact hierarchy, phase gates, tracker
semantics).

## Principles

1. **Design for change** — Prefer structures that are easy to modify over
   structures that are easy to describe today. This changes decisions when a
   tidy short-term model would make likely product changes expensive.

2. **Design for simplicity** — Start with the minimal viable approach.
   Additional complexity requires justification. This changes decisions when a
   generalized solution has no current requirement behind it.

3. **Validate your work** — Every change should be verified through the most
   appropriate means available (tests, type checks, manual verification). This
   changes decisions when speed and evidence pull against each other.

4. **Make intent explicit** — Code, configuration, and documentation should
   make the *why* visible, not just the *what*. This changes decisions when an
   implicit convention would save words but hide rationale.

5. **Prefer reversible decisions** — When uncertain, choose the option that
   is easiest to undo or change later. This changes decisions when confidence
   is low and both options satisfy current requirements.

## Tension Resolution

When principles pull in opposite directions, document the resolution strategy
here. Each entry should name the two principles, describe when they conflict,
and state how to decide.

*No tensions identified yet. As you add project-specific principles, use this
section to resolve any conflicts with existing principles.*

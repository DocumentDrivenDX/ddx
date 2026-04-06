---
name: helix-design
description: 'Create a comprehensive design document through iterative refinement before build work. Public command surface: helix design.'
argument-hint: "[scope]"
disable-model-invocation: true
---

# Design

Create a thorough design document through iterative self-critique and
refinement. Invest deeply in planning quality before any code is written.

This skill depends on shared HELIX planning assets under `workflows/`. If the
shared workflow library is missing, stop and report an incomplete HELIX
package.

## When to Use

- Starting a new feature or project
- When a functionality change requires new or expanded design authority before
  build work can continue
- When changing HELIX itself: workflow contract, tracker semantics, skill
  behavior, installer behavior, or CLI control rules
- Before creating execution issues for a large scope
- When existing planning artifacts are thin or incomplete
- When the implementation path is unclear

## Steps

1. **Load context** — read all existing planning artifacts (vision, PRD,
   architecture, designs), current implementation, and issue queue state.

2. **Draft a comprehensive design** covering: problem statement, requirements,
   architecture decisions with alternatives, interface contracts, data model,
   error handling, security, test strategy, implementation ordering with
   dependency graph, risk register, and observability.

3. **Refine iteratively** (4-5 rounds minimum) — after each draft round,
   assume there are 80+ elements missed or underspecified. Challenge every
   assumption, interface, error path, and edge case. Track change velocity
   per round.

4. **Detect convergence** — stop when substantive changes drop below 5 for
   two consecutive rounds.

5. **Output the design** to `docs/helix/02-design/plan-YYYY-MM-DD[-scope].md`.

6. **Capture execution work** — once the intended design is clear enough,
   create or refine HELIX tracker issues that represent the implementation
   slices needed to make the design real.

## After Design

Run `helix polish` to create and refine issues from the design, then
`helix run --summary` to execute (use `--summary` for token-efficient
monitoring when managing the run from an outer agent).

## References

- Action prompt: `workflows/actions/plan.md`
- Output template: `workflows/templates/plan-document.md`
- CLI: `helix design [--rounds N] [scope]`

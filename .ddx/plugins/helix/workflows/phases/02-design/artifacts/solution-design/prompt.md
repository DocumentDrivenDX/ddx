# Solution Design Generation Prompt

Create a solution design that maps requirements to a concrete approach.

## Purpose

Solution Design is the **feature-level design artifact**. Its unique job is to
translate a Feature Specification into a selected technical approach, domain
model, component decomposition, interface usage, and requirement-to-design
traceability.

It applies Architecture, ADRs, Contracts, and Concerns to one feature or
cross-component capability. It does not redefine the system architecture. It
does not plan a single story's code changes; that belongs in Technical Design.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/arc42-solution-strategy.md` grounds feature-level approach,
  decomposition, interfaces, and concise design rationale.
- `docs/resources/c4-model.md` grounds component and interaction views that
  support feature-level decomposition.

## Focus
- Create a feature-level artifact named `docs/helix/02-design/solution-designs/SD-XXX-[name].md`.
- Show the main options and why the chosen one wins.
- Keep the domain model, decomposition, and tradeoffs concise.
- Cover cross-component system behavior and feature-level structure.
- Do not collapse into story-level implementation details; those belong in a
  technical design (`TD-XXX-*`).
- Preserve only the decisions needed by build and test.
- If the feature requires changing Architecture or an accepted ADR, stop and
  update that governing artifact first.

## Boundary Test

| If you are writing... | Put it in... |
|---|---|
| Product/feature behavior and acceptance criteria | Feature Specification |
| System-wide structure, deployment, and quality attributes | Architecture |
| One architecture-significant decision | ADR |
| Exact API/file/event/CLI surface | Contract |
| Feature-level approach and decomposition | Solution Design |
| Story-level code plan and file/module changes | Technical Design |

## Completion Criteria
- Requirements are mapped.
- Tradeoffs are explicit.
- The chosen approach is clear.
- The output is clearly feature-level and disambiguated from a technical
  design.
- Every P0 requirement has a corresponding design element and test strategy.

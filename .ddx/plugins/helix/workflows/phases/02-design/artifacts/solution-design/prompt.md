# Solution Design Generation Prompt
Create a solution design that maps requirements to a concrete approach.

## Focus
- Create a feature-level artifact named `docs/helix/02-design/solution-designs/SD-XXX-[name].md`.
- Show the main options and why the chosen one wins.
- Keep the domain model, decomposition, and tradeoffs concise.
- Cover cross-component system behavior and feature-level structure.
- Do not collapse into story-level implementation details; those belong in a
  technical design (`TD-XXX-*`).
- Preserve only the decisions needed by build and test.

## Completion Criteria
- Requirements are mapped.
- Tradeoffs are explicit.
- The chosen approach is clear.
- The output is clearly feature-level and disambiguated from a technical
  design.

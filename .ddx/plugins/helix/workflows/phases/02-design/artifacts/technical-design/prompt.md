# Technical Design for User Story Prompt

Create a concise technical design for one user story.

## Purpose

Technical Design is the **story-level implementation design artifact**. Its
unique job is to make one user story buildable by naming the concrete component
changes, files, interfaces, data model changes, security implications, tests,
rollback path, and implementation sequence.

It inherits Architecture and Solution Design. It must not redesign the feature
or system. If the story cannot be implemented without changing the parent
Solution Design, ADR, Contract, or Architecture, update that governing artifact
first.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/google-small-cls.md` grounds bounded implementation slices
  with related tests and rollback.
- `docs/resources/cucumber-executable-specifications.md` grounds mapping
  acceptance criteria to observable tests.

## Focus
- Create a story-level artifact named `docs/helix/02-design/technical-designs/TD-XXX-[name].md`.
- Map each acceptance criterion to component changes, interfaces, data, security, and tests.
- Stay on the vertical slice for the story.
- Assume the broader architecture is already set by the parent solution design.
- Do not expand into a feature-wide or system-wide design; that belongs in a
  solution design (`SD-XXX-*`).
- Keep implementation sequence and rollout or migration notes only when they affect execution.

## Boundary Test

| If you are writing... | Put it in... |
|---|---|
| Feature-wide approach or decomposition | Solution Design |
| One architectural decision | ADR |
| Exact external interface contract | Contract |
| One story's implementation shape, files, tests, and rollback | Technical Design |
| Test fixtures and detailed test cases | Story Test Plan |
| Work queue slicing and execution status | Implementation Plan or DDx bead |

## Completion Criteria
- The story is implementable.
- Key interfaces, changes, and test coverage are explicit.
- The design stays compact.
- The output is clearly story-level and disambiguated from a solution design.
- The implementation sequence can be turned into one or more small,
  reviewable changes without losing test coverage.

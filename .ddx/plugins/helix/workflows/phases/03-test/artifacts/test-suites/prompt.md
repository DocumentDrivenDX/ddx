# Test Suites Generation Prompt

Create the test suite layout for the Red phase. Keep it concise and project-specific: define the suite boundaries, the minimum behavior each suite must cover, and any shared data conventions needed to make the tests executable.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/fowler-practical-test-pyramid.md` grounds suite balance across
  unit, integration, and end-to-end coverage.
- `docs/resources/google-test-sizes.md` grounds runtime and dependency
  expectations for suite grouping.
- `docs/resources/openapi-specification.md` grounds API contract suites where
  interface contracts exist.

## Storage Location

`tests/` at the project root

## Include

- contract, integration, unit, and E2E boundaries
- the behaviors each suite owns
- required fixtures, factories, or mocks
- any coverage target that matters for this stack
- suite ownership, execution commands, and evidence outputs

## Keep Out

- generic TDD teaching text
- oversized code examples
- repeated explanations of why tests come first

Use template at `.ddx/plugins/helix/workflows/phases/03-test/artifacts/test-suites/template.md`.

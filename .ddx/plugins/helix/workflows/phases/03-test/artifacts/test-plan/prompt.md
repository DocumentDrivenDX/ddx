# Test Plan Generation Prompt

Create the project-level test plan for the Test phase. Keep it concise, but include the minimum structure needed to drive failing tests before implementation.

## Purpose

The Test Plan is the **project-level verification strategy**. Its unique job is
to define test levels, coverage targets, critical paths, data strategy,
infrastructure, sequencing, risks, and build handoff commands before
implementation starts.

It does not contain every story-specific test case. Story Test Plans own the
exact executable checks for one story. The Test Plan owns the portfolio: what
must be covered, where confidence should come from, and how CI enforces it.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/google-test-sizes.md` grounds test levels by scope,
  isolation, dependencies, and CI enforcement.
- `docs/resources/fowler-practical-test-pyramid.md` grounds balanced coverage
  across fast focused tests and fewer broad end-to-end tests.

## Storage Location

`docs/helix/03-test/test-plan.md`

## What to Include

- test levels and scope
- framework choices only where they matter
- coverage targets and critical paths
- test data strategy
- sequencing, dependencies, and infrastructure needs
- risks that can block test execution

## Keep In Mind

- tests are the executable specification
- every test should trace to a requirement or story
- coverage targets should be risk-based and enforced, not decorative
- do not add generic testing doctrine that the template already implies

## Boundary Test

| If you are writing... | Put it in... |
|---|---|
| Overall test levels, coverage targets, data strategy, CI gates | Test Plan |
| One story's concrete test cases and fixtures | Story Test Plan |
| Feature behavior or acceptance criteria | Feature Specification or User Story |
| Implementation sequencing for code changes | Implementation Plan |

Use template at `.ddx/plugins/helix/workflows/phases/03-test/artifacts/test-plan/template.md`.

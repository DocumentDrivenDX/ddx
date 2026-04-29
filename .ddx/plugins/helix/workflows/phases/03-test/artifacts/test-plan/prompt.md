# Test Plan Generation Prompt

Create the project-level test plan for the Test phase. Keep it concise, but include the minimum structure needed to drive failing tests before implementation.

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
- do not add generic testing doctrine that the template already implies

Use template at `.ddx/plugins/helix/workflows/phases/03-test/artifacts/test-plan/template.md`.

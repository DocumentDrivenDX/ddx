# Build Plan Generation Prompt

Create the canonical build plan for the Build phase. Keep it short, but preserve the sequencing, issue boundaries, and verification rules needed to execute implementation against the test plan and technical designs.

## Storage Location

`docs/helix/04-build/implementation-plan.md`

## Required Inputs

- `docs/helix/03-test/test-plan.md` and `docs/helix/03-test/test-plans/TP-*.md`
- `docs/helix/02-design/technical-designs/TD-*.md`
- project-level design constraints

## Include

- scope and governing artifacts
- build order and dependencies
- issue decomposition rules
- quality gates and closeout criteria
- risks that should refine upstream artifacts

## Template

`.ddx/plugins/helix/workflows/phases/04-build/artifacts/implementation-plan/template.md`
For tracker conventions see `ddx bead --help`.

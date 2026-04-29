# Test Procedures Generation Prompt

Create concise procedures for writing, running, and maintaining tests. Focus on the steps an implementer needs for this project, not general testing advice.

## Storage Location

`docs/helix/03-test/test-procedures.md`

## Include

- per-test-type writing rules only where they differ
- local and CI execution commands
- validation and quality checks
- the common failure modes worth documenting

Use template at `.ddx/plugins/helix/workflows/phases/03-test/artifacts/test-procedures/template.md`.

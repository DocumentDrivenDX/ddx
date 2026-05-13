# Test Procedures Generation Prompt

Create concise procedures for writing, running, and maintaining tests. Focus on the steps an implementer needs for this project, not general testing advice.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/google-test-sizes.md` grounds different execution
  expectations for small, medium, and large tests.
- `docs/resources/fowler-practical-test-pyramid.md` grounds the balance between
  fast focused tests and fewer broad confidence tests.
- `docs/resources/cucumber-executable-specifications.md` grounds procedures for
  acceptance examples when behavior needs shared language.

## Storage Location

`docs/helix/03-test/test-procedures.md`

## Include

- per-test-type writing rules only where they differ
- local and CI execution commands
- validation and quality checks
- the common failure modes worth documenting
- evidence capture and pass/fail rules

Use template at `.ddx/plugins/helix/workflows/phases/03-test/artifacts/test-procedures/template.md`.

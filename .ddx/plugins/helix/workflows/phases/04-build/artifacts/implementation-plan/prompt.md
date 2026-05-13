# Build Plan Generation Prompt

Create the canonical build plan for the Build phase. Keep it short, but preserve the sequencing, issue boundaries, and verification rules needed to execute implementation against the test plan and technical designs.

## Purpose

The Implementation Plan is the **build sequencing and execution-readiness
artifact**. Its unique job is to translate approved story, technical design, and
test-plan context into bounded implementation slices with dependencies,
validation gates, and closeout evidence.

It is not the tracker. The runtime owns issue state and execution. This artifact
defines the intended build shape so DDx beads or another runtime can execute
without inventing scope, ordering, or validation rules.

## Reference Anchors

Use this local resource summary as grounding:

- `docs/resources/google-small-cls.md` grounds small, reviewable,
  rollback-friendly implementation slices with related tests.

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

## Boundary Test

| If you are writing... | Put it in... |
|---|---|
| Product or feature behavior changes | PRD / Feature Specification / User Story |
| Design or interface decisions | Solution Design / Technical Design / Contract / ADR |
| Exact story tests and fixtures | Story Test Plan |
| Build slice order, dependencies, and validation gates | Implementation Plan |
| Assignee, live status, claim, execution logs | DDx bead or runtime issue |

## Template

`.ddx/plugins/helix/workflows/phases/04-build/artifacts/implementation-plan/template.md`
For tracker conventions see `ddx bead --help`.

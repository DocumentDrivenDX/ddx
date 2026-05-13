# Feature Registry Generation Prompt
Maintain the feature registry as the source of truth for IDs, status, dependencies, ownership, and traceability.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/ibm-requirements-management.md` grounds requirements
  traceability, prioritization, validation, and change management.
- `docs/resources/atlassian-product-backlog.md` grounds visible prioritized
  work, dependency awareness, and refinement.

## Focus
- Assign new FEAT-XXX IDs sequentially.
- Keep status changes and dependencies explicit.
- Preserve traceability to stories, designs, contracts, tests, and code.
- Keep descriptions short; detail belongs in feature specs, stories, designs, and tests.

## Role Boundary

The Feature Registry is not the PRD, backlog, or tracker. It assigns durable
feature identity and preserves feature-level traceability. The PRD defines
requirements; Feature Specifications define behavior; DDx beads track execution.

## Completion Criteria
- Entries are brief and complete.
- IDs are unique and never reused.
- The registry stays easy to scan.
- Every active feature links to its governing artifact or clearly states the missing link.

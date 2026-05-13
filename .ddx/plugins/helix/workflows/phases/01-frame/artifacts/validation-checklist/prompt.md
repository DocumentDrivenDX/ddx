# Validation Checklist Generation Prompt
Create the checklist that decides whether Frame is ready to move forward.

## Reference Anchors

Use this local resource summary as grounding:

- `docs/resources/ibm-requirements-management.md` grounds traceability,
  validation, verification, and change management expectations.

## Focus
- Check only the gates that matter: completeness, consistency, traceability, and stakeholder approval.
- Keep the pass/fail criteria concrete.
- Avoid duplicating the content already covered by source artifacts.
- Cite evidence or the missing artifact for every conditional or failed gate.

## Role Boundary

Validation Checklist is not a replacement for PRD review, risk review, or
stakeholder review. It is the final Frame phase gate: it records whether the
required artifacts are coherent enough for Design and what conditions remain.

## Completion Criteria
- The checklist is short and actionable.
- Blocking gaps are easy to spot.
- Cross-references are verified.
- Result is Pass, Conditional Pass, or Fail with named conditions.

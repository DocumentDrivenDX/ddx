# Compliance Requirements Analysis Prompt
Document the compliance obligations for this project in the local template.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/ftc-safeguards-rule.md` grounds financial customer
  information security obligations and applicability caveats.
- `docs/resources/nist-privacy-framework.md` grounds privacy risk management,
  data processing, controls, and validation evidence.

## Focus
- Identify only the regulations and standards that actually apply to this system.
- Mark uncertain applicability explicitly and route it to counsel or compliance review.
- Map each obligation to its source, affected scope, concrete controls, owners, evidence, and timing.
- Keep the result concise and implementation-relevant.
- Do not invent legal conclusions. State assumptions and review gaps.

## Role Boundary

Compliance Requirements is not Security Requirements or the Threat Model. It
defines external obligations and the controls/evidence needed to satisfy them.
Security Requirements turns those obligations into security behavior; Threat
Model analyzes abuse paths and mitigations.

## Completion Criteria
- Applicable, not-applicable, and uncertain obligations are identified.
- Controls, evidence, owners, and deadlines are explicit.
- No generic filler is added.

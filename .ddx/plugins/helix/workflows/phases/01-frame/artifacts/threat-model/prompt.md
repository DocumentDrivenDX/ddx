# Threat Modeling Prompt
Document the project threat model with enough detail to drive mitigations.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/owasp-threat-modeling-cheat-sheet.md` grounds assets, data
  flows, trust boundaries, STRIDE, assumptions, and mitigation mapping.
- `docs/resources/owasp-asvs.md` grounds mapping threat mitigations into
  verifiable security controls.

## Focus
- Define boundaries, assets, and trust changes first.
- Analyze threats with STRIDE and map them to controls.
- Keep risk scoring and mitigation ownership explicit.
- Treat missing boundaries, unclear assets, or unstated assumptions as findings.

## Role Boundary

Threat Model is not Security Requirements or Security Architecture. It explains
what can go wrong and where. Security Requirements state what controls must be
true; Security Architecture places those controls in the design.

## Completion Criteria
- The threat surface is clear.
- Important threats are prioritized.
- Mitigations are concrete.
- High-risk threats have owners and verification hooks.

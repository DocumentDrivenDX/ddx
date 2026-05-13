# Security Requirements Generation Prompt
Document the security requirements the project must satisfy before design and build.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/owasp-asvs.md` grounds application security requirements and
  verification expectations.
- `docs/resources/ftc-safeguards-rule.md` grounds financial customer
  information safeguards and applicability caveats.
- `docs/resources/nist-privacy-framework.md` grounds privacy risk management
  and data-processing controls.

## Focus
- Cover authentication, authorization, data protection, privacy, validation, and logging.
- Turn requirements into concrete controls and tests where possible.
- Keep compliance and risk notes brief but explicit.
- Make every requirement verifiable by design review, automated test, manual test, or audit evidence.

## Role Boundary

Security Requirements is not the Threat Model or Security Architecture. It
states what security outcomes and controls must be true. Threat Model explains
how the system can be attacked; Security Architecture explains where and how
controls are implemented.

## Completion Criteria
- Required controls are identified.
- Risks and assumptions are visible.
- The result is specific enough to guide design.
- Acceptance criteria are testable and traceable to controls, risks, or compliance obligations.

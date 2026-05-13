# Security Tests Generation
Create concise, project-specific security tests that map threats and security requirements to executable checks.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/owasp-asvs.md` grounds verifiable application security
  controls.
- `docs/resources/owasp-wstg.md` grounds concrete web and API security test
  scenarios.
- `docs/resources/owasp-threat-modeling-cheat-sheet.md` grounds traceability
  from threats and mitigations to verification.

## Focus
- Cover the highest-risk threats first.
- Use a small threat-to-test matrix rather than broad prose.
- Include only the fixtures, setup, tooling, and controls that this stack actually needs.
- Treat scanner output as evidence, not as a substitute for threat-specific
  tests.

## Completion Criteria
- Relevant threat coverage is explicit.
- Expected failures and pass criteria are clear.
- The output is usable in the Red phase.
- Residual risks are named with an owner or follow-up.

# Architecture Decision Record (ADR) Generation Prompt

Write a compact ADR that captures one architecture-significant decision, the
alternatives, and the consequences.

## Purpose

An ADR is the **single-decision record** for architecture-significant choices.
Its unique job is to preserve why a decision was made, what alternatives were
considered, what tradeoffs were accepted, and when the decision should be
revisited.

ADRs are not architecture documents. Architecture owns the overall structural
model. ADRs are not solution designs or technical designs; those apply accepted
decisions to narrower scopes. ADRs are not meeting notes; keep only the context
that changes how future readers should evaluate the decision.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/adr-github-organization.md` grounds ADRs as
  single-decision records with rationale, tradeoffs, and consequences.
- `docs/resources/google-cloud-architecture-decision-records.md` grounds ADR
  traceability to architecture evolution, code, and infrastructure context.

## Focus
- State the context and decision plainly.
- Keep alternatives and tradeoffs honest but brief.
- Note validation and references only if they affect the decision.
- Use one ADR per decision. If the decision has independent parts, split it.
- Treat accepted ADRs as history. New decisions supersede old records instead
  of rewriting them.

## Boundary Test

| If you are writing... | Put it in... |
|---|---|
| Overall system structure or deployment topology | Architecture |
| One architecture-significant decision and rationale | ADR |
| Feature-specific design applying accepted architecture | Solution Design |
| Story-level component or code plan | Technical Design |
| API schema, event payload, or file format | Contract |
| Work steps or sequencing | Implementation Plan |

## Completion Criteria
- The decision is unambiguous.
- Alternatives are compared clearly.
- Consequences are explicit.
- Status and supersession state are clear.
- Reconsideration triggers are concrete when the decision has uncertainty.

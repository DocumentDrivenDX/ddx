---
ddx:
  id: "[artifact-id]"
---

# ADR-[NUMBER]: [Title]

| Date | Status | Deciders | Related | Confidence |
|------|--------|----------|---------|------------|
| [YYYY-MM-DD] | [Proposed/Accepted/Deprecated/Superseded] | [Names] | [FEAT-XXX] | [High/Med/Low] |

## Context

| Aspect | Description |
|--------|-------------|
| Problem | [Specific problem] |
| Current State | [Existing situation] |
| Requirements | [Key requirements driving this] |
| Decision Drivers | [Forces that make this architecture-significant] |

## Decision

We will [decision statement].

**Key Points**: [Point 1] | [Point 2] | [Point 3]

## Alternatives

| Option | Pros | Cons | Evaluation |
|--------|------|------|------------|
| [Option 1] | [Advantages] | [Disadvantages] | [Rejected: reason] |
| [Option 2] | [Advantages] | [Disadvantages] | [Rejected: reason] |
| **[Selected]** | [Advantages] | [Disadvantages + mitigations] | **Selected: reason** |

## Consequences

| Type | Impact |
|------|--------|
| Positive | [Good outcomes] |
| Negative | [Trade-offs, technical debt] |
| Neutral | [Side effects] |

## Risks

| Risk | Prob | Impact | Mitigation |
|------|------|--------|------------|
| [Risk 1] | H/M/L | H/M/L | [Strategy] |

## Validation

| Success Metric | Review Trigger |
|----------------|----------------|
| [Metric 1] | [Condition for reconsideration] |

## Supersession

- **Supersedes**: [ADR-XXX or None]
- **Superseded by**: [ADR-YYY or None]

## Concern Impact

If this decision affects the project's active concerns or overrides a
library practice, note the impact here:

- **Concern selection**: [Does this ADR select, change, or constrain a concern?]
- **Practice override**: [Does this ADR override a library concern practice? If so,
  update `docs/helix/01-frame/concerns.md` Project Overrides with this ADR ref.]
- **No concern impact**: [Delete this section if the ADR has no concern relevance.]

## References

- [PRD section link]
- [Related ADRs]

## Review Checklist

Use this checklist when reviewing an ADR:

- [ ] Context names a specific problem — not "we need to decide about X"
- [ ] Decision statement is actionable — "we will" not "we should consider"
- [ ] At least two alternatives were evaluated
- [ ] Each alternative has concrete pros and cons, not vague assessments
- [ ] Selected option's rationale explains why it wins over the best alternative
- [ ] Consequences include both positive and negative impacts
- [ ] Negative consequences have documented mitigations
- [ ] Risks are specific with probability and impact assessments
- [ ] Validation section defines how we'll know if the decision was right
- [ ] Review triggers define conditions for reconsidering the decision
- [ ] Concern impact section is complete (or explicitly marked as no impact)
- [ ] ADR is consistent with governing feature spec and PRD requirements

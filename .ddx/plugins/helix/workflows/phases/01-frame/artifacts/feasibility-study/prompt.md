# Feasibility Study Generation Prompt
Assess whether the project is feasible and what it would take to proceed.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/doj-feasibility-study.md` grounds pre-commitment
  feasibility, alternatives, decision criteria, and recommendation.
- `docs/resources/eib-project-feasibility.md` grounds option analysis,
  cost-benefit, organizational, compliance, and risk dimensions.

## Focus
- Separate technical, business, operational, and resource feasibility.
- Compare realistic alternatives, including delaying or doing nothing where useful.
- State the recommendation clearly.
- Capture the main risks, constraints, and open questions.
- Tie conclusions to evidence and confidence, not optimism.

## Role Boundary

Feasibility Study is not the Business Case, PRD, or Solution Design. It decides
whether the opportunity is viable enough to justify deeper framing or delivery
commitment. Business Case owns investment return; PRD owns required behavior;
Solution Design owns the chosen implementation approach.

## Completion Criteria
- The recommendation is unambiguous.
- Each feasibility dimension is summarized briefly.
- Assumptions and mitigations are explicit.
- The preferred alternative is justified against at least one rejected alternative.

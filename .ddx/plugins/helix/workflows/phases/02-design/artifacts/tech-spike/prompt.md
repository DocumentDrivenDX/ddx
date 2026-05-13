# Technical Spike Generation Prompt
Use the spike to answer one technical question with evidence.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/safe-spikes.md` grounds spikes as bounded experiments that
  reduce uncertainty before implementation.
- `docs/resources/agile-alliance-sizing-spikes.md` grounds spikes as visible,
  time-boxed learning work rather than hidden delivery work.

## Focus
- State the question, hypothesis, and method.
- Keep the investigation small and measurable.
- Separate spike evidence from production implementation.
- End with findings, limitations, and a recommendation that updates design,
  creates follow-up work, or stops the approach.

## Completion Criteria
- The uncertainty is reduced.
- Evidence is documented.
- The recommendation is actionable and scoped to the evidence.
- Any remaining uncertainty is named.

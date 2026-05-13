# Principles Generation Prompt

Help the user create a project principles document that guides judgment calls
across all HELIX phases.

## Purpose

Project Principles define the project's durable judgment model. Their unique
job is to help agents and humans choose between two plausible options when the
Product Vision, PRD, feature specs, concerns, ADRs, tests, and implementation
plans do not prescribe an exact answer.

They are not a second requirements document. They are not a concern catalog.
They are not ADRs. They are not workflow rules. A good principle changes a
real decision without pretending to settle every future case.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/agile-manifesto-principles.md` frames principles as durable
  tradeoff preferences that guide many decisions without becoming procedure.
- `docs/resources/govuk-design-principles.md` models compact, memorable,
  decision-changing principles that stay distinct from a rulebook.

## Bootstrap Flow

1. **Check for existing principles**: If `docs/helix/01-frame/principles.md`
   already exists, load it and offer to refine rather than replace.

2. **Load HELIX defaults**: Read `.ddx/plugins/helix/workflows/principles.md` for the baseline
   design principles. Present them to the user as the starting point.

3. **Discovery conversation**: Ask the user three questions to surface
   project-specific values:
   - "What does your project value most?"
   - "What trade-offs do you consistently lean toward?"
   - "What past mistakes should these principles help you avoid?"

4. **Synthesize**: Combine the user's input with the HELIX defaults to
   produce a project principles document. The user may keep, modify, or
   remove any HELIX default. Removed defaults stay removed — HELIX does
   not re-add them.

5. **Tension detection**: For each pair of principles in the result, evaluate
   whether they could pull in opposite directions for a realistic decision.
   Flag any unresolved tensions and ask the user for a resolution strategy.

6. **Write the file**: Output to `docs/helix/01-frame/principles.md` using
   the template from `template.md`. The user owns the file from this point.

## Principles Quality Criteria

Each principle must be:

- **Decision-changing**: It must change at least one real choice. If removing
  the principle would not change any decision, it is not a principle.
- **Actionable**: An agent or developer reading it should know which option
  to prefer in a concrete scenario.
- **Tradeoff-shaped**: It should say what to prefer when two valid options
  compete. "Always do X" is usually a rule, not a principle.
- **Concise**: One sentence for the principle, one sentence for the
  rationale. If it needs a paragraph, it may be a policy, not a principle.

Reject or flag:

- Workflow rules (belong in enforcers or ratchets, not principles)
- Aspirational statements that do not change decisions
- Principles so broad they apply to every project ("write good code")
- Requirements that define product behavior (belong in PRD or feature specs)
- Technology or quality domains (belong in Concerns)
- Specific decisions already made (belong in ADRs)

## Boundary Test

For every candidate principle, ask:

| Question | If yes |
|---|---|
| Does it define what the product must do? | Move it to the PRD or a feature spec. |
| Does it name an active quality area, technology stack, or operating concern? | Move it to Concerns. |
| Does it record a specific decision and alternatives? | Move it to an ADR. |
| Does it require a mandatory process step? | Move it to workflow rules, enforcers, or ratchets. |
| Does it only sound virtuous? | Delete it or rewrite it around a real tradeoff. |

## Size Thresholds

Monitor the number of principles and provide guidance:

- **At 8 principles**: "Consider whether all of these are decision-changing.
  Can any be consolidated?"
- **At 12 principles**: "The Agile Manifesto has 12 and most teams can name
  maybe 4-5. Consider consolidating."
- **At 15+ principles**: "This has grown beyond a decision framework into a
  wish list. Strongly recommend pruning to the principles that actually
  change decisions."

## Tension Detection

When evaluating a set of principles for tensions:

1. Parse each principle into a short semantic summary.
2. For each pair, ask: "Is there a realistic decision where these two
   principles pull in opposite directions?"
3. For each detected tension, check whether the tension resolution section
   already addresses it.
4. Flag unresolved tensions with a concrete example scenario.
5. Accept the user's resolution strategy and add it to the tension
   resolution section of the document.

## Completion Criteria

- The principles document contains only decision-changing principles.
- No workflow rules are included (those belong in enforcers/ratchets).
- All identified tensions have resolution strategies.
- The document is within the size ceiling (ideally 5-8 principles).
- The user has reviewed and approved the final set.

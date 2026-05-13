# Product Vision Prompt

Create a concise product vision that aligns stakeholders on direction before
requirements, design, and delivery work begin.

## Storage Location

Store at: `docs/helix/00-discover/product-vision.md`

## Purpose

A **north star document** that makes the product's direction transferable. Its
unique job is to answer four questions before any PRD exists: who is this for,
what alternative do they use today, what future state are we trying to create,
and how will we know the direction is working?

Every downstream artifact — PRD, specs, designs, tests — traces back to this
document. If the vision is vague, everything built on it drifts.

## Reference Anchors

Use these references as grounding, not as extra sections to copy:

- `docs/resources/product-vision-board.md` captures target group, needs, product direction, and business goals. HELIX uses the same strategic ingredients but keeps detailed business justification in the Business Case.
- `docs/resources/geoffrey-moore-positioning.md` forces the vision to name the target customer, need, category, benefit, and primary alternative.
- `docs/resources/atlassian-vision-creation.md` frames vision as the shared picture of the future that aligns stakeholders before strategy and execution.

## Template Adherence

**The template is the contract.** The 8 sections in `template.md` cover what a
vision should say. Do not add new H2 sections. Do not rename or reorder
existing ones. The body should land within ~1.5× the template's line count
(template is ~70 lines, so target ≤ 105).

If you have content that doesn't fit one of the 8 sections, see "Stay in your
lane" below — it almost certainly belongs in a different artifact type.

## Stay in Your Lane

Product vision is for *direction*. If you find yourself writing about:

| This content | Belongs in |
|---|---|
| Methodology, activities, authority order | `.ddx/plugins/helix/workflows/README.md`, activities glossary |
| Market sizing, revenue model, investment rationale | `00-discover/business-case.md` |
| Competitor analysis or feature-by-feature comparison | `00-discover/competitive-analysis.md` |
| Principles or judgment lenses | `01-frame/principles.md` |
| Risks (likelihood × impact) | `01-frame/risk-register.md` (master), PRD risks (PRD-scoped) |
| Non-goals, "what we won't build" | `01-frame/prd.md` |
| Per-feature requirements | `01-frame/features/FEAT-*.md` |
| UI interaction details (clicks, screens) | feature spec or user story |
| Architecture or technology choices | `02-design/` |
| Release-scoped success metrics | `01-frame/prd.md` (vision keeps strategic, long-horizon outcomes only) |
| Definitional schemas for metrics | `06-iterate/metric-definition.md` |

## Key Principles

- **Be concise** — keep the mission to 1-2 sentences.
- **Be specific** — name target customers, name competitors, state measurable
  outcomes. Placeholders and hedging ("various users", "significant impact")
  are not acceptable.
- **Be compelling** — connect the vision to real customer pain and a concrete
  end state.
- **Be honest** — if you can't fill a section with substance, that's a signal
  the thinking isn't done yet. Flag it rather than filling with platitudes.

## Section-by-Section Guidance

### Mission Statement
Write it so someone outside the team understands what you do in one breath.
Test: could you say this in a single tweet?

### Positioning (Moore's Template)
Fill in every blank with a real noun. "For [target] who [need]" must name a
specific customer segment and a specific pain — not a category. "Unlike
[alternative]" must name an actual product or approach the customer uses today.
If you can't name the alternative, you don't understand the market yet.

### Vision
Describe the desired end state, not a timeline. What changes for users? What
changes in the market? Avoid "we will be the leading..." — describe what the
world looks like, not your position in it.

### User Experience
Walk through a concrete session. Use present tense. Name the actions the user
takes and what the system does in response. This should read like a usage
scenario, not marketing copy.

### Target Market
"Who" must be specific enough to find these people. "Software teams" is too
broad. "Teams of 3-15 engineers using AI coding agents daily who ship
weekly" is specific enough.

### Key Value Propositions
Each row must pass the "so what?" test. The customer benefit column should
describe what changes for the customer, not restate the capability.

### Success Definition
Every metric must be measurable with a tool or process you can name. "User
satisfaction" is not measurable. "NPS > 40 from monthly survey" is.

These are *strategic, long-horizon* outcomes — what success looks like over
12–24 months. Release-scoped metrics belong in the PRD; feature-scoped metrics
in feature specs; metric schemas in `metric-definition`. Aim for 3–5 strategic
indicators, not a comprehensive metric catalog.

### Why Now
Ground this in an observable change — a technology shift, a market event, a
regulatory change, a behavioral trend. "AI is getting better" is too vague.
"Coding agents can now implement bounded tasks reliably but teams lack a
supervisory layer" is grounded.

## Quality Checklist

After drafting, verify every item. If any blocking check fails, revise before
committing.

### Blocking

- [ ] H2 section list exactly matches the template (no added, removed, or renamed sections)
- [ ] Body is ≤ 1.5× the template's line count (currently ≤ 105 lines)
- [ ] Positioning names a specific customer segment (not a category)
- [ ] Positioning names a specific competitor or alternative (not "existing solutions")
- [ ] Target market is specific enough to identify real people
- [ ] Every success metric has a numeric target or measurable outcome
- [ ] User experience section describes a concrete scenario (not abstract benefits)

### Warning

- [ ] Mission fits in a tweet (under 280 characters)
- [ ] Vision describes an end state, not a timeline or market position
- [ ] Why Now cites an observable change, not a general trend
- [ ] Value propositions pass the "so what?" test
- [ ] No section contains only placeholder text
- [ ] No sentence's main predicate is `not <noun>` without a positive predicate in the same sentence

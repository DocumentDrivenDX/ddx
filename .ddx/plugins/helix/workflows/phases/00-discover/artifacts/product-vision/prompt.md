# Product Vision Prompt

Create a concise product vision that aligns stakeholders on mission, direction, and value.

## Storage Location

Store at: `docs/helix/00-discover/product-vision.md`

## Purpose

A **north star document** that keeps direction, value, and success criteria
clear. Every downstream artifact — PRD, specs, designs, tests — traces back to
this document. If the vision is vague, everything built on it drifts.

## Key Principles

- **Be concise** — keep the mission to 1-2 sentences.
- **Be specific** ��� name target customers, name competitors, state measurable
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

### Why Now
Ground this in an observable change — a technology shift, a market event, a
regulatory change, a behavioral trend. "AI is getting better" is too vague.
"Coding agents can now implement bounded tasks reliably but teams lack a
supervisory layer" is grounded.

## Quality Checklist

After drafting, verify every item. If any blocking check fails, revise before
committing.

### Blocking

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

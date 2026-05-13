# PRD Generation Prompt

Create a PRD that frames the problem, product scope, priorities, and success
criteria clearly enough that downstream feature specs, designs, tests, and
implementation work can trace back to it.

## Storage Location

Store at: `docs/helix/01-frame/prd.md`

## Purpose

The PRD is the **product-scope authority for what to build and why**. Its
unique job is to translate the Product Vision into prioritized, measurable
requirements and boundaries. It sits between the product vision (which defines
direction) and feature specs (which define feature-level detail). Every design
decision and implementation choice should trace back to a PRD requirement.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/atlassian-prd.md` frames a PRD as shared understanding of purpose, behavior, user needs, assumptions, out-of-scope items, and success criteria.
- `docs/resources/aha-prd-template.md` supports concise cross-functional scope: what is being built, who it is for, and how it delivers value.
- `docs/resources/ibm-requirements-management.md` grounds measurable, prioritized, traceable requirements and validation/verification discipline.

## Key Principles

- **Problem first** — the problem section should make someone feel the pain
  before they see the solution.
- **Decision-oriented** — every section should help someone make a build/skip
  decision. If a section doesn't inform a decision, it's filler.
- **Testable requirements** — every P0 requirement should be verifiable. If
  you can't describe how to test it, it's too vague.
- **Traceable boundaries** — requirements should connect upward to the Product
  Vision and downward to feature specs, designs, tests, and build work.
- **Honest non-goals** — non-goals should exclude things someone might
  reasonably expect to be in scope. "Not a replacement for X" only matters if
  someone might assume it is.

## Stay in Your Lane

Product requirements are for product scope. If you find yourself writing about:

| This content | Belongs in |
|---|---|
| Market sizing, ROI, investment case | `00-discover/business-case.md` |
| Positioning, target market, long-horizon strategic success | `00-discover/product-vision.md` |
| Detailed feature behavior and edge cases | `01-frame/features/FEAT-*.md` |
| User journey phrasing independent of product-level requirements | `01-frame/user-stories.md` |
| Architecture choices or implementation approach | `02-design/` |
| Detailed test cases and fixtures | `03-test/` |
| Build sequencing and execution slices | `04-build/implementation-plan.md` |

## Section-by-Section Guidance

### Summary
Write this last. This section must work as a **standalone 1-pager**: what
we're building, who uses it, the problem, the solution approach, and the top
2-3 success metrics. Someone who reads only this section should understand the
product well enough to decide whether to invest time in the full PRD. Test:
could a new team member read this alone and explain the product to someone
else?

### Problem
Describe the failure mode, not the absence of your solution. "Users don't have
a X" is weak. "Users spend N hours/week doing Y manually because Z doesn't
exist, leading to W failures" is strong. Quantify where possible.

### Goals
Each goal should describe a state change, not an activity. "Build a dashboard"
is an activity. "Operators can see system health without SSH" is a state
change.

### Success Metrics
Every metric needs three things: what you're measuring, a numeric target, and
how you'll measure it. "User satisfaction" is not a metric. "NPS > 40 from
monthly survey of active users" is. Drop the Timeline column — success metrics
should define what success looks like, not when it happens.

### Non-Goals
Each non-goal should prevent scope creep on something plausible. "Not a
general-purpose AI" is only useful if someone might think it is. Test: would
someone on the team argue for including this? If not, it's not a useful
non-goal.

### Personas
Name them. Give them a role, goals, and pain points specific enough to
validate with a real person. "Alex the Developer" with generic goals is a
template, not a persona.

### Requirements (P0/P1/P2)
P0 = the product is broken without this. P1 = the product is weak without
this. P2 = the product is better with this. If you have more than 7 P0s,
you're not prioritizing.

Each requirement should be stable enough to trace into feature specs and tests.
If a requirement describes a screen, algorithm, API field, or implementation
sequence in detail, move that detail downstream and keep the PRD at product
scope.

### Functional Requirements
These are the detailed behavioral specs. Each one should be testable — someone
reading it should know how to write an acceptance test. Organize by subsystem
or user flow, not by priority.

### Acceptance Test Sketches
For each P0 requirement, write a concrete scenario: what the user does, what
input they provide, and what observable result they see. These aren't full test
cases — they're the minimum an implementer (human or agent) needs to verify
the requirement is met. An AI agent should be able to read a sketch and write
a passing test without asking clarifying questions.

### Technical Context
Name the stack, key dependencies with versions, API schemas, and platform
targets. Be specific enough that an implementer knows what to install and what
interfaces to code against. "React" is not enough — "React 18 with TypeScript
5.x and Vite 6" is. If there's an API schema (OpenAPI, GraphQL SDL), point to
it. This section exists because AI agents need concrete dependency and
interface information to produce correct implementations.

**Important**: This section records stack decisions — it does not make them.
Stack selection rationale belongs in ADRs (Architecture Decision Records). If
you're documenting a choice that doesn't have an ADR yet, note it in Open
Questions. If an existing ADR contradicts what you'd write here, the ADR
governs until it's superseded.

### Constraints
Name real constraints, not aspirational ones. "Must work on mobile" is a
constraint only if you'd otherwise skip it. Budget, compliance, and platform
constraints matter most.

### Assumptions
These are bets. When an assumption is wrong, the plan breaks. Name each one
so the team knows what to watch.

### Risks
Each risk needs a concrete mitigation, not "monitor closely." If the
mitigation is monitoring, say what you'll monitor and what triggers action.

### Open Questions
List unresolved items explicitly rather than leaving `[TBD]` markers
scattered through the document. Each question should name who can answer it
and what's blocked by it. This section is honest about what you don't know
yet — it's better to have a clear list of unknowns than a document that
pretends to be complete.

### Success Criteria
These are the acceptance criteria for the entire initiative. They should be
observable outcomes ("operators can do X without Y") not activities ("we
shipped Z").

## Quality Checklist

After drafting, verify every item. If any blocking check fails, revise before
committing.

### Blocking

- [ ] Problem section quantifies the pain or names a specific failure mode
- [ ] Every P0 requirement is testable (someone could write an acceptance test)
- [ ] Every P0 has an acceptance test sketch with inputs and expected outputs
- [ ] Success metrics have numeric targets and named measurement methods
- [ ] Requirements trace upward to the Product Vision and downward to downstream artifacts
- [ ] No `[TBD]`, `[TODO]`, or `[NEEDS CLARIFICATION]` markers in any section except Open Questions
- [ ] Non-goals exclude something a reasonable person might assume is in scope
- [ ] Personas are specific enough to validate with a real user

### Warning

- [ ] Summary works as a standalone 1-pager (problem, solution, metrics)
- [ ] Goals describe state changes, not activities
- [ ] Risk mitigations are concrete actions, not "monitor"
- [ ] P0 requirements number 7 or fewer
- [ ] Assumptions are falsifiable
- [ ] Functional requirements are organized by subsystem or flow, not priority
- [ ] Technical Context names specific versions, not just library names
- [ ] Open Questions name who can answer and what's blocked

---
ddx:
  id: "[artifact-id]"
---

# Product Requirements Document

## Summary

[This section should work as a standalone 1-pager. Include: what we're
building, who uses it, what problem it solves, the solution approach, and the
top 2-3 success metrics. Write this last — it should be a distillation of the
full PRD, not an introduction. Someone who reads only this section should
understand the product well enough to decide whether to read the rest.]

## Problem and Goals

### Problem

[What is broken or missing? Who is affected? Be specific about the failure
mode — not "users struggle with X" but "users spend N hours per week doing X
because Y doesn't exist."]

### Goals

1. [Primary goal — what changes for users]
2. [Secondary goal]

### Success Metrics

| Metric | Target | Measurement Method |
|--------|--------|--------------------|
| [Metric] | [Numeric target] | [Named tool or process] |

### Non-Goals

[What we are explicitly not trying to achieve. Each non-goal should exclude
something a reasonable person might assume is in scope.]

Deferred items tracked in `docs/helix/parking-lot.md`.

## Users and Scope

### Primary Persona: [Name]

**Role**: [Job title/function]
**Goals**: [What they want to achieve]
**Pain Points**: [Current frustrations — specific enough to validate]

### Secondary Persona: [Name]

[Same structure]

## Requirements

Each requirement should trace to the Product Vision and be specific enough to
drive feature specs, designs, tests, and implementation work without embedding
the detailed design here.

### Must Have (P0)

1. [Core capability — what must be true for the product to be usable]

### Should Have (P1)

1. [Important feature — valuable but not blocking launch]

### Nice to Have (P2)

1. [Enhancement — improves experience but can be deferred]

## Functional Requirements

[Detailed behavioral requirements organized by subsystem or user flow. Each
requirement should be testable — someone reading it should know how to verify
whether it's satisfied.]

## Acceptance Test Sketches

[For each P0 requirement, describe a concrete scenario with inputs and
expected outputs. These aren't full test cases — they're the minimum needed
for an implementer (human or agent) to verify the requirement is met.]

| Requirement | Scenario | Input | Expected Output |
|-------------|----------|-------|-----------------|
| [P0 requirement] | [What the user does] | [Specific input or state] | [Observable result] |

## Technical Context

[Stack, key dependencies with versions, API schemas, and platform targets.
Be specific enough that an implementer knows what to install and what
interfaces to code against. This section records current stack decisions — it
does not make them. Stack selection rationale belongs in ADRs. If a choice
here isn't backed by an ADR yet, note it in Open Questions.]

- **Language/Runtime**: [e.g., TypeScript 5.x, Node 20+]
- **Key Libraries**: [e.g., React 18, Tailwind CSS 4]
- **Data/Storage**: [e.g., PostgreSQL 16, Redis 7]
- **APIs**: [e.g., OpenAPI spec at docs/api/v2.yaml]
- **Platform Targets**: [e.g., Linux, macOS; browser: Chrome/Firefox/Safari latest]

## Constraints, Assumptions, Dependencies

### Constraints

- **Technical**: [Platform or technology limits]
- **Business**: [Budget, timeline, resource limits]
- **Legal/Compliance**: [Regulatory requirements]

### Assumptions

- [Key assumptions — what must be true for this plan to work]

### Dependencies

- [External systems, teams, or artifacts this work depends on]

## Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| [Risk] | High/Med/Low | High/Med/Low | [Concrete strategy, not "monitor"] |

## Open Questions

[Unresolved items that need answers before or during implementation. Each
should name who can answer it and what's blocked by it. Prefer explicit
questions here over `[TBD]` markers scattered through the document.]

- [ ] [Question] — blocks [what], ask [who]

## Success Criteria

[What must be true to call the initiative successful. These should be
observable outcomes, not activities.]

## Review Checklist

Use this checklist when reviewing a PRD artifact:

- [ ] Summary works as a standalone 1-pager — someone can decide whether to read the rest
- [ ] Problem statement describes a specific failure mode with concrete cost
- [ ] Goals are outcomes, not activities ("users can X" not "we build Y")
- [ ] Success metrics have numeric targets and named measurement methods
- [ ] Non-goals exclude things a reasonable person might assume are in scope
- [ ] Personas have specific pain points, not generic descriptions
- [ ] P0 requirements are necessary for launch — removing any one makes the product unusable
- [ ] P1/P2 requirements are correctly prioritized relative to each other
- [ ] Every P0 requirement has an acceptance test sketch
- [ ] Requirements can trace upward to the Product Vision and downward to downstream artifacts
- [ ] Functional requirements are testable — each can be verified with specific inputs and expected outputs
- [ ] Technical context names specific versions and interfaces, not vague technology areas
- [ ] Risks have concrete mitigations ("we do X"), not vague strategies ("we monitor")
- [ ] Open questions name who can answer and what is blocked
- [ ] No contradictions between requirements sections
- [ ] PRD is consistent with the governing product vision

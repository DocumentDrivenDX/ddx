---
ddx:
  id: "[artifact-id]"
---

# Feature Specification: FEAT-XXX — [Feature Name]

**Feature ID**: FEAT-XXX
**Status**: [Draft | Specified | Approved]
**Priority**: [P0 | P1 | P2]
**Owner**: [Team/Person]

## Overview

[What this feature is and why it exists. 2-3 sentences connecting this feature
to a specific PRD requirement.]

## Ideal Future State

[Describe the future product behavior once this feature is working well. Focus
on what users can understand, decide, or accomplish. For broad product-surface,
workflow, IA, or documentation features, this section should come before the
problem framing so requirements are pulled toward the desired outcome instead
of only reacting to current pain.]

## Problem Statement

- **Current situation**: [What exists now — be specific]
- **Pain points**: [What is not working and for whom]
- **Desired outcome**: [What success looks like — measurable]

## Functional Areas

[For features that span more than one surface, user mode, workflow stage, or
domain object, map the areas before writing requirements. This prevents
unrelated requirements from collapsing into one list. Omit only when the
feature is a single narrow capability.]

| Area | User question or job | Feature responsibility |
|------|----------------------|------------------------|
| [Area] | [What the user needs to know or do] | [What this feature must provide] |

## Requirements

### Functional Requirements by Area

[Each requirement should be testable. Group requirements by functional area
when the feature has multiple areas. Use stable prefixes that make the scope
clear, such as `HOME-01`, `TYPE-01`, `NAV-01`, or `FR-01` for narrow features.]

#### [Area Name]

[PREFIX-01]. [Requirement]
[PREFIX-02]. [Requirement]

### Acceptance Criteria

[Capture observable examples for the highest-risk or most important
requirements. Given/When/Then phrasing is allowed but not required. Do not
describe implementation steps.]

| Requirement | Scenario | Given | When | Then |
|-------------|----------|-------|------|------|
| [PREFIX-01] | [Observable case] | [Starting state] | [User/system action] | [Expected result] |

### Non-Functional Requirements

- **Performance**: [Specific target, e.g., "95th percentile response < 200ms"]
- **Security**: [Specific requirement, not "must be secure"]
- **Scalability**: [Specific target, e.g., "handles 10k concurrent users"]
- **Reliability**: [Specific target, e.g., "99.9% uptime"]

## User Stories

[List the user stories that implement this feature. Each story is a separate
file in `docs/helix/01-frame/user-stories/`. Reference by ID — do not
duplicate story content here.]

- [US-XXX — Story title](../user-stories/US-XXX-slug.md)
- [US-XXX — Story title](../user-stories/US-XXX-slug.md)

## Edge Cases and Error Handling

[Feature-level edge cases that span multiple stories. Story-specific edge
cases belong in the story file.]

- **[Condition]**: [Expected behavior]

## Success Metrics

[How do we know this feature is working? Metrics specific to this feature,
not the product-level metrics from the PRD.]

- [Metric with target]

## Constraints and Assumptions

- [Constraint or assumption specific to this feature]

## Dependencies

- **Other features**: [FEAT-XXX if this feature depends on another]
- **External services**: [APIs, libraries, or systems this feature requires]
- **PRD requirements**: [Which P0/P1/P2 requirements this addresses]

## Out of Scope

[What this feature explicitly does not cover. Each item should prevent a
plausible scope question.]

## Review Checklist

Use this checklist when reviewing a feature specification:

- [ ] Overview connects this feature to a specific PRD requirement
- [ ] Ideal future state describes the desired user-visible outcome, not only current problems
- [ ] Problem statement describes what exists now and what is broken — not just what is wanted
- [ ] Functional areas are mapped when the feature spans multiple surfaces, workflows, or domain objects
- [ ] Requirements are grouped by functional area when a flat list would mix unrelated scopes
- [ ] Domain objects that sound similar are explicitly separated (for example, artifact instances vs artifact types)
- [ ] Every functional requirement is testable — you can write an assertion for it
- [ ] Acceptance criteria cover important happy paths, errors, and edge cases with observable outcomes
- [ ] Non-functional requirements have specific numeric targets, not "must be fast"
- [ ] Edge cases cover realistic failure scenarios, not just happy paths
- [ ] Success metrics are specific to this feature, not product-level metrics
- [ ] Dependencies reference real artifact IDs (FEAT-XXX, external APIs)
- [ ] Out of scope excludes things someone might reasonably assume are in scope
- [ ] No implementation details ("use X library", "create Y table") — specify WHAT not HOW
- [ ] Feature is consistent with governing PRD requirements
- [ ] No `[NEEDS CLARIFICATION]` markers remain unresolved for P0 features

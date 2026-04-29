---
dun:
  id: FEAT-XXX
  depends_on:
    - helix.prd
---
# Feature Specification: FEAT-XXX — [Feature Name]

**Feature ID**: FEAT-XXX
**Status**: [Draft | Specified | Approved]
**Priority**: [P0 | P1 | P2]
**Owner**: [Team/Person]

## Overview

[What this feature is and why it exists. 2-3 sentences connecting this feature
to a specific PRD requirement.]

## Problem Statement

- **Current situation**: [What exists now — be specific]
- **Pain points**: [What is not working and for whom]
- **Desired outcome**: [What success looks like — measurable]

## Requirements

### Functional Requirements

[Each requirement should be testable. Number them for traceability.]

1. [Requirement]
2. [Requirement]

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
- [ ] Problem statement describes what exists now and what is broken — not just what is wanted
- [ ] Every functional requirement is testable — you can write an assertion for it
- [ ] Non-functional requirements have specific numeric targets, not "must be fast"
- [ ] Edge cases cover realistic failure scenarios, not just happy paths
- [ ] Success metrics are specific to this feature, not product-level metrics
- [ ] Dependencies reference real artifact IDs (FEAT-XXX, external APIs)
- [ ] Out of scope excludes things someone might reasonably assume are in scope
- [ ] No implementation details ("use X library", "create Y table") — specify WHAT not HOW
- [ ] Feature is consistent with governing PRD requirements
- [ ] No `[NEEDS CLARIFICATION]` markers remain unresolved for P0 features

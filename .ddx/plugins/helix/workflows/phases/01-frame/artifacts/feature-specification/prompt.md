# Feature Specification Generation Prompt

Create a feature specification that is precise enough to support design,
user story creation, and test planning.

## Storage Location

Store at: `docs/helix/01-frame/features/FEAT-NNN-<name>.md`

## Purpose

A feature spec defines the **scope and requirements** for one major
capability. It sits between the PRD (which defines what the product needs)
and user stories (which define vertical slices for implementation). The
feature spec owns requirements; user stories own the user journey.

## Key Principles

- **Scope, not solution** — describe what the feature must do, not how to
  build it. Implementation details belong in design docs.
- **One feature, one spec** — if a spec covers two independent capabilities,
  split it.
- **Stories by reference** — list user story IDs, don't duplicate story
  content. Stories are separate files with their own lifecycle.
- **Testable requirements** — every functional requirement should be
  verifiable. If you can't describe how to test it, it's too vague.
- **Leave unknowns explicit** — use Open Questions at the bottom rather than
  inventing detail you don't have.

## Section-by-Section Guidance

### Overview
Connect this feature to a specific PRD requirement. "This feature implements
PRD P0-3" is better than "This feature improves the user experience."

### Problem Statement
Same standard as the PRD: describe the failure mode, not the absence of your
feature. Quantify where possible.

### Functional Requirements
Number each requirement for traceability. Each one should be independently
testable. These are what the feature must do — user stories describe how
users interact with these capabilities.

### Non-Functional Requirements
Every NFR needs a specific target. "Must be fast" is not a requirement.
"95th percentile response under 200ms" is. Only include NFRs relevant to
this specific feature, not product-wide NFRs from the PRD.

### User Stories
Reference by ID and title with a relative link. Do not duplicate story
content — the story file is the source of truth. If stories haven't been
written yet, list placeholders with `[TODO: create story]` and note it in
Open Questions.

### Edge Cases and Error Handling
Feature-level edge cases that span multiple stories. If an edge case is
specific to one story, it belongs in that story's file.

### Success Metrics
Feature-specific metrics, not product-level metrics from the PRD. How do
you know this specific feature is working as intended?

### Dependencies
Name specific feature IDs, external APIs, and PRD requirement numbers.
"Depends on auth" is too vague. "Depends on FEAT-002 (auth middleware)
and the OAuth2 provider API" is specific.

### Out of Scope
Each item should prevent a plausible scope question during implementation.
"Not a replacement for the database" is only useful if someone might think
it is.

## Quality Checklist

After drafting, verify every item. If any blocking check fails, revise before
committing.

### Blocking

- [ ] Overview links to a specific PRD requirement
- [ ] Every functional requirement is testable
- [ ] Non-functional requirements have specific numeric targets
- [ ] User stories are referenced by ID (not duplicated inline)
- [ ] Dependencies name specific feature IDs and external systems
- [ ] No `[NEEDS CLARIFICATION]` markers remain

### Warning

- [ ] Problem statement quantifies the pain
- [ ] At least one feature-level edge case documented
- [ ] Success metrics are feature-specific (not product-level)
- [ ] Out of scope excludes something plausible

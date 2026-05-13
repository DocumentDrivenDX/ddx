# Feature Specification Generation Prompt

Create a feature specification that is precise enough to support design,
user story creation, and test planning.

## Storage Location

Store at: `docs/helix/01-frame/features/FEAT-NNN-<name>.md`

## Purpose

A feature spec is the **feature-level authority for behavior and boundaries**.
It translates PRD requirements into precise feature behavior, functional areas,
acceptance criteria, non-functional expectations, edge cases, and
feature-specific success measures.

It sits between the PRD (which defines product scope) and user stories (which
define vertical slices through the feature). The feature spec owns feature
behavior. User stories own user journeys. Solution and technical designs own
how the behavior will be built.

## Reference Anchors

Use these local resource summaries as grounding:

- `docs/resources/ibm-requirements-management.md` grounds traceable,
  prioritized, verifiable requirements.
- `docs/resources/cucumber-executable-specifications.md` grounds concrete
  examples as readable acceptance specifications without prescribing
  implementation or tooling.

## Key Principles

- **Future state before current pain** — describe the desired user-visible
  outcome before optimizing around today's broken surface. The problem statement
  explains why the change is needed; it should not be the only organizing frame.
- **Scope, not solution** — describe what the feature must do, not how to
  build it. Implementation details belong in design docs.
- **Behavior, not journey** — specify feature behavior and acceptance criteria.
  Put end-to-end user flow narrative in user stories.
- **One feature, one spec** — if a spec covers two independent capabilities,
  split it.
- **Functional areas before requirements** — when a feature spans multiple
  surfaces, user modes, workflow stages, or domain objects, name those areas
  before writing requirements. Group requirements by area instead of producing
  one flat list.
- **Separate similar domain objects** — if readers might confuse two things,
  define them separately before requirements. For example, "Artifacts" are
  project-specific instances; "Artifact Types" are reusable methodology
  definitions.
- **Stories by reference** — list user story IDs, don't duplicate story
  content. Stories are separate files with their own lifecycle.
- **Testable requirements** — every functional requirement should be
  verifiable. If you can't describe how to test it, it's too vague.
- **Concrete acceptance examples** — add examples for important rules,
  permissions, errors, and edge cases. They should show observable behavior,
  not internal steps.
- **Leave unknowns explicit** — use Open Questions at the bottom rather than
  inventing detail you don't have.

## Boundary Test

| If you are writing... | Put it in... |
|---|---|
| Product goals, personas, launch priority, or product-level metrics | PRD |
| Feature behavior, boundaries, acceptance criteria, and edge cases | Feature Specification |
| A vertical user journey through one or more feature requirements | User Story |
| Component choices, data model, APIs, or implementation approach | Solution/Technical Design |
| Detailed test cases, fixtures, or automation strategy | Test Plan or Story Test Plan |
| Build sequencing and work slices | Implementation Plan |

## Section-by-Section Guidance

### Overview
Connect this feature to a specific PRD requirement. "This feature implements
PRD P0-3" is better than "This feature improves the user experience."

### Ideal Future State
Describe the target state in user-visible terms. A good future state answers:

- What can the user understand, decide, or accomplish?
- What does the product surface make clear?
- How should the feature feel when it is working well?

For IA, documentation, onboarding, workflow, or product-surface features, this
section is mandatory. It should lead the spec toward the desired experience,
not merely away from the current failure mode.

### Problem Statement
Same standard as the PRD: describe the failure mode, not the absence of your
feature. Quantify where possible. Keep it subordinate to the future state; do
not let the spec become a list of current complaints.

### Functional Areas
Use this section whenever a feature has more than one surface, reader mode,
workflow stage, or domain object. The area map should make clear what belongs
where before requirements are written.

Examples:

- Home, Why, Use, Artifact Types, Artifacts, Concerns, Reference, Navigation
- Intake, Planning, Execution, Review, Reporting
- Admin, Operator, End user, Auditor
- API contract, CLI surface, generated docs, validation

### Functional Requirements
Number each requirement for traceability. Group requirements by functional
area when the feature spans multiple areas. Use stable prefixes that make the
scope clear (`NAV-01`, `TYPE-01`, `ART-01`) or use plain `FR-01` for narrow
single-area features.

Each requirement should be independently testable. These are what the feature
must do — user stories describe how users interact with these capabilities.

If a requirement mentions two areas joined by "and", split it unless the
relationship between those areas is itself the requirement.

### Acceptance Criteria
Capture observable examples for the highest-risk or most important
requirements. Use concise Given/When/Then phrasing if it helps, but do not
require Cucumber tooling. Each example should identify the requirement it
validates and the expected result.

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
- [ ] Ideal Future State is present for broad product-surface, workflow, IA, or documentation features
- [ ] Functional Areas is present when the feature spans multiple surfaces, workflows, user modes, or domain objects
- [ ] Similar domain objects are separated before requirements are written
- [ ] Functional requirements are grouped by area when a flat list would mix unrelated scopes
- [ ] Every functional requirement is testable
- [ ] Acceptance criteria cover the highest-risk requirements with observable examples
- [ ] Non-functional requirements have specific numeric targets
- [ ] User stories are referenced by ID (not duplicated inline)
- [ ] Dependencies name specific feature IDs and external systems
- [ ] No `[NEEDS CLARIFICATION]` markers remain

### Warning

- [ ] Problem statement quantifies the pain
- [ ] At least one feature-level edge case documented
- [ ] Success metrics are feature-specific (not product-level)
- [ ] Out of scope excludes something plausible

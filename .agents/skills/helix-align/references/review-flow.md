# Review Flow

## Authority Order

1. Product Vision
2. Product Requirements
3. Feature Specs / User Stories
4. Architecture / ADRs
5. Solution / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code / Build Artifacts

Rules:

- Higher layers govern lower layers.
- Tests govern build execution but do not override requirements or design.
- Source code reflects current state but does not redefine the plan.
- If a higher layer is missing or contradictory, do not infer intent from lower layers.

## Review Sequence

### 1. Bootstrap

- verify `ddx bead` is available
- if tracker access is missing or unhealthy, stop immediately
- create or reconcile one review epic:
  - `type: epic`
  - labels: `helix`, `phase:review`, `kind:review`
- create or reconcile one child review issue per functional area:
  - `type: task`
  - parent: review epic
  - labels: `helix`, `phase:review`, `kind:review`, plus area labels

### 2. Reconstruct Intent

Using planning artifacts only, summarize:

- vision
- requirements
- feature specs / user stories
- architecture / ADRs
- solution / technical designs
- test plans
- implementation plans

### 3. Validate Planning Stack

Check:

- Vision -> Requirements
- Requirements -> Feature Specs / User Stories
- Feature Specs / User Stories -> Architecture
- Architecture -> Solution / Technical Designs
- Technical Designs / Test Plans -> Implementation Plans
- Specs / Stories / Designs -> Tests

Identify:

- contradictions
- missing links
- underspecified areas
- stale artifacts
- same-layer conflicts

### 4. Review Implementation

Map implementation to the planning stack:

- package/module topology
- entry points
- interfaces
- tests
- feature flags / config switches
- unplanned or orphaned code paths

#### Acceptance Criteria Validation

For each user story and feature spec in scope:

- extract acceptance criteria from the governing artifact
- classify each as SATISFIED, TESTED_NOT_PASSING, UNTESTED, or UNIMPLEMENTED
- record in the Gap Register with evidence

### 5. Classify Gaps

Use exactly one:

- ALIGNED
- INCOMPLETE
- DIVERGENT
- UNDERSPECIFIED
- STALE_PLAN
- BLOCKED

For each gap include:

- planning evidence
- implementation evidence
- explanation
- resolution direction: `code-to-plan`, `plan-to-code`, `decision-needed`, or `quality-improvement`
- owning review issue

#### Quality Evaluation

For ALIGNED or INCOMPLETE areas, evaluate robustness, maintainability, and
performance against the planning stack. Record quality concerns as supplementary
Gap Register entries with direction `quality-improvement`.

### 6. Consolidate

Write one durable report at:

- `docs/helix/06-iterate/alignment-reviews/AR-YYYY-MM-DD[-scope].md`

### 7. Generate Execution Issues

Only after consolidation:

- create deterministic execution issues for real gaps
- set the closest governing artifact in `spec-id`
- add blockers with `ddx bead dep add`
- create doc/design issues before code issues when upstream artifacts must change first

#### Issue Coverage Verification

After creating execution issues, verify every non-ALIGNED gap and every
UNTESTED/UNIMPLEMENTED acceptance criterion has at least one covering issue.
Create missing issues before proceeding. The issue set must fully represent the
work to reach the end state defined by the planning stack.

### 8. Output Execution Order

Include:

- dependency chain
- critical path
- parallelizable work
- blockers
- first recommended execution set

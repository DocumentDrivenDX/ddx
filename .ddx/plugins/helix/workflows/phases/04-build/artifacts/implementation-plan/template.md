---
dun:
  id: helix.implementation-plan
  depends_on:
    - helix.test-plan
---
# Build Plan

## Scope

**Governing Artifacts**:
- [docs/helix/01-frame/...]
- [docs/helix/02-design/...]
- [docs/helix/03-test/...]

## Shared Constraints

- [Constraint from requirements, design, architecture, or security]

## Build Sequencing

| Order | Story / Area | Governing Artifacts | Depends On | Notes |
|------|---------------|---------------------|------------|-------|
| 1 | [US-XXX or area] | [TP/TD refs] | None | [Why first] |
| 2 | [US-XXX or area] | [TP/TD refs] | [Dependency] | [Why next] |

## Issue Decomposition

Story-level work is tracked via `ddx bead` in `.ddx/issues.jsonl`.

**Per-issue requirements**:
- Labels: `helix`, `phase:build`, `kind:build`, `story:US-{story-id}`
- References: user story, technical design, story test plan, this build plan
- `spec-id` pointing at the nearest governing artifact
- Blockers as dependency links

| Story / Area | Goal | Dependencies |
|--------------|------|--------------|
| [US-XXX] | [Outcome] | [Deps] |

## Quality Gates

- [ ] Failing tests exist before implementation starts
- [ ] All required tests pass before closing a build issue
- [ ] Behavior changes update canonical documents
- [ ] Code review is complete before phase exit

## Risks

| Risk | Impact | Response |
|------|--------|----------|
| [Risk] | [H/M/L] | [Action] |

## Exit Criteria

- [ ] Build issue set is defined with sequence and dependencies
- [ ] Shared constraints are documented
- [ ] Verification expectations are explicit

## Review Checklist

Use this checklist when reviewing an implementation plan:

- [ ] Governing artifacts are listed and exist on disk
- [ ] Shared constraints trace back to requirements, design, or architecture
- [ ] Build sequence has a justified ordering — not just arbitrary
- [ ] Dependencies between build steps are explicit
- [ ] Each story/area references its governing artifacts (TP, TD)
- [ ] Issue decomposition follows tracker conventions (labels, spec-id, deps)
- [ ] Quality gates are specific and enforceable, not aspirational
- [ ] Risks have concrete responses ("we do X"), not vague strategies
- [ ] Plan is consistent with governing test plan and technical designs

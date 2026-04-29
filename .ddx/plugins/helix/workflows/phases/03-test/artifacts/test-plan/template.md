---
dun:
  id: helix.test-plan
  depends_on:
    - helix.architecture
---
# Test Plan

## Testing Strategy

**Goals**: [Primary objective] | [Quality gates]
**Out of Scope**: [Excluded areas]

### Test Levels

| Level | Coverage Target | Priority |
|-------|-----------------|----------|
| Contract | [Target] | P0/P1 |
| Integration | [Target] | P0/P1 |
| Unit | [Target] | P0/P1 |
| E2E | [Target] | P0/P1 |

### Frameworks

| Type | Framework | Reason |
|------|-----------|--------|
| Contract | [Framework] | [Why] |
| Integration | [Framework] | [Why] |
| Unit | [Framework] | [Why] |
| E2E | [Framework] | [Why] |

## Test Data

| Type | Strategy |
|------|----------|
| Fixtures | [Static data approach] |
| Factories | [Dynamic generation] |
| Mocks | [External service mocking] |

## Coverage Requirements

| Metric | Target | Minimum | Enforcement |
|--------|--------|---------|-------------|
| Line | 80% | 70% | CI blocks |
| Critical | 100% | 100% | Required |

### Critical Paths (P0)

1. [Auth flow]
2. [Core transaction]
3. [Data persistence]
4. [Error handling]

### Secondary Paths (P1-P2)

- P1: [Secondary features] | P2: [Edge cases, rare scenarios]

## Implementation Order
1. [What must be written first and why]
2. [What follows]
3. [What can wait]

## Infrastructure

| Requirement | Specification |
|-------------|---------------|
| CI Tool | [Tool, version] |
| Test DB | [Type, seeding, cleanup] |
| Services | [Required services] |

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Flaky tests | High | Retry logic, isolation |
| Slow execution | Med | Parallelize |

**Known Gaps**: [Limitations and accepted risks]

## Build Handoff

**Commands**: `[test command]` | `[coverage command]`
**Priority**: [Recommended order]

## Review Checklist

Use this checklist when reviewing a test plan:

- [ ] Test levels cover contract, integration, unit, and E2E with coverage targets
- [ ] Framework choices are justified and consistent with project concerns
- [ ] Critical paths (P0) are identified and have 100% coverage requirements
- [ ] Test data strategy covers fixtures, factories, and mocks
- [ ] Coverage requirements have both targets and minimums with enforcement rules
- [ ] Implementation order is justified — what must be tested first and why
- [ ] Infrastructure requirements are specific (tool versions, service deps)
- [ ] Risks include flaky test mitigation and slow execution strategies
- [ ] Known gaps are documented with accepted risk rationale
- [ ] Build handoff commands are concrete and runnable
- [ ] Test plan traces back to acceptance criteria from governing feature specs
- [ ] No untested P0 requirement — every P0 acceptance criterion has a test

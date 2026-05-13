---
ddx:
  id: "[artifact-id]"
---

# Security Tests

**Project**: [Project Name]
**Date**: [Creation Date]

## Scope

- Threats covered: [Threat IDs or control names]
- Required setup: [Environments, accounts, fixtures]
- Out of scope: [What this suite does not test]

## Test Matrix

| Threat / Control | Test ID | Test | Expected Result | Evidence |
|------------------|---------|------|-----------------|----------|
| [Threat] | [SEC-001] | [Test case] | [Expected behavior] | [Command/output/review] |

## Tooling

```yaml
sast:
  tool: [tool]
  trigger: [when it runs]
dast:
  tool: [tool]
  target: [environment]
dependency_scan:
  tool: [tool]
```

## Key Test Cases

### SEC-TC-001: [Name]

**Steps**: [Minimal reproducible steps]
**Expected**: [Expected result]
**Pass Criteria**: [What proves the control works]

### SEC-TC-002: [Name]

**Steps**: [Minimal reproducible steps]
**Expected**: [Expected result]
**Pass Criteria**: [What proves the control works]

## Abuse Cases

| Abuse Case | Test | Expected Control | Evidence |
|------------|------|------------------|----------|
| [Abuse case] | [Test ID] | [Control behavior] | [Evidence] |

## Evidence

| Test ID | Command / Review | Result | Evidence Location |
|---------|------------------|--------|-------------------|
| [SEC-001] | [command or review step] | [pass/fail] | [path/link] |

## Residual Risk

| Risk | Reason Not Fully Covered | Owner | Follow-Up |
|------|--------------------------|-------|-----------|
| [Risk] | [Reason] | [Owner] | [Issue/artifact] |

## Done

- [ ] High-risk threats mapped to tests
- [ ] Applicable controls covered
- [ ] Tests are executable and deterministic
- [ ] Evidence and residual risk are recorded

# Security Test Suite

**Project**: [Project Name]
**Date**: [Creation Date]

## Coverage Summary

- Threats covered: [Threat IDs or control names]
- Required setup: [Environments, accounts, fixtures]
- Out of scope: [What this suite does not test]

## Security Test Categories

- Authentication: [authn controls to verify]
- Authorization: [authz controls to verify]
- Input validation: [injection and tampering checks]
- Data protection: [encryption, masking, logging rules]
- Session management: [token and lifecycle checks]

## Threat Matrix

| Threat / Control | Test | Expected Result | Pass Criteria |
|------------------|------|-----------------|---------------|
| [Threat] | [Test case] | [Expected behavior] | [What proves it passes] |

## Automated Security Testing

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

### SEC-TC-002: [Name]
**Steps**: [Minimal reproducible steps]
**Expected**: [Expected result]

## Compliance and Abuse Cases

- [ ] [Regulatory or policy check]
- [ ] [Abuse or rate-limit check]
- [ ] [Audit/logging check]

## Done

- [ ] High-risk threats mapped to tests
- [ ] Applicable controls covered
- [ ] Tests are executable and deterministic

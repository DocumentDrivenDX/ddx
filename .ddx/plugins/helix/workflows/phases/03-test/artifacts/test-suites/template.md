---
ddx:
  id: "[artifact-id]"
---

# Test Suite Structure

**Project**: [Project Name]
**Test Framework**: [Framework]

## Suite Inventory

| Suite | Path | Scope | Runtime | Required |
|-------|------|-------|---------|----------|
| Unit | `tests/unit/` | [Business rules and pure functions] | [Target] | Yes |
| Integration | `tests/integration/` | [Component and persistence flows] | [Target] | Yes |
| Contract | `tests/contract/` | [API/interface contract] | [Target] | Yes |
| E2E | `tests/e2e/` | [Critical user journeys] | [Target] | [Yes/No] |
| Security | `tests/security/` | [Threat/control checks] | [Target] | [Yes/No] |

## Coverage Mapping

## Contract Tests

| Requirement / Contract | Suite | Test File | Coverage |
|------------------------|-------|-----------|----------|
| [Contract item] | Contract | [file] | [Success, validation, auth, error] |

## Integration Tests

| Flow | Suite | Test File | Coverage |
|------|-------|-----------|----------|
| [Flow] | Integration | [file] | [Coordination, persistence, failure] |

## Unit Tests

| Rule / Module | Suite | Test File | Coverage |
|---------------|-------|-----------|----------|
| [Rule] | Unit | [file] | [Cases] |

## Security Tests

| Threat / Control | Suite | Test File | Coverage |
|------------------|-------|-----------|----------|
| [Threat] | Security | [file] | [Control behavior] |

## Test Data

| Asset | Purpose |
|-------|---------|
| Fixtures | [Canonical valid, invalid, and edge payloads] |
| Factories | [Generated test objects] |
| Mocks | [External services or time/network controls] |

## Execution Commands

```bash
[unit command]
[integration command]
[contract command]
[security command]
```

## Ownership

| Suite | Owner | Review Trigger |
|-------|-------|----------------|
| [Suite] | [Owner] | [When this suite changes] |

## Evidence

| Suite | Evidence Output | Required in CI |
|-------|-----------------|----------------|
| [Suite] | [Path/link] | [Yes/No] |

## Readiness
- [ ] Suite boundaries are defined
- [ ] Shared test data assets are identified
- [ ] All planned suites begin in RED
- [ ] Commands, owners, and evidence outputs are recorded

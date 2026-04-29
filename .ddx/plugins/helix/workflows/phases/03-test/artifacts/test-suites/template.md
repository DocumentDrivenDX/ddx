# Test Suite Structure

**Project**: [Project Name]
**Coverage Target**: [Percentage]%
**Test Framework**: [Framework]

## Test Organization

```
tests/
├── contract/           # API endpoint tests
├── integration/        # Component interaction tests
├── unit/              # Business logic tests
├── e2e/               # End-to-end user journeys
├── fixtures/          # Test data
├── factories/         # Data generators
├── mocks/             # Service mocks
├── helpers/           # Test utilities
└── setup/             # Test configuration
```

## Contract Tests

| Endpoint | Method | Test File | Status |
|----------|--------|-----------|--------|
| /api/[resource] | GET | [resource].get.test.ts | Red |
| /api/[resource] | POST | [resource].post.test.ts | Red |

Coverage: success path, validation, auth, and not-found/conflict behavior.

## Integration Tests

| Component A | Component B | Test File | Status |
|------------|-------------|-----------|--------|
| [Service] | [Repository] | [service]-persistence.test.ts | Red |
| [Service] | [External API] | [service]-api.test.ts | Red |

Coverage: component coordination, persistence, and downstream failure handling.

## Unit Tests

| Module | Function | Test File | Status |
|--------|----------|-----------|--------|
| [validators] | validate[Entity] | validators.test.ts | Red |
| [calculators] | calculate[Metric] | calculators.test.ts | Red |

Coverage: happy path, edge/error cases, and business rules.

## E2E Tests

| Journey | Steps | Critical | Test File | Status |
|---------|-------|----------|-----------|--------|
| [User Registration] | 5 | Yes | registration.e2e.test.ts | Red |
| [Purchase Flow] | 8 | Yes | purchase.e2e.test.ts | Red |

## Test Data

| Asset | Purpose |
|-------|---------|
| Fixtures | [Canonical valid, invalid, and edge payloads] |
| Factories | [Generated test objects] |
| Mocks | [External services or time/network controls] |

## Coverage Targets

| Metric | Target |
|--------|--------|
| Overall Line Coverage | 80% |
| Contract Test Coverage | 100% |
| Critical Path Coverage | 100% |
| Error Handling Coverage | 90% |

## Readiness
- [ ] Suite boundaries are defined
- [ ] Shared test data assets are identified
- [ ] All planned suites begin in RED

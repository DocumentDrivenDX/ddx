# HELIX Action: Write Story Tests

Create failing tests that serve as executable specifications for user stories, enabling the TDD Red-Green-Refactor cycle.

## Prerequisites

- [ ] User story with clear acceptance criteria
- [ ] Design specifications available
- [ ] Test environment and frameworks configured

## Action Workflow

### 1. Analyze Story

For each acceptance criterion, identify:
- Specific behavior, inputs, and expected outputs
- Edge cases and error conditions
- Test level (unit, integration, contract, E2E)
- Test data and mock requirements

### 2. Write Tests

Structure tests by acceptance criterion:

```javascript
describe('User Story: [Story Title]', () => {
  describe('AC1: [Description]', () => {
    it('should [expected behavior] when [condition]', () => {
      // Arrange -> Act (will fail) -> Assert
    });
    it('should handle [edge case]', () => { /* ... */ });
    it('should [error handling] when [error condition]', () => { /* ... */ });
  });
});
```

### 3. Validate

- All tests fail (no implementation exists)
- Each acceptance criterion has corresponding tests
- Tests are independent and can run in isolation

## Outputs

- `tests/unit/[story-id]/` - Unit tests
- `tests/integration/[story-id]/` - Integration tests
- `tests/contract/[story-id]/` - Contract tests
- `tests/e2e/[story-id]/` - E2E tests
- `tests/data/[story-id]/` - Test data specifications

## Test Pyramid

- **Unit (70%)**: Fast, isolated, specific
- **Integration (20%)**: Component interactions
- **E2E (10%)**: Full user journeys

## Quality Gates

- [ ] All acceptance criteria have corresponding tests
- [ ] Edge cases and error conditions covered
- [ ] Tests are independent and repeatable
- [ ] All tests initially fail (Red phase)
- [ ] Test coverage meets project standards

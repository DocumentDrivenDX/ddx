# Write Unit Tests

Create unit tests for all business logic, pure functions, and isolated components. These form the foundation of the testing pyramid.

## Output Location

`tests/unit/` organized by component type: `models/`, `services/`, `utils/`, `components/`

## Requirements

- Minimum 80% code coverage for business logic
- 100% coverage for critical algorithms and data transformations
- All public methods/functions tested
- Edge cases and error conditions covered

## Test Structure (AAA Pattern)

```javascript
describe('ComponentName', () => {
  it('should [expected behavior] when [given specific input]', () => {
    // Arrange - Set up test data and dependencies
    // Act - Execute the function under test
    // Assert - Verify the outcome
  });
});
```

## What to Test

- **Pure functions**: Input/output transformations, calculations, validation, formatting
- **Class methods**: State changes, constructor init, getter/setter behavior
- **Error handling**: Invalid input, null/undefined, exceptions, error messages
- **Edge cases**: Boundary values, empty collections, min/max, special characters

## Mocking Strategy

Mock only external dependencies (APIs, databases, file system, network, time, randomness). Never mock the unit under test.

```javascript
const mockDb = {
  query: jest.fn().mockResolvedValue([{ id: 1, name: 'Test' }]),
  insert: jest.fn().mockResolvedValue({ id: 2 }),
};
const service = new UserService(mockDb);
```

## Naming Conventions

- Files: `{component}.test.{ext}`
- Tests: Start with "should" and describe behavior

## Key Rules

**DO**: Test one thing per test. Keep tests deterministic. Use test data builders. Test public interfaces.

**DON'T**: Test implementation details. Make tests depend on each other. Test external libraries. Write complex test logic.

## Quality Checklist

- [ ] All public methods/functions have tests
- [ ] Edge cases and error scenarios covered
- [ ] Tests are independent and isolated
- [ ] Tests run quickly (< 5 seconds for suite)
- [ ] No hardcoded magic numbers
- [ ] Coverage targets met

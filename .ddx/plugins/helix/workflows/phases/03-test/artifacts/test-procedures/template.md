# Test Procedures

## Pre-Test Setup

- [ ] Test framework configured
- [ ] CI pipeline ready
- [ ] Test data and mocks prepared

## Writing Procedures

### Contract Tests
1. Review the API or interface contract.
2. Add one file per endpoint or command.
3. Cover success, validation, auth, and error paths.
4. Verify the test fails before implementation exists.

### Integration Tests
1. Review dependencies and data flow.
2. Use the smallest realistic mix of real and mocked services.
3. Cover coordination and failure handling.

### Unit Tests
1. Isolate business rules and edge cases.
2. Keep each test focused on one behavior.
3. Confirm the test fails for the right reason.

## Execution

### Local
```bash
npm test
npm test -- --coverage
npm test -- --watch
```

### CI

Run the same suite on push and PRs. Block merges on failure.

## Quality Checklist

- [ ] Test names describe behavior
- [ ] Tests are independent and deterministic
- [ ] Assertions are specific
- [ ] Managed fixtures or factories are used

## Troubleshooting

| Problem | Likely Cause | Fix |
|---------|--------------|-----|
| Passing too early | Implementation or weak assertions | Recheck expectations |
| Poor failure messages | Test is too broad | Split the test |
| Flaky results | Time, state, or external deps | Remove dependency or isolate state |

## Handoff

- [ ] Required tests are written and failing
- [ ] CI is configured
- [ ] Test docs are complete

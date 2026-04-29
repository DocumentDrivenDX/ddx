# Write Integration Tests

Create integration tests verifying component interactions, service boundaries, and data flow. Use real services and dependencies wherever possible.

## Output Location

`tests/integration/` organized by type: `api/`, `database/`, `services/`, `external/`

## Key Principle: Integration-First

- **Use real services over mocks** whenever possible
- **Test in production-like environments** (containerized)
- **Verify actual integration points**, not simulations
- **Maintain test databases** that mirror production structure

## Test Scope

- **Service integration**: App-to-DB, microservice communication, message queues, cache, external APIs
- **Data flow**: Persistence/retrieval, transaction boundaries, event propagation, state sync
- **API contracts**: Request/response formats, error responses, auth/authz, rate limiting

## Test Structure

```javascript
describe('UserService Integration', () => {
  let database, service;

  beforeAll(async () => {
    database = await createTestDatabase();
    await database.migrate();
    service = new UserService(database);
  });
  afterAll(async () => { await database.close(); });
  beforeEach(async () => { await database.truncate(['users', 'sessions']); });

  it('should create and retrieve user with sessions', async () => {
    const user = await service.createUser({ name: 'Test User' });
    const session = await service.createSession(user.id);
    const retrieved = await service.getUserWithSessions(user.id);
    expect(retrieved.sessions).toHaveLength(1);
  });
});
```

## Database Tests

Use real test databases, not mocks. Use `test_${JEST_WORKER_ID}` for parallel support. Test transactions (commit on success, rollback on failure).

## When Mocking is Necessary

Only mock when real service is not feasible (e.g., email, SMS). Still verify the integration point: assert correct parameters were passed to the mock.

## Environment

Use Docker Compose for consistent test environments (postgres, redis, rabbitmq, etc.).

## Quality Checklist

- [ ] All service boundaries have integration tests
- [ ] Database operations tested with real database
- [ ] API contracts validated
- [ ] Transaction boundaries tested
- [ ] Error propagation verified
- [ ] Tests run in < 30s and can run in parallel
- [ ] Test data properly isolated

**Avoid over-mocking**: Don't mock the database, internal services, or message queues. Use containerized real services instead.

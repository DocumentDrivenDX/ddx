# Practices: Testing

## Requirements (Frame phase)

- Every user story must have testable acceptance criteria
- Acceptance criteria must be specific enough to write a failing test before
  implementation starts (TDD)
- Non-functional requirements (latency, throughput, availability) must have
  measurable targets that become ratchet floors

## Design

- Identify test boundaries during design: what is unit-testable, what needs
  integration, what needs E2E
- Design for testability: dependency injection, pure functions, clear
  interfaces at system boundaries
- Plan fake data strategies: what entities need factories, what external
  services need stubs
- Identify invariants for property-based testing during contract design
  ("this function is commutative", "serialization round-trips", "output
  length is bounded by input length")
- Identify failure modes for chaos testing: what happens when the database
  is slow, the queue is full, the external API returns 500

## Implementation

### Test structure
- One test file per module/component, co-located or in a parallel `test/`
  directory
- Test files named `<module>.test.<ext>` or `<module>_test.<ext>`
- Group related tests with `describe` / `context` / `mod tests`
- Each test tests one behavior — if you need "and" in the name, split it

### Test data
- Factory functions with sensible defaults:
  ```
  createTestUser({ role: "admin" })  // everything else has defaults
  ```
- Faker-generated values for strings, numbers, dates, emails, addresses
- Deterministic seeds: set per-test seed, log on failure for replay
- No shared mutable state between tests — each test sets up its own data

### Stubs and boundaries
- Stub at system boundaries: external HTTP APIs, email, SMS, payment
- Use real implementations for internal boundaries: real database, real
  queue, real file system (via temp directories)
- Dependency injection for swappable dependencies — not monkey-patching
  or module-level mocking
- For HTTP stubs: record/replay from real responses where possible, then
  maintain the fixtures as contract snapshots

### Property-based testing
- Define properties as pure boolean functions: `(input) => invariant(fn(input))`
- Start with at least: round-trip (encode/decode), idempotency (applying
  twice = applying once), monotonicity (larger input → larger or equal output),
  and no-crash (arbitrary input does not panic/throw)
- Shrink failing cases to minimal reproduction
- Run property tests in CI with enough iterations (100+ per property)

### Fuzz testing
- Fuzz all parsers, deserializers, and user-input validators
- Fuzz with both random byte sequences and structured mutation
- Corpus: seed with real-world examples, edge cases, and prior crash inputs
- Run fuzz campaigns periodically (nightly CI or dedicated fuzz job) — not
  necessarily on every PR

### Chaos testing
- Start with timeout and retry behavior: inject latency into stubs, verify
  the system retries or degrades gracefully
- Test circuit breakers: stub returns errors → verify fallback activates
- Test data integrity under failure: kill the process mid-transaction →
  verify no partial writes
- Scope chaos to integration/E2E tests — unit tests should be deterministic

### Performance ratchets
- Define floors for key metrics: P95 latency, test suite duration, bundle
  size, memory usage, query count
- Store floors in a committed file (e.g., `.ratchets.json`, `ratchets.toml`)
- CI measures and compares against floor; fails on regression
- Auto-bump floor when measured value exceeds it by the defined margin
- See `.ddx/plugins/helix/workflows/ratchets.md` for the full ratchet pattern and override
  protocol

## Testing pyramid

| Layer | What | Speed | Isolation | Proportion |
|-------|------|-------|-----------|------------|
| Unit | Pure logic, schemas, value objects | < 10ms each | Full | ~70% |
| Integration | Service + real deps, API + DB | < 1s each | Partial | ~20% |
| E2E | Full user journey, browser or CLI | < 30s each | None | ~10% |
| Property | Invariants over random input | varies | Full | Alongside unit |
| Fuzz | Crash/panic discovery | campaign | Full | Periodic |
| Chaos | Failure resilience | varies | Partial | Alongside integration |

## Quality Gates

- All tests pass — no exceptions, no skips without linked issues
- Coverage ratchet: measured coverage ≥ committed floor (project-specific
  tooling)
- Performance ratchet: key metrics ≥ committed floor
- New features have tests tracing to acceptance criteria
- Property tests run for all serialization, parsing, and arithmetic modules
- No test mocks internal implementation — only system boundaries

# Concern: Testing

## Category
quality-attribute

## Areas
all

## Components

- **Philosophy**: Tests exist to find bugs in the code, not to prove it works
- **Strategy**: Multi-layer coverage with stubs over mocks, fake data over fixtures
- **Boundary testing**: Property-based, fuzz, chaos, and performance ratchets
- **Traceability**: Tests tied to acceptance criteria from governing artifacts

## Constraints

### Tests must always pass

- A failing test means the code is broken, not the test. Fix the code first.
- If a test is genuinely wrong, fix the test and document why in the commit
  message — but this should be rare. Default assumption: the test is right.
- Never skip, disable, or delete a test to make CI green. `.skip` and
  `@Ignore` are tech debt — they hide regressions.
- Never commit with failing tests. If you cannot fix the failure, leave the
  code uncommitted and report the blocker.
- Flaky tests are bugs. Investigate and fix the non-determinism (race
  conditions, time-dependence, shared state) — do not retry-loop around them.

### Tests identify issues in the code

- Tests are specifications: they define what the system should do, and they
  fail when the system does something else.
- A test that never fails is not providing value. If a test has never caught
  a bug, question whether it is testing something meaningful.
- Prioritize testing behavior (what the system does) over implementation
  (how it does it). Tests that break when you refactor internals without
  changing behavior are over-coupled.

### Tests trace to acceptance criteria

- Every acceptance criterion in a user story or feature spec should have at
  least one test that exercises it.
- Test names should read as behavior specifications: "creates an invoice
  when all line items are valid" not "test_create_invoice_3".
- Untested acceptance criteria are unverified requirements — they belong in
  the alignment review gap register.

### Stubs over mocks

- Prefer stubs (canned return values) over mocks (call-sequence assertions).
  Stubs test behavior; mocks test implementation.
- Use real implementations where practical: real databases via testcontainers
  or Docker Compose, real HTTP via local servers, real file systems via temp
  directories.
- Mock only at true system boundaries: third-party APIs, payment processors,
  email services, external auth providers.
- Never mock the thing you are testing. If you are testing a service, give it
  real (or stubbed) dependencies, not a mocked version of itself.

### Fake data over fixtures

- Generate test data with fakers (`@faker-js/faker`, `fake`, `proptest`
  strategies, `quickcheck` generators) — not static JSON fixtures.
- Fake data exposes hidden assumptions: hardcoded IDs, implicit ordering,
  length-dependent logic, locale-specific formatting.
- Seed fakers deterministically per test for reproducibility. Print the seed
  on failure so the exact data can be replayed.
- Factory functions over raw constructors: `createTestInvoice({ status: "draft" })`
  with defaults for everything not under test.

### Multi-layer coverage

- **Unit tests**: Pure functions, value objects, schema validation, business
  rules. Fast, no I/O.
- **Integration tests**: Service + real database, HTTP handler + real router,
  queue consumer + real queue. Test the contracts between components.
- **E2E tests**: Full stack from user action to database and back. Cover
  critical user journeys, not every edge case.
- **Contract tests**: API schema validation between services. Ensure
  producer and consumer agree on shape and semantics.
- Test the right things at the right layer. Do not duplicate unit-level
  assertions in E2E tests.

### Boundary testing

- **Property-based testing**: Define invariants ("output is always sorted",
  "round-trip encode/decode is identity") and let the framework generate
  hundreds of random inputs. Use `proptest`, `fast-check`, `hypothesis`, or
  `quickcheck`.
- **Fuzz testing**: Feed malformed, random, and adversarial inputs to
  parsers, validators, and serializers. Fuzz tests find crashes, panics,
  and undefined behavior that unit tests miss.
- **Chaos testing**: Inject failures (network partitions, disk full, OOM,
  slow responses, clock skew) into integration and E2E tests. Verify
  graceful degradation and recovery.
- **Performance ratchets**: Define quantitative floors for latency,
  throughput, memory, bundle size, or test duration. Commit floors to
  version control. CI fails if the metric regresses below the floor. See
  `.ddx/plugins/helix/workflows/ratchets.md` for the ratchet pattern.

## Drift Signals (anti-patterns to reject in review)

- `test.skip`, `.skip()`, `@Ignore`, `@Disabled` without a linked issue → remove or fix
- Mocking the database in integration tests → use a real database
- Mock-heavy tests that assert call order → replace with stubs and behavior assertions
- Static JSON fixture files → replace with factory functions using fakers
- Test named `test1`, `testHelper`, `it works` → rename to describe behavior
- `expect(true).toBe(true)` or tautological assertions → delete or replace with real assertion
- Commented-out tests → delete (git has the history)
- Retry loop around a flaky test → fix the flake
- No tests for a new feature → block the PR

## When to use

Every project. This concern is universal — it applies regardless of language,
framework, or domain. Compose with language-specific concerns
(`typescript-bun`, `rust-cargo`, etc.) for tooling-specific test practices.

## ADR References

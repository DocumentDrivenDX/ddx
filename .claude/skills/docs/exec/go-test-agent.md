---
ddx:
  id: EXEC-GO-TEST-AGENT
  depends_on:
    - FEAT-006
  execution:
    kind: command
    required: true
    command:
      - "go"
      - "test"
      - "./internal/agent/..."
    cwd: cli
    timeout_ms: 300000
---
# Exec: Go Test (internal/agent)

Runs the Go unit tests for `cli/internal/agent/...`, which is the package that
hosts the execute-bead, execute-loop, and gate evaluation logic governed by
FEAT-006 (Agent Service). Any execute-bead attempt targeting a bead whose
`spec-id` includes `FEAT-006` must pass this test suite before the result is
allowed to land on the default branch.

## Why this is deterministic

- The `internal/agent` package uses in-memory fakes (`fakeExecuteBeadGit`,
  `fakeAgentRunner`) and does not contact any network service, so the tests are
  reproducible in CI and in local worktrees.
- The suite is already run on every `make test` invocation and is kept green as
  a release gate per the project's testing policy.
- Runtime is well under the 5-minute default timeout that
  `evaluateRequiredGates` enforces, leaving significant headroom.

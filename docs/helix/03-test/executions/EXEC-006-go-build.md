---
ddx:
  id: exec.FEAT-006.go-build
  depends_on:
    - FEAT-006
  execution:
    kind: command
    required: true
    command:
      - go
      - build
      - ./...
    cwd: cli
    timeout_ms: 60000
---
# Go Build Gate for FEAT-006

Required build gate for the DDx Agent Service feature lane.

Verifies that the Go CLI compiles cleanly after any execute-bead iteration
that touches FEAT-006 governing artifacts. A compilation failure blocks
landing so broken code never reaches the base branch.

This gate is intentionally minimal: it only checks that the code builds.
Deeper test coverage is in [[exec.FEAT-006.execute-bead-cli-regressions]]
and the broader test plan [[TP-006-agent-session-capture]].

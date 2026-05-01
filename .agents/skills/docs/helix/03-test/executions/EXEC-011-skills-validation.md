---
ddx:
  id: exec.FEAT-011.skills-validation
  depends_on:
    - FEAT-011
    - FEAT-006
  execution:
    kind: command
    required: true
    command:
      - go
      - run
      - .
      - skills
      - check
    cwd: cli
    timeout_ms: 120000
---
# Skills Validation

Validates the shipped DDx skill packages that support execute-bead and
execute-loop guidance.

This is merge-blocking for the current docs-and-skills lane because the queue
relies on those skills to steer agent execution correctly.

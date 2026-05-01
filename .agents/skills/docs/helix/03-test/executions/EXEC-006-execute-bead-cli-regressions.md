---
ddx:
  id: exec.FEAT-006.execute-bead-cli-regressions
  depends_on:
    - FEAT-006
  execution:
    kind: command
    required: true
    command:
      - go
      - test
      - ./cmd
      - -run
      - TestExecuteBead(ResolvesPathStyleSpecID|SynthesizesPromptAndArtifacts)|TestAgentExecuteLoop
      - -count=1
    cwd: cli
    timeout_ms: 120000
---
# Execute-Bead CLI Regressions

Targeted regression coverage for the execute-bead / execute-loop lane.

This execution keeps the high-signal command small enough to run on routine
docs-and-contract work without forcing the full CLI test suite.

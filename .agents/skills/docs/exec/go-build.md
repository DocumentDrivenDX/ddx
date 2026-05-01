---
ddx:
  id: EXEC-GO-BUILD
  depends_on:
    - FEAT-001
  execution:
    kind: command
    required: true
    command:
      - "go"
      - "build"
      - "-buildvcs=false"
      - "./..."
    cwd: cli
    timeout_ms: 300000
---
# Exec: Go Build

Compiles every Go package in `cli/` to verify the DDx CLI still builds cleanly
after any change to a FEAT-001 governing bead. The command is
`go build -buildvcs=false ./...` so it is deterministic regardless of whether
the execute-bead worktree has VCS metadata stamped (the Makefile `make build`
target uses `git describe` and can fail inside nested or partial worktrees —
this gate intentionally bypasses that).

This gate is `required: true`, so any execute-bead attempt targeting a bead
whose `spec-id` includes `FEAT-001` must pass `go build` cleanly before the
result is allowed to land on the default branch.

## Why this is deterministic

- `go build ./...` only reads source files and the Go module cache; it has no
  network dependency once modules are resolved.
- `-buildvcs=false` removes the only known source of non-determinism (VCS
  stamping failures when git metadata is missing).
- Exit code is 0 if and only if every package compiles, matching the intent of
  the gate.

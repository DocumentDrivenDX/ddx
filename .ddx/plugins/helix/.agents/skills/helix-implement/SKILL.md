---
name: helix-build
description: 'Execute one bounded HELIX build pass. Public command surface: helix build.'
argument-hint: "[issue-id|scope]"
disable-model-invocation: true
---

# Build

Run one bounded managed implementation attempt for an execution-ready bead.

If a specific issue or scope is given, use: $ARGUMENTS

`helix build` is a compatibility wrapper over the DDx managed execution
surface. Its contract is:

- resolve one execution-ready bead from the current queue or selector
- delegate the bounded attempt to `ddx agent execute-bead`
- keep HELIX authority-order and bead-shaping rules in force
- avoid substituting a direct `ddx agent run` prompt or a private
  claim/execute/close loop for managed execution work

## Execution Surface

Use this surface for one explicit bead or selector:

```bash
helix build [issue-id|scope]
ddx agent execute-bead <bead-id> [--from <rev>] [--no-merge]
```

`ddx agent execute-bead` is the bounded managed execution primitive. Direct
`ddx agent run` is not the build surface for execution-ready beads.

## Read Before Executing

Load:

- `.ddx/plugins/helix/workflows/actions/implementation.md`
- `.ddx/plugins/helix/workflows/EXECUTION.md`
- the bead's governing artifacts via `spec-id`, parent, and context digest

## Readiness Rules

Before dispatching a managed build attempt:

1. Ensure the selector resolves to an execution-ready bead.
2. Ensure the bead has deterministic acceptance and success-measurement
   criteria: exact commands, named checks, files, fields, or observable end
   state that can prove success.
3. If the bead lacks that contract, do not force execution. Route it to
   `helix polish`, `helix triage`, or another planning surface first.

## Operator Guidance

- Use `helix build` when the operator wants one explicit bounded attempt.
- Use `ddx agent execute-loop` for queue drain.
- Use `helix run` only when the compatibility wrapper's supervisory routing is
  needed on top of DDx-managed execution.
- File follow-on beads for newly discovered work instead of silently expanding
  the current bead's scope.

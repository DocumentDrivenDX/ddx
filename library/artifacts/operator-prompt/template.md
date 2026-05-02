---
ddx:
  id: OP-NNN
  depends_on: []
operator_prompt:
  schema_version: 1
  issue_type: operator-prompt
  status: proposed
  priority: 2                    # 0..4 (clamped to MinPriority..MaxPriority)
  source: web-ui                 # web-ui | cli | api
  labels:
    - kind:operator-prompt
    - source:web-ui
---
# Operator Prompt: <Short Title>

## Prompt body

The verbatim instruction the operator submitted. The execute-bead harness
treats this body as the contract — there is no separate acceptance criteria
verifier for operator-prompt beads. Be specific and complete.

## Approval flow

Operator-prompt beads are created in `proposed` status and excluded from
`ddx work` / execute-loop drain until an operator approves them by
transitioning the status to `open`. Allowed transitions out of `proposed`:

- `proposed → open` — approve, queue for execution
- `proposed → cancelled` — decline, do not run

No other transitions out of `proposed` are permitted.

## Hard constraint: no self-mutation

An operator-prompt bead's execution **may not** create, edit, or close
another operator-prompt bead. The constraint is enforced at bead-create
time by `OperatorPromptMutationGuard` and cannot be bypassed by labels,
metadata, or harness configuration. If a chained workflow is required,
file the follow-up work as a regular `task` bead.

## Acceptance

The auto-AC stub for operator-prompt beads is:

> Agent must produce a diff or no_changes rationale; the prompt body is the
> contract.

Override the stub only when you want a structural AC verifier to run
(rare — the prompt body is normally the only contract).

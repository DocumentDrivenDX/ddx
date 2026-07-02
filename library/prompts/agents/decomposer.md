# Decomposer Agent Prompt

The decomposer agent splits a bead that is too large into 2-5 independently
executable child bead specs, after the escalation ladder has been exhausted.

## Input Contract

The prompt envelope is a JSON object with the following fields:

- `title` (string): the bead title (immutable — do not change)
- `description` (string): the current bead description
- `acceptance` (string): the current acceptance criteria
- `failure_history` (array of objects, newest first): the last N attempt summaries,
  each with `summary` and optional `body` fields drawn from the bead's event log

## Output Contract

Return exactly one JSON array of 2-5 child spec objects:

```json
[
  {
    "title": "imperative action naming subsystem and change",
    "description": "PROBLEM + ROOT CAUSE with file:line + PROPOSED FIX + NON-SCOPE",
    "acceptance": "numbered ACs, at least one naming a Test* function",
    "labels": ["phase:6", "area:agent", "kind:feature"]
  }
]
```

Each object must have non-empty `title`, `description`, and `acceptance`.
`labels` may be an empty array. Return only the JSON array — no surrounding
prose, no markdown fences.

## Role Guidance

You are splitting a bead that has repeatedly failed because it is too large for
a single agent to execute. Study the failure history to understand what made the
bead hard to execute, then split into 2-5 slices that together cover all the
parent's work.

### Child Bead Requirements

Each child must:

- Have a clear, imperative title naming the subsystem and change
- Contain a self-contained description (PROBLEM + ROOT CAUSE with file:line +
  PROPOSED FIX + NON-SCOPE), since the child body is the agent's entire prompt
  with no access to chat history or operator context
- Have ~3-6 numbered ACs, at least one naming a `Test*` function or a
  `go test -run` filter; the final two entries must be
  `cd cli && go test ./<paths>/... green` and `lefthook run pre-commit passes`
- Be independently executable by a sub-agent without operator hand-curation

### Ordering

If children must be completed in sequence, state explicit dependency references
in each child's description (e.g., "Requires ddx-XXXXXXXX to complete first").
If children are parallel, state "no deps" in each description.

### Preservation

Preserve verbatim in child descriptions:

- Governing artifact references (e.g., `FEAT-010`, `ADR-023`)
- Named test functions (e.g., `TestFoo`, `TestBarBaz`)
- File and line references (e.g., `cli/internal/agent/foo.go:42`)
- NON-SCOPE section bullets (scope boundaries must not be erased)

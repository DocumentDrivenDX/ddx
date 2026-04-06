---
name: helix-review
description: Run the HELIX fresh-eyes review. Use when the user wants `helix review` behavior after implementation.
argument-hint: "[scope]"
---

# Review

Execute the HELIX fresh-eyes review action.

## Steps

1. Read and apply `workflows/actions/fresh-eyes-review.md`.
2. Scope the review to `$ARGUMENTS` when provided, otherwise review the recent
   implementation work.
3. Focus on bugs, regressions, missing tests, and follow-on work.
4. Report concrete findings first. If the work is clean, say so briefly.

## Follow-Up Bead Policy

Before the review closes, **every actionable follow-up** (severity critical,
high, or medium) must be filed as a tracker bead via `ddx bead create`. Do not
close the review with prose suggestions that have no corresponding bead — the
ready queue is the only durable hand-off mechanism between review and
execution.

See the "Filing Findings as Tracker Issues" section in
`workflows/actions/fresh-eyes-review.md` for the exact bead format.

## Output

- findings first
- concrete evidence
- filed beads for actionable follow-ups (critical/high/medium)
- trailer lines: REVIEW_STATUS, ISSUES_COUNT, FINDINGS_FILED, AGENTS_MD_UPDATED, LEARNINGS_FILED

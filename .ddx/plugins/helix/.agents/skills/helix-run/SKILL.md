---
name: helix-run
description: Run HELIX autopilot. Use when the user wants `helix run` behavior or when work should keep advancing until human input is required.
argument-hint: "[scope|issue-id]"
---

# Run

Run the HELIX bounded operator loop against the built-in tracker.

This skill assumes the full HELIX package is installed. Shared resources used
by multiple HELIX skills live under `.ddx/plugins/helix/workflows/`; skill-local assets live with
the individual skill directories.

## Use This Skill When

- the user wants `helix run` behavior from inside the agent
- the repo uses `ddx bead` for execution tracking
- the user wants ready HELIX work executed until the bounded loop stops
- the user wants queue-drain decisions honored instead of a blind loop
- the user wants HELIX to keep moving across requirements, designs, issues,
  build work, and review until human judgment is required

For command-specific work, prefer the mirrored companion skills such as
`helix-build`, `helix-check`, `helix-align`, and `helix-backfill`.

## HELIX Phases

`FRAME -> DESIGN -> TEST -> BUILD -> DEPLOY -> ITERATE`

- `Frame`: define the problem, users, requirements, and acceptance criteria
- `Design`: define architecture, contracts, and technical approach
- `Test`: write failing tests that specify behavior
- `Build`: implement the minimum code to make tests pass
- `Deploy`: release with monitoring and rollback readiness
- `Iterate`: learn from production and plan the next cycle

## Authority Order

When artifacts disagree, prefer:

1. Product Vision
2. Product Requirements
3. Feature Specs / User Stories
4. Architecture / ADRs
5. Solution / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code / Build Artifacts

Tests govern build execution, but they do not override upstream requirements or
design.

## Execution Layer

HELIX uses the built-in tracker (`ddx bead`) for execution tracking.
Issues are stored in `.ddx/beads.jsonl`.

- Use `ddx bead` issues, dependencies, parents, `spec-id`, and labels.
- Do not invent custom issue files or custom status taxonomies.
- Recommended labels: `helix`, plus phase/kind/traceability labels as needed.
- See `ddx bead --help` for tracker command mapping.

Reference docs (read as needed):

- `.ddx/plugins/helix/workflows/README.md`
- `.ddx/plugins/helix/workflows/actions/check.md` when the user wants queue health or the next action
- `.ddx/plugins/helix/workflows/actions/implementation.md` when the user wants ready build work executed
- relevant phase README and artifact prompts/templates

Shared HELIX resources resolve from `.ddx/plugins/helix/workflows/`. If those resources are
missing, stop and report an incomplete HELIX package instead of improvising.

## Background Mode

For long-running work, use the `helix-worker` skill instead. It launches
`helix run` as a background CLI process with `--summary` mode and monitors
progress via log files. This skill (`helix-run`) executes inline — use it
when you need live adjustments between cycles or for short runs.

## On Invocation

When this skill is invoked inline, **execute work immediately** — do not just
report status, do not just describe what you would do, do not ask for
confirmation. Start doing real work right now.

### Step 1 — Find ready issues

Run this command:

```bash
ddx bead ready --json
```

If no ready issues exist, skip to Step 6 (Queue Drain).

### Step 2 — Select and claim one issue

Pick the best ready issue (smallest unblocked issue with clear governing
artifacts). Inspect it:

```bash
ddx bead show <id>
ddx bead dep tree <id>
```

Then claim it:

```bash
ddx bead update <id> --claim
```

### Step 3 — Load context and build

1. Read the issue's `spec-id`, parent, labels, and acceptance criteria.
2. Read the governing artifacts (requirements, design, tests) referenced by
   the issue.
3. Read `.ddx/plugins/helix/workflows/actions/implementation.md` for full phase-specific
   rules (build, deploy, iterate).
4. Build the work: write code, update docs, create follow-on issues for
   any out-of-scope work discovered.

### Step 4 — Verify

Run all project verification: tests, lint, type checks, format checks. If
verification fails, fix within scope or leave the issue open with a status note.

### Step 5 — Commit and close

1. Commit with the issue ID in the message.
2. Close the issue: `ddx bead close <id>`
3. Go back to Step 1 for the next ready issue.

### Step 6 — Queue drain

When no ready issues remain, read and execute
`.ddx/plugins/helix/workflows/actions/check.md` to decide what happens next. That action
produces a `NEXT_ACTION` code:

- `BUILD` → go to Step 1
- `DESIGN` → run the design action once, then re-evaluate the queue
- `ALIGN` → read and execute `.ddx/plugins/helix/workflows/actions/reconcile-alignment.md`
- `BACKFILL` → read and execute `.ddx/plugins/helix/workflows/actions/backfill-helix-docs.md`
- `WAIT` / `GUIDANCE` → report what is blocking and stop
- `STOP` → report that no actionable work remains

### Scope narrowing

If the user provides a scope or selector (e.g., an issue ID, feature name, or
phase), narrow all steps to that scope.

## How To Work

1. Identify the current phase from the docs and tests.
2. Do the minimum correct work for that phase.
3. Preserve traceability to upstream artifacts.
4. Keep Build subordinate to failing tests.
5. If build work reveals design drift, refine upstream artifacts explicitly.

## Core Questions

- `Frame`: what problem are we solving, for whom, and how will we know it works?
- `Design`: what structure, contracts, and constraints satisfy the requirement?
- `Test`: what failing tests prove the behavior?
- `Build`: what is the minimum implementation to make those tests pass?
- `Deploy`: how do we release safely and observe health?
- `Iterate`: what did we learn, and what follow-up work belongs in the tracker?

## Notes

- Use TDD strictly: Red -> Green -> Refactor.
- Security belongs in every phase.
- Escalate contradictions instead of patching around them in code.
- For repo-wide reconciliation or traceability work, use the alignment review flow.
- For repo-wide documentation reconstruction, use the backfill flow rather than inventing requirements from code alone.
- When the ready queue drains, use the check flow before deciding to align, backfill, wait, or stop.

### Test Phase Artifacts
- Test Plan
- Test Suites
- Security Tests

### Build Phase Artifacts
- Implementation Plan

### Deploy Phase Artifacts
- Project-specific checklists and runbooks as needed

### Iterate Phase Artifacts
- Alignment Reviews
- Backfill Reports
- Metric Definitions

## When to Use HELIX

**Good fit**:
- New products or features requiring high quality
- Mission-critical systems where bugs are expensive
- Teams practicing or adopting TDD
- AI-assisted development needing structure
- Security-sensitive applications

**Not ideal for**:
- Quick prototypes or POCs
- Simple scripts with minimal complexity
- Emergency fixes needing immediate deployment

Always enforce the test-first approach: specifications drive implementation, quality is built in from the start.

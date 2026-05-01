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

Managed execution belongs to DDx:

- `ddx agent execute-loop` is the primary queue-drain surface for
  execution-ready work.
- `ddx agent execute-bead <bead-id> [--from <rev>] [--no-merge]` is the
  bounded single-bead managed execution primitive.
- `helix run` is a compatibility controller over DDx-managed execution plus
  HELIX supervisory routing (`check`, `design`, `polish`, review, alignment).
- Direct `ddx agent run` remains appropriate only for non-managed prompts such
  as planning, review, alignment, and other work that should not auto-claim or
  auto-close execution beads.

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

### Step 1 — Load the execution contract

- Read `.ddx/plugins/helix/workflows/EXECUTION.md`.
- Read `.ddx/plugins/helix/workflows/actions/check.md` for queue routing.
- Read `.ddx/plugins/helix/workflows/actions/implementation.md` for bounded
  execution rules.

### Step 2 — Inspect queue state

Run:

```bash
ddx bead ready --json
```

If execution-ready work exists, prefer the DDx-managed queue-drain path over a
hand-rolled claim/execute/close loop.

### Step 3 — Use the right execution surface

- If the user wants the actual compatibility controller, invoke `helix run`
  rather than re-implementing it bead-by-bead inside the skill.
- If the user wants a single explicit bead run, use `helix build` or
  `ddx agent execute-bead`, not a direct `ddx agent run` prompt.
- If the work is planning, review, alignment, or another non-managed prompt,
  use the corresponding HELIX skill or direct `ddx agent run` flow instead of
  the managed execution lane.

### Step 4 — Queue drain and routing

When no execution-ready work remains, read and execute
`.ddx/plugins/helix/workflows/actions/check.md` to decide what happens next. That action
produces a `NEXT_ACTION` code:

- `BUILD` → go to Step 1
- `DESIGN` → run the design action once, then re-evaluate the queue
- `ALIGN` → read and execute `.ddx/plugins/helix/workflows/actions/reconcile-alignment.md`
- `BACKFILL` → read and execute `.ddx/plugins/helix/workflows/actions/backfill-helix-docs.md`
- `WAIT` / `GUIDANCE` → report what is blocking and stop
- `STOP` → report that no actionable work remains

### Step 5 — Enforce execution-ready bead quality

Do not treat a build/deploy/iterate bead as queue-ready unless its acceptance
criteria are machine-auditable enough for DDx-managed execution to decide
success from the bead contract itself. If the bead still says "works",
"complete", or "aligned" without naming commands, checks, files, or observable
end state, route it to `helix polish` or `helix triage` instead of executing it.

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

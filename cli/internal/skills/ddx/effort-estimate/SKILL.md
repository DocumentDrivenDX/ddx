---
name: effort-estimate
description: Estimate effort for a bead, spec, or task by dispatching to one or more harnesses and aggregating structured complexity, time, and risk findings. Use before scheduling work, breaking into beads, or prioritizing a backlog.
---

# Effort Estimate

A workflow skill for estimating the effort required to complete a bead,
spec, or free-form task description. Dispatches a structured prompt to
one or more harnesses via `ddx run` (or `ddx run`) and aggregates
complexity, time, and risk assessments into a single output.

## When to use

- **Before bead breakdown** — estimate a feature spec before filing
  child beads so the breakdown scope is calibrated.
- **Backlog prioritization** — compare effort across candidate beads
  before pulling the next one into the ready queue.
- **Scheduling** — turn a bead into a time estimate before committing
  to a sprint or milestone.
- **Risk triage** — surface high-risk or under-specified work before
  assigning it to a model or harness with limited context budget.

Single-harness estimates are fine for routine beads. Use two harnesses
when estimates diverge or when the work is novel / high-stakes.

## Workflow

### 1. Prepare the estimation target

Gather the content into a prompt file. Include:

- The bead description and acceptance criteria (paste directly or
  reference the bead ID so the agent can read it), OR the spec excerpt
  or free-form task description.
- Any governing artifacts (FEAT-\* IDs, ADR links, CONTRACT-\* IDs)
  that constrain the implementation.
- Known constraints: stack, team size, deadline, blocked dependencies.

Write the prompt to a file, e.g. `estimate-target.md`:

```markdown
## Target

<paste bead description + AC, or spec excerpt, or task description>

## Governing artifacts

- FEAT-010: docs/helix/01-frame/features/FEAT-010-task-execution.md
- (add others as relevant)

## Known constraints

- Stack: Go + SvelteKit
- No external service calls
- Must not break existing CLI surface

## Estimation question

Produce a structured effort estimate. Include complexity rating,
time-to-complete range, key risks, and a task breakdown if the
work exceeds one session.
```

### 2. Dispatch to a harness

For a single estimate:

```bash
ddx run --harness claude --min-power 10 \
  --prompt estimate-target.md \
  > estimate.md
```

For a two-harness cross-check (recommended for specs > ~1 day):

```bash
ddx run --harness codex  --min-power 10 --prompt estimate-target.md > estimate-codex.md  &
ddx run --harness claude --min-power 10 --prompt estimate-target.md > estimate-claude.md &
wait
```

### 3. Require structured output

Include this output contract in your prompt so findings are
aggregateable:

```markdown
## Output contract

Produce an effort estimate in this format:

### Complexity

| Dimension | Rating | Notes |
|---|---|---|
| Scope | XS / S / M / L / XL | <what drives it> |
| Novelty | Low / Medium / High | <new patterns, unknowns> |
| Risk | Low / Medium / High | <failure modes, blockers> |
| Reversibility | Easy / Hard | <how bad is a wrong turn> |

### Time estimate

- **Optimistic:** <N hours / days> (everything goes right)
- **Likely:** <N hours / days> (normal friction)
- **Pessimistic:** <N hours / days> (blockers surface)

### Assumptions

List every assumption baked into the estimate. An assumption that
turns out wrong is likely to shift the estimate by at least one band.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| <risk> | Low / Medium / High | Low / Medium / High | <mitigation> |

### Task breakdown

Break the work into steps if the likely estimate exceeds 4 hours.
For each step: name, likely time, dependencies.

| Step | Likely time | Depends on |
|---|---|---|
| <step> | <time> | <step or none> |

### Verdict: WELL_SCOPED | NEEDS_BREAKDOWN | UNDER_SPECIFIED
```

A `NEEDS_BREAKDOWN` verdict means the work should be filed as an epic
with child beads before execution. An `UNDER_SPECIFIED` verdict means
governing artifacts or constraints are missing and the estimate is
unreliable — resolve gaps before scheduling.

### 4. Aggregate across harnesses

When running two harnesses:

1. **Time estimates within one band** → use the higher bound as the
   schedule anchor.
2. **Time estimates diverging by more than one band** → surface both;
   do not average. Divergence signals under-specification or a novelty
   gap; treat as a WARNING.
3. **Risk ratings differing** → use the higher risk rating; record
   both rationales.
4. **Verdicts differ** (e.g., one says `WELL_SCOPED`, other says
   `NEEDS_BREAKDOWN`) → treat the more conservative verdict as
   authoritative. Escalate to the user.

```bash
# Combine for review
cat estimate-codex.md estimate-claude.md > estimate-combined.md
```

### 5. Record the estimate

Attach the combined estimate to the bead as evidence:

```bash
ddx bead evidence add <id> --type estimate --body estimate-combined.md
```

Or tag the bead with the complexity rating for queue sorting:

```bash
ddx bead update <id> --tag complexity:M
ddx bead update <id> --tag risk:high
```

## Estimation framing

Include this framing instruction in every estimation prompt to get
calibrated rather than optimistic output:

> You are a senior engineer estimating work for a teammate. Err on the
> side of the pessimistic bound — underestimates cause missed
> commitments; overestimates only cause pleasant surprises. If you
> cannot estimate a step because the governing artifact is ambiguous,
> say so explicitly rather than guessing. A `NEEDS_BREAKDOWN` verdict
> is not a failure — it is useful signal.

## Anti-patterns

- **Estimating without governing artifacts.** An estimate without
  constraints is a guess. Always include the relevant FEAT-\* or spec
  in the prompt.
- **Averaging divergent estimates.** Divergence is signal, not noise.
  Show both and surface the disagreement.
- **Scheduling `UNDER_SPECIFIED` work.** Resolve spec gaps before
  committing to a timeline. An unreliable estimate is worse than no
  estimate.
- **Skipping breakdown on L/XL beads.** Work estimated at > 1 day
  should produce child beads before entering the execute queue.
- **Single-harness estimates on novel work.** Novel work (new patterns,
  unknown APIs, first-time integrations) benefits from cross-model
  coverage because unfamiliarity bias varies by training data.

## CLI reference

```bash
# Single-harness estimate
ddx run --harness claude --min-power 10 \
  --prompt estimate-target.md > estimate.md

# Two-harness cross-check (parallel)
ddx run --harness codex  --min-power 10 --prompt estimate-target.md > estimate-codex.md  &
ddx run --harness claude --min-power 10 --prompt estimate-target.md > estimate-claude.md &
wait

# Store estimate as bead evidence
ddx bead evidence add <id> --type estimate --body estimate-combined.md

# Tag bead with complexity
ddx bead update <id> --tag complexity:M
ddx bead update <id> --tag risk:high

# List beads by complexity tag (for prioritization)
ddx bead list --tag complexity:L
ddx bead list --tag complexity:XL
```

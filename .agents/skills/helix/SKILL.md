---
name: helix
description: Route HELIX methodology work to the right planning, alignment, design, review, execution, or release workflow. Use when the user asks to use HELIX, work with HELIX artifacts, align documents, frame requirements, design a change, evolve specs, review work, decide what is next, or manage HELIX-governed work without naming a specific helix-* skill.
argument-hint: "[intent or scope]"
---

# HELIX Router

Use this as the HELIX entrypoint. Users should not need to memorize individual
workflow skill names. Route the request to the smallest HELIX workflow that
fits, then follow the matching workflow contract below.

Rule: do not add separate public `helix-*` skills. Add or refine a route inside
this skill instead.

## Routing Rules

Prefer the first matching route:

| User intent | Mode |
|---|---|
| Convert rough intent into governed HELIX work | input |
| Create or refine product vision, PRD, feature specs, or user stories | frame |
| Reconcile artifacts, check traceability, find drift, align documents, or move content between artifact layers | align |
| Thread a new, changed, removed, or incident-driven requirement through existing artifacts | evolve |
| Create a technical design before implementation | design |
| Reconstruct missing or incomplete docs from evidence | backfill |
| Fresh-eyes review of recent work, PRs, plans, or implementation | review |
| Refine beads/work items for execution readiness | polish |
| Decide the next safe HELIX action | check or next |
| Execute one bounded implementation pass | build |
| Run the bounded operator loop | run |
| Commit verified HELIX/DDx work | commit |
| Cut a release | release |
| Run an optimization experiment | experiment |
| Monitor a background HELIX run | worker |

When multiple routes fit, choose the highest-authority planning route first:
`frame` before `design`, `align` before `evolve` when the task is diagnostic,
and `evolve` before `build` when a requested implementation lacks governing
artifact coverage.

## Workflow Contracts

### Input

Use for sparse user intent that needs to become governed HELIX work.

1. Clarify scope only when missing information would make the resulting work
   unsafe or unactionable.
2. Identify governing artifacts that already exist.
3. Produce or update planning work items rather than implementation work when
   authority is missing.
4. Keep created work standalone: include context, acceptance criteria, labels,
   parent/dependency relationships, and verification commands.

### Frame

Use for creating or refining product vision, PRD, feature specs, and user
stories.

1. Read existing Frame artifacts first.
2. Read the relevant artifact template and prompt before drafting.
3. Keep each artifact in its lane: vision is direction, PRD is product scope,
   feature specs are feature behavior, stories are vertical user outcomes.
4. Validate blocking template checks before treating the artifact as ready.
5. Create follow-up design or implementation work only after the framing
   artifact can govern it.

### Align

Use for reconciliation, traceability audits, drift checks, and artifact content
placement reviews.

1. Start from authority: vision, PRD, features/stories, architecture/ADRs,
   designs, tests, implementation plans, code.
2. Reconstruct intent from planning artifacts before inspecting lower layers.
3. Classify each gap as `ALIGNED`, `INCOMPLETE`, `DIVERGENT`,
   `UNDERSPECIFIED`, `STALE_PLAN`, or `BLOCKED`.
4. Produce one durable alignment report when the action is more than a
   conversational review.
5. Create or identify follow-up work for every non-aligned gap.

### Evolve

Use when the user wants to add, remove, amend, or thread a requirement through
the HELIX artifact stack.

1. Analyze the requested change and affected product areas.
2. Discover governing artifacts that need updates.
3. Detect conflicts with existing artifacts and open work.
4. Apply updates in authority order: vision, PRD, feature specs/stories,
   designs, decisions, test plans, implementation plans.
5. Create follow-up work with dependencies where ordering matters.

### Design

Use when implementation needs design authority before build work.

1. Load governing artifacts, existing designs, implementation context, tests,
   and open work for the scope.
2. Draft problem statement, requirements, architecture decisions, interfaces,
   data model, errors, security, testing, sequencing, risks, and observability.
3. Iterate through self-critique until material changes converge.
4. Write the design to the project HELIX design location.
5. Derive ordered, verifiable implementation work from the design.

### Backfill

Use to reconstruct missing or incomplete HELIX artifacts from evidence.

1. Read available evidence: artifacts, implementation, tests, releases, and
   recorded decisions.
2. Separate confirmed facts from inference.
3. Reconstruct only what the evidence supports.
4. Mark uncertainty explicitly.
5. Create follow-up work for unresolved authority gaps.

### Review

Use for fresh-eyes review of plans, PRs, implementation, or recent work.

1. Scope the review narrowly.
2. Inspect governing artifacts, changed implementation, tests, and public
   projection relevant to the scope.
3. Report findings first, ordered by severity, with concrete evidence.
4. File durable follow-up work for actionable medium-or-higher findings when
   the project uses DDx/HELIX tracking.

### Polish

Use to refine work items before execution.

1. Load open work for the scope and any governing plan.
2. Run multiple passes for deduplication, coverage, acceptance quality,
   dependency correctness, sizing, and label hygiene.
3. Require execution-ready beads to name exact files, commands, checks, fields,
   or observable repository states.
4. If acceptance cannot be sharpened from governing artifacts, flag the work as
   not execution-ready and route it back through planning.

### Check And Next

Use when the safe next action is ambiguous.

1. Inspect the queue, governing artifacts, and known blockers.
2. Decide conservatively among build, design, alignment, backfill, polish, wait,
   guidance, or stop.
3. Do not dispatch another workflow silently.
4. If missing tracked work is discovered, create or recommend explicit work
   before returning the next action.

### Build And Run

Use only when the user explicitly asks for HELIX execution.

1. Build handles one bounded implementation pass for a selected work item.
2. Run handles the bounded operator loop over ready work.
3. Stay within the governing bead/work item.
4. Do not broaden scope beyond the named work.
5. Verify with the project gate before reporting completion.

### Commit

Use when verified work should be committed.

1. Inspect the diff and separate unrelated user changes.
2. Run the project gate.
3. Commit only the intended scope with traceable message text.
4. Preserve DDx execute-bead history: never squash, rebase, amend, or filter
   branches containing execute-bead or `[ddx-*]` commits.

### Release

Use for cutting a HELIX plugin release.

1. Confirm release scope and version.
2. Run required validation and site build.
3. Tag, push, and publish according to project release rules.
4. Report artifacts, tag, and verification results.

### Experiment

Use for metric-driven optimization loops.

1. Define the goal, metric, baseline, intervention, and stop condition.
2. Run bounded iterations.
3. Measure after each iteration.
4. Keep changes or revert/adjust based on metric evidence.

### Worker

Use to launch and monitor a background HELIX operator loop.

1. Start the run with durable logs and pid capture.
2. Poll sparingly for progress, blockers, or completion.
3. Report status without losing the run evidence.
4. Stop only when requested or when the workflow reaches a safe stopping point.

## Alignment Content Migration

If a user asks whether content belongs in the right HELIX document, use align
mode. The alignment output must include a content migration ledger for every
misplaced content unit:

| Field | Required content |
|---|---|
| Source | Artifact path and line references |
| Content unit | Small named chunk of content |
| Classification | `keep`, `move`, `split`, `delete`, `needs-new-artifact`, or `decision-needed` |
| Destination | Exact destination artifact path or artifact type |
| Content to add | Destination-shaped draft content |
| Template fit | Destination section and blocking/warning checks |
| Destination risks | Any template check the proposed addition would fail |
| Follow-up | Tracker issue ID or explicit issue to create |

Do not remove content from one artifact unless the destination content and
follow-up work are captured durably.

## Operating Discipline

- Use the workflow contracts in this skill as the active interface; consult
  packaged workflow prompts only when deeper mode-specific detail is needed.
- For DDx-backed projects, obey bead-first rules before writing files or tracker
  mutations.
- Do not silently start implementation when the request is planning, alignment,
  review, or routing.
- If the correct route is unclear, use check mode rather than guessing.
- Preserve HELIX authority order: vision, PRD, features/stories, architecture
  and ADRs, designs, tests, implementation plans, code.

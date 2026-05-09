# Interactive Steward — Default Queue-Steward Conversation Workflow

This file governs broad conversational DDx sessions: queue orientation, planning,
review, guidance folding, spec alignment, and bead breakdown. It is the first
stop for any DDx request that is not an explicit worker command, direct code
edit, or explicit review-only request.

## Mode

**Mode: `interactive-steward`**

The interactive steward combines orientation, planning, review, alignment, and
breakdown into a coherent conversation loop. It does **not** implement ready
beads directly — that is `bead_execution` mode (`ddx work` / `ddx try`).

When a human says "what should I work on next", "what's blocking the queue",
"review with fresh eyes", "fold in this guidance", "break this down into specs
and beads", or "make this testable", they are talking to the steward, not the
worker.

## Router precedence

1. **Explicit worker commands** — `ddx work`, `ddx try <id>`, "execute bead
   `<id>`", "start the worker" → `bead_execution` → `reference/work.md`. Stop;
   do not enter steward mode.
2. **Broad conversational DDx prompts** — queue orientation, planning, review,
   guidance folding, spec alignment, breakdown → **`interactive-steward`** → this
   file, then delegate to the matching domain reference.
3. **Explicit code/doc edits** — "fix this bug", "update this file" →
   `direct_user_implementation`.
4. **Explicit review-only requests** — "review this", "check against spec" →
   `review` → `reference/review.md`.

The steward delegates to domain references rather than duplicating their guidance:

- Queue/health queries → `reference/status.md`
- Review → `reference/review.md`
- Bead writing/breakdown → `reference/beads.md`
- Queue execution → `reference/work.md`

## Session brief

At the start of an interactive session, the steward collects a `SessionBrief`:

- **Focus**: the active bead ID or goal, if stated.
- **Horizon**: how much work the user wants to get through this session.
- **Constraints**: blockers, frozen areas, external dependencies.
- **Mode preference**: plan, review, align specs, or break down work.

The `SessionBrief` is kept in-conversation and referenced for all follow-on
actions. **Never write the brief to `/tmp` or a path outside the repository** —
if it needs persistence, embed it into a bead description or spec update.

## Phase outputs

Each steward phase produces a named output contract. Bead/spec outputs inline
the accepted session brief; they do not rely on chat history.

### orient → QueueFacts

Probes queue state. First try:

```bash
ddx work focus --help
```

If the command is available, run:

```bash
ddx work focus
```

If `ddx work focus` is unavailable (exits non-zero for `--help`), fall back to:

```bash
ddx bead status         # lifecycle and derived queue counts
ddx bead blocked        # status=blocked external/recheckable blockers
ddx bead ready          # actionable execution-ready open beads
ddx bead show <id>      # targeted inspection of the top ready bead
```

If `ddx work focus` exists but fails, report that failure — do not silently fall
back.

`QueueFacts` includes: total open, execution-ready bead IDs, dependency-waiting
beads, proposed/operator-attention beads, external blockers, and any error from
the focus command.

### plan → SessionBrief

Given `QueueFacts`, produce a `SessionBrief` for this session:

- Which beads to attempt (ordered by priority + dependency satisfaction).
- Which actions are consent-gated (adversarial review, tracker mutations).
- What success looks like for the session.

### fresh-eyes review → Findings + Verdict

Structured local review of current branch or work-in-progress against bead ACs
or governing spec.

Returns:

- `Findings`: per-AC or per-concern items with file:line evidence.
- `Verdict`: APPROVE | REQUEST_CHANGES | BLOCK.

**Multi-harness adversarial review is consent-gated.** Suggest it if the change
is high-stakes, but do not dispatch it unless the user explicitly agrees. Phrase
the suggestion as:

> "Would you like me to run an adversarial multi-harness review? This dispatches
> `ddx run` across multiple personas and costs more time."

### fold guidance → Accepted + Rejected + Unresolved + revised plan

The user provides guidance (feedback, spec change, constraint) to fold into the
current plan:

1. Parse each guidance item.
2. Accept items that align with the session brief and spec.
3. Reject items that conflict with governing artifacts (state the conflict).
4. Mark unresolved items that need a spec update or human decision.
5. Produce a revised `SessionBrief` incorporating the accepted guidance.

### align specs → SpecDeltas

Compare current implementation or bead AC against the governing spec (`FEAT-*`,
`ADR-*`, `SD-*`):

1. Read the governing artifact (follow `spec-id` if present on the bead).
2. Identify divergences between spec requirements and current AC or code.
3. Return `SpecDeltas`: which spec sections are satisfied, which are diverged,
   and what changes are needed to align.

If no `spec-id` is present, ask the user which governing artifact to align
against before proceeding.

### breakdown → filed bead IDs + dependency edges + named tests + verification commands

Break an epic or broad goal into execution-ready beads:

1. Produce a bead breakdown with title, description, and AC for each slice.
2. Each AC names at least one test function or `go test -run` filter.
3. File each bead via `ddx bead create`.
4. Wire dependency edges via `ddx bead dep add`.
5. Return: filed bead IDs, parent/dependency graph, named tests per bead, and
   verification commands.

Durable bead outputs inline the accepted `SessionBrief` and do not reference
`/tmp` files or chat history.

## Mutation policy

| Phase | Tracker mutations | Code edits |
|---|---|---|
| orient | no | no |
| plan | no | no |
| fresh-eyes review | no | no |
| fold guidance | no (unless breakdown follows) | no |
| align specs | no | no |
| breakdown | yes (`ddx bead create` / `ddx bead dep add`) | no |

The steward **never** edits code directly. It plans, reviews, aligns, and breaks
down. Actual code changes happen in `bead_execution` mode via `ddx work` /
`ddx try`.

## Queue orientation fallback

If `ddx work focus` is not yet available, use the three-command fallback:

```bash
ddx bead status
ddx bead blocked
ddx bead ready
```

Report the three outputs as `QueueFacts`. Note that `ddx work focus` is not yet
available so the user knows the fallback was used.

## Spec alignment

To align specs, read the governing artifact referenced by `spec-id` on the
current bead or task context. If no `spec-id` is present, ask the user which
governing artifact to align against before proceeding.

## Breakdown

A well-formed breakdown satisfies the 8-criterion bead-authoring rubric (see
`docs/helix/06-iterate/bead-authoring-template.md`). Each child bead must be
self-contained: a competent agent given only the bead body must be able to pick
a file to edit and run tests without asking.

Breakdown outputs include:

- Filed bead IDs (from `ddx bead create` output).
- Parent/dependency edges (from `ddx bead dep add` output).
- Named test functions or `go test -run` filters per bead.
- Verification commands per bead.

# Review — Bead Review and Quorum Review

## Mode note

This reference covers **explicit `review` mode**: requests that name review as
the primary action — "review this", "check against spec", `ddx bead review <id>`.

If the user said "review with fresh eyes", "fold in this guidance and review
again", or any broad orientation phrase, they are in **`interactive-steward`**
mode — see `reference/interactive.md` for the fresh-eyes review and fold-guidance
phases. Multi-harness adversarial review is consent-gated in steward mode.

Review mode is **read-only** by default. Code edits only happen if the user
explicitly requests fixes after seeing the findings.

---

DDx has two distinct review concepts. They are **not interchangeable**.

| Concept | Command | What it does |
|---|---|---|
| **Bead review** | `ddx bead review <id>` | Grades a bead's implementation against its declared acceptance criteria. Per-AC verdict with evidence. |
| **Comparison/adversarial review** | workflow skill composition over `ddx run` | Dispatches review prompts across multiple roles or configured agent routes and aggregates the verdicts. Used for high-stakes or adversarial checks. |

## Bead review

```bash
ddx bead review <bead-id>
```

Generates a review-ready prompt that includes:

- Bead title, description, acceptance criteria (verbatim).
- Governing artifact content (follows `spec-id`).
- The diff since the bead's base commit (execution evidence commits
  excluded — see the related bug ddx-39e27896 for exclusion
  pathspec).
- Review instructions: output contract, verdict taxonomy.

The reviewer (whoever receives the prompt — a human, `ddx run`, or a workflow
skill composition) produces a structured verdict:

```
## Bead review: ddx-<id>

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### Per-criterion

1. [AC text]
   Verdict: APPROVE | REQUEST_CHANGES | BLOCK
   Evidence: <file:line, test output, diff hunk, or explicit "not verifiable">

2. [AC text]
   ...

### Summary
<1-3 sentences of overall context>

### Findings
<detailed issues, if any; omit when verdict is APPROVE with no findings>
```

The review output can be stored as bead evidence:

```bash
ddx bead evidence add <id> --type review --body <path-to-review.md>
```

## Comparison/adversarial review

```bash
ddx run --persona code-reviewer --prompt review-prompt.md > review-code.md
ddx run --persona architect --prompt review-prompt.md > review-arch.md
```

Aggregation policies used by workflow skills:

- `majority` — at least ⌈N/2⌉ harnesses must agree on the verdict.
- `unanimous` — all harnesses must agree.
- `any` — pass if any harness APPROVES (rarely useful; adversarial
  checks should require stronger consensus).

Use comparison/adversarial review when:

- The change is high-stakes (migrations, security surface, API
  contracts, auth).
- The review surface is controversial and one harness's opinion
  isn't enough.
- You want adversarial pressure against a single-model blindspot
  (different model families have different failure modes).

The aggregated output shows each harness's verdict side-by-side plus
the aggregate decision. Disagreements are surfaced, not hidden.

## Required evidence format

Both review kinds share an evidence contract: **no verdict without
evidence.**

- "APPROVE" with no `Evidence:` lines → reject as malformed.
- "REQUEST_CHANGES" without pointing at the change needed → reject.
- "BLOCK" without a reason → reject (see the related bug
  ddx-39e27896 about `review-malfunction` retries).

Evidence takes one of these forms:

1. **File:line reference**: `cli/internal/bead/store.go:142`
2. **Test output**: `FAIL: TestBeadClose_UnclaimsOnNonSuccess (0.01s)`
3. **Diff hunk**: a short quote from the diff showing the offending
   or approved change.
4. **Explicit "not verifiable"**: the AC is written in a way that
   cannot be mechanically checked; flag this as a bead-authoring
   issue rather than silently approving.

## Anti-patterns

- **Hallucinated approval.** "Looks good to me!" with no per-AC
  grading. The review is worthless.
- **Reviewing the prompt instead of the diff.** The reviewer evaluated
  "what you intended" not "what you actually changed." Verdict must
  reference the diff or the resulting code.
- **Verdict without evidence.** Every verdict must have at least one
  evidence line; APPROVE is no exception (evidence can be "AC met:
  `go test ./foo/... passes` output attached").
- **Silencing disagreement in comparison review.** If two reviewers disagree,
  don't collapse to a single "decision." Surface the disagreement
  in the output and let the human decide.
- **Comparison review for routine work.** Single-reviewer bead review is fine
  for most beads. Reserve multi-reviewer comparison for the cases that justify the
  cost.

## Subagents: not this skill's business

Some harnesses can run a comparison in forked/isolated subagent
contexts; each harness implements this differently. This skill
doesn't specify how isolation is achieved —
`ddx run` is the portable invocation, and the harness decides whether to fork,
spawn, or run inline.

## CLI reference

```bash
# Bead review (single-reviewer AC grading)
ddx bead review <id>                           # generate review prompt
ddx bead review <id> --execute --harness claude  # dispatch and grade
ddx bead evidence add <id> --type review --body r.md  # store review

# Comparison review (workflow composition)
ddx run --persona code-reviewer --prompt review.md > review-code.md
ddx run --persona architect --prompt review.md > review-arch.md
```

Full flag list: `ddx bead review --help`, `ddx run --help`.

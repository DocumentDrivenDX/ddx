---
name: adversarial-review
description: Run an adversarial review of a plan, spec, bead, or PR by dispatching to multiple harnesses with adversarial framing and aggregating findings. Use when you want pressure-testing against a single-model blindspot, high-stakes review, or multi-round critique before committing to a direction.
---

# Adversarial Review

A workflow skill for pressure-testing a target (plan, spec, bead, code change,
or PR) by dispatching adversarial review prompts to multiple harnesses and
aggregating the findings. Replaces ad-hoc inline quorum invocations with a
structured, repeatable workflow.

## When to use

- **Plans and specs** before breaking into beads — catch ambiguity and missing
  constraints early.
- **High-stakes beads** — migrations, auth, API contracts, security surface.
- **Controversial changes** — one model's opinion is not enough; different model
  families catch different failure modes.
- **Multi-round refinement** — run 2–3 rounds; use findings to revise; re-run
  until BLOCKING count drops to zero.

Single-harness bead review (`ddx bead review <id>`) is fine for routine work.
Reserve adversarial-review for the cases that justify the cost.

## Workflow

### 1. Prepare the target

Gather the content to review into a single prompt file. Include:

- The artifact being reviewed (plan text, spec section, bead description + AC,
  or diff / PR summary).
- The question you want pressure-tested ("Is this design complete?", "What are
  the failure modes?", "What does this spec leave undefined?").
- Any governing artifacts the reviewer should check against (FEAT-\* IDs,
  CONTRACT-\* IDs, ADR links).

Write the prompt to a file, e.g. `review-target.md`:

```markdown
## Target

<paste plan, spec excerpt, or diff>

## Governing artifacts

- FEAT-010: docs/helix/01-frame/features/FEAT-010-task-execution.md
- (add others)

## Review question

Find BLOCKING issues: ambiguities, missing constraints, design gaps,
internal contradictions, or anything that would cause implementation
rework. Be adversarial — your job is to find problems, not validate.
```

### 2. Dispatch to multiple harnesses

Run each harness in parallel using `ddx run --harness <name>`.
Use `--min-power 10` (or `--harness` for explicit control). Route to
harnesses from different model families for maximum coverage.

```bash
# Two-harness adversarial dispatch (parallel)
ddx run --harness codex --min-power 10 \
  --prompt review-target.md \
  > findings-codex.md &

ddx run --harness claude --min-power 10 \
  --prompt review-target.md \
  > findings-claude.md &

wait
```

For three harnesses:

```bash
ddx run --harness codex  --min-power 10 --prompt review-target.md > findings-codex.md  &
ddx run --harness claude --min-power 10 --prompt review-target.md > findings-claude.md &
ddx run --harness gemini --min-power 10 --prompt review-target.md > findings-gemini.md &
wait
```

### 3. Require structured output

Each harness must produce findings in this format. Include this contract
in the prompt:

```markdown
## Output contract

Produce findings as:

### Findings

| Severity | Area | Finding |
|---|---|---|
| BLOCKING | <area> | <specific issue> |
| WARNING  | <area> | <specific issue> |
| NOTE     | <area> | <observation> |

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### Summary
<2–4 sentences>
```

A finding with no evidence is invalid. Cite the specific line, section,
or gap that caused the finding. "Looks fine" is not a finding.

### 4. Aggregate

Read all findings files and collate:

1. **BLOCKING** items from any harness → escalate immediately. One BLOCKING
   is enough to stop.
2. **WARNING** items with ≥2 harnesses in agreement → treat as BLOCKING.
3. **WARNING** items unique to one harness → review manually, weigh cost.
4. **NOTE** items → log; address in a follow-up pass or ignore.

Surface disagreements explicitly. If harness A says BLOCKING and harness B
says NOTE for the same area, show both verdicts — do not silently collapse
to the weaker one.

```bash
# Combine findings for review
cat findings-codex.md findings-claude.md > findings-combined.md
```

### 5. Iterate until clean

Revise the target based on BLOCKING findings. Re-run the adversarial review
against the revised version. Typical well-scoped plans converge in 2–3
rounds. Stop when:

- No harness returns a BLOCKING verdict, AND
- All WARNINGs have been explicitly accepted or addressed.

Record the round count and final verdict in the bead evidence:

```bash
ddx bead evidence add <id> --type review --body findings-combined.md
```

## Adversarial framing

The adversarial framing instruction to include in every review prompt:

> You are a critic, not a validator. Your job is to find every way this
> could fail, every constraint it leaves undefined, every assumption it
> bakes in without stating, every interface it leaves ambiguous. A BLOCKING
> finding is anything that would cause implementation rework, a migration
> hazard, or a spec gap that agents will interpret differently. Do not
> balance criticism with praise — a useful adversarial review is entirely
> about what is wrong.

## Personas

Bind a `specification-enforcer` persona for the strictest checks:

```bash
ddx run --harness codex \
  --persona specification-enforcer \
  --prompt review-target.md
```

The `specification-enforcer` persona refuses drift from governing artifacts
and surfaces spec gaps others miss.

## Anti-patterns

- **Single-harness adversarial review.** The point is cross-model coverage.
  One harness is bead review, not adversarial review.
- **No structured output contract.** Free-form critique is unaggregateable.
  Always include the findings table + verdict contract in the prompt.
- **Collapsing disagreements.** If harnesses disagree, show both verdicts.
  Hiding disagreement defeats the purpose.
- **Running adversarial review on routine beads.** Reserve for high-stakes
  or controversial work. Routine beads use `ddx bead review`.
- **Stopping after one round.** One round finds problems. Iteration verifies
  that the revision actually resolves them.

## CLI reference

```bash
# Parallel adversarial dispatch
ddx run --harness codex  --min-power 10 --prompt review-target.md > findings-codex.md  &
ddx run --harness claude --min-power 10 --prompt review-target.md > findings-claude.md &
wait

# With persona
ddx run --harness codex --persona specification-enforcer \
  --prompt review-target.md > findings-codex.md

# Store findings as evidence
ddx bead evidence add <id> --type review --body findings-combined.md

# Bead review (routine, single-harness — not this skill)
ddx bead review <id>
ddx bead review <id> --execute --harness claude
```

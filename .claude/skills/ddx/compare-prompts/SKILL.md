---
name: compare-prompts
description: Dispatch the same prompt across N agent invocations (different harnesses, models, personas, or prompt variants) and aggregate the results into a single comparison report. Use when you need to benchmark prompt variants, run a quorum-style consensus check, compare model outputs side-by-side, or replay a prompt under altered conditions. Replaces the removed core quorum flag and benchmark CLI.
---

# Compare Prompts

A workflow skill for N-arm dispatch and result aggregation. Composes
`ddx run` (the layer-1 invocation primitive) into a comparison
matrix: same prompt, varied conditions, structured aggregation.

This skill replaces two retired surfaces:

- the removed inline quorum flag.
- the retired benchmark CLI from the old task-dispatch namespace.

Both collapse into the same workflow shape: dispatch N invocations,
aggregate their outputs, render a verdict.

## When to use

- **Prompt benchmarking** — compare two prompt variants on the same
  task to decide which one ships.
- **Model comparison** — same prompt across `claude` / `codex` /
  `gemini` (or different models within one harness) to see where
  outputs diverge.
- **Consensus checks (quorum)** — run a review prompt across N
  harnesses and require majority/unanimous agreement before acting.
- **Persona comparison** — same prompt under different personas to
  see how role framing changes the answer.
- **Replay under altered conditions** — re-run a prior prompt with a
  changed model, power bound, or context to A/B a hypothesis.

For pressure-testing a plan or spec with adversarial framing, prefer
the `adversarial-review` skill — it layers a critic-only prompt
contract on top of this same N-arm shape.

## Workflow

### 1. Define the comparison axis

Pick exactly one axis to vary. Mixing axes makes results
uninterpretable.

| Axis | Vary | Hold constant |
|---|---|---|
| harness | `--harness claude`, `--harness codex`, `--harness gemini` | prompt, power bounds, persona |
| model | `--model <a>`, `--model <b>` | harness, prompt, persona |
| prompt | prompt-v1.md, prompt-v2.md | harness, model, persona |
| persona | `--persona reviewer`, `--persona architect` | harness, prompt, model |
| power | `--min-power 1`, `--min-power 10` | harness, prompt, persona |

If you must vary two axes (e.g., harness × model), enumerate the full
matrix explicitly and label each arm.

### 2. Author the prompt

Write a single prompt file. Include an **output contract** so each
arm produces structurally comparable output:

```markdown
## Task

<the actual ask — code review, plan critique, design question, etc.>

## Output contract

Produce your answer as:

### Verdict: <one line>

### Reasoning
<2–6 sentences>

### Specific findings
- <finding 1>
- <finding 2>
```

Without an output contract, aggregation degenerates into freeform
prose comparison. The contract is what makes N-arm dispatch
mechanically aggregateable.

### 3. Dispatch the arms in parallel

Run each arm as a backgrounded `ddx run`, capturing stdout
to a per-arm file. Wait for all arms before aggregating.

```bash
PROMPT=prompt.md
OUT=compare-$(date +%Y%m%d-%H%M%S)
mkdir -p "$OUT"

# Three-arm harness comparison
ddx run --harness claude --min-power 10 --prompt "$PROMPT" > "$OUT/claude.md" 2> "$OUT/claude.err" &
ddx run --harness codex  --min-power 10 --prompt "$PROMPT" > "$OUT/codex.md"  2> "$OUT/codex.err"  &
ddx run --harness gemini --min-power 10 --prompt "$PROMPT" > "$OUT/gemini.md" 2> "$OUT/gemini.err" &
wait
```

Per-arm conventions:

- One file per arm, named for the varied axis value (`claude.md`,
  `codex.md`, …) — not `arm1.md` / `arm2.md`.
- Capture stderr separately. Failures in one arm should not silently
  drop a result row from the aggregate.
- Tag arms with the full invocation in a header comment when the
  filename alone doesn't disambiguate (e.g., model+persona variants).

### 4. Verify each arm produced output

Before aggregating, confirm each arm exited cleanly and produced
output that satisfies the prompt's output contract. An arm that
errored or returned empty must be marked as such — never silently
omitted.

```bash
for arm in "$OUT"/*.md; do
  if [ ! -s "$arm" ]; then
    echo "ARM FAILED: $arm"
  fi
done
```

### 5. Aggregate

Aggregate per the comparison axis. The aggregation policy depends on
the use case:

**Quorum (consensus check):**

- `unanimous` — every arm must produce the same verdict.
- `majority` — strictly more than half the arms agree.
- `any` — at least one arm produced the verdict (lowest bar).

Surface disagreement explicitly. If 2 of 3 arms say APPROVE and 1
says BLOCK, the report shows all three verdicts plus the policy
outcome. Hidden disagreement defeats the purpose of multi-arm
dispatch.

**Benchmark (prompt or model variant comparison):**

- Tabulate the structured output across arms (verdict + findings).
- Flag arms that diverged from the others on the verdict line.
- Note quality differentials — but do not pick a winner mechanically;
  human judgment closes the comparison.

**Replay (altered-conditions re-run):**

- Compare the new arm against the prior baseline arm directly.
- Surface the diff in verdict and findings; do not collapse to a
  single "improved / regressed" label.

### 6. Render the comparison report

Combine arms into a single Markdown report with a row per arm and
the structured output side-by-side:

```bash
cat > "$OUT/report.md" <<EOF
# Compare-prompts report

- Axis: harness
- Prompt: $PROMPT
- MinPower: 10
- Arms: claude, codex, gemini

## Per-arm output

### claude
$(cat "$OUT/claude.md")

### codex
$(cat "$OUT/codex.md")

### gemini
$(cat "$OUT/gemini.md")

## Aggregation

<verdict table + policy outcome>
EOF
```

If the comparison is associated with a bead (e.g., a quorum review
of the bead's diff), attach the report as evidence:

```bash
ddx bead evidence add <id> --type review --body "$OUT/report.md"
```

## Replacing Removed Quorum Flags

The retired inline quorum flags map to this workflow:

| Old | New |
|---|---|
| old harness list | three parallel `ddx run --harness <name>` calls |
| old majority policy | aggregation step 5, "majority" policy |
| old unanimous policy | aggregation step 5, "unanimous" policy |
| inline aggregated stdout | `report.md` rendered by step 6 |

The skill formalizes what core used to do inline. Behavioral parity
requires choosing the same harness set, the same policy, and routing each arm
through the same `MinPower`/`MaxPower` bounds.

## Replacing `agent benchmark`

The retired benchmark CLI mapped a prompt across a fixed matrix and rendered a
results table. The skill subsumes it by:

- Authoring the matrix as the per-arm dispatch list in step 3.
- Reusing the output contract in step 2 to keep arms aggregateable.
- Rendering the results table in step 6.

For benchmarking across many prompts (a prompt suite), see the
`benchmark-suite` skill — it composes this skill across a prompt
matrix.

## Anti-patterns

- **Mixing axes silently.** Varying both harness and prompt at once
  hides which variable produced the difference. Pick one axis, or
  enumerate the full matrix.
- **No output contract.** Free-form arm output is not
  aggregateable. Always include the structured output contract in
  the prompt.
- **Dropping failed arms.** A failed arm is data. Mark it as failed
  in the report — do not silently aggregate over the survivors.
- **Picking a winner mechanically.** Quorum aggregation is
  mechanical (policy → verdict). Benchmark aggregation requires
  human judgment on quality, not a counting rule.
- **One-arm "comparison".** A single dispatch is `ddx run`,
  not a comparison. This skill requires N ≥ 2.
- **Reusing `arm1.md` / `arm2.md` filenames.** Name files for the
  varied axis value so the report is self-describing.

## CLI reference

```bash
# Three-harness quorum (majority policy)
ddx run --harness claude --min-power 10 --prompt prompt.md > out/claude.md &
ddx run --harness codex  --min-power 10 --prompt prompt.md > out/codex.md  &
ddx run --harness gemini --min-power 10 --prompt prompt.md > out/gemini.md &
wait

# Two-prompt benchmark (one harness, varying prompt)
ddx run --harness claude --min-power 10 --prompt prompt-v1.md > out/v1.md &
ddx run --harness claude --min-power 10 --prompt prompt-v2.md > out/v2.md &
wait

# Two-persona comparison
ddx run --harness claude --persona reviewer  --prompt prompt.md > out/reviewer.md  &
ddx run --harness claude --persona architect --prompt prompt.md > out/architect.md &
wait

# Attach the comparison report as bead evidence
ddx bead evidence add <id> --type review --body out/report.md
```

## Related skills

- `adversarial-review` — N-arm dispatch with adversarial critic
  framing layered on top.
- `benchmark-suite` — composes `compare-prompts` across a prompt
  matrix for suite-level evaluation.
- `replay-bead` — re-run a prior bead's prompt under altered
  conditions; uses the replay aggregation shape from step 5.

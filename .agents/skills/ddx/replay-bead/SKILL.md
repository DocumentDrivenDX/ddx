---
name: replay-bead
description: Re-run a previously executed bead under altered conditions (different harness, model, profile, persona, or base revision) and diff the new attempt against the prior baseline. Use when investigating why an attempt failed, A/B-testing a model upgrade on a known task, or sanity-checking a regression. Replaces the deprecated replay CLI from the old task-dispatch namespace.
---

# Replay Bead

A workflow skill for re-running a prior bead attempt under altered
conditions and diffing the result against the original baseline.
Composes `ddx try` (the layer-2 bead-attempt primitive) with the
output structure from `compare-prompts` step 5 (replay aggregation).

This skill replaces the retired replay CLI from the old task-dispatch
namespace. The retired command silently re-ran a bead's reconstructed prompt
under a swapped harness/model and dropped the result into a sandbox worktree.
The skill formalizes the same shape: pin the baseline, vary exactly one
condition, capture the new attempt as a peer record, render a diff.

## When to use

- **Failure investigation** — a bead attempt failed or produced a
  poor diff; replay it under a stronger harness or different profile
  to see whether the task is tractable at all.
- **Model upgrade A/B** — replay a representative bead under the
  newer model to see whether the diff materially improves before
  rolling the model out as the default.
- **Regression sanity check** — replay a previously-green bead under
  a candidate change (new prompt, new persona, new profile) to
  confirm nothing regressed on a known-good task.
- **Base-revision sensitivity** — replay against current HEAD instead
  of the original parent revision to see whether intervening commits
  changed what the agent produces.

For dispatching the *same* prompt across N peers in parallel without
a baseline, prefer `compare-prompts`. Replay is specifically a
**1-vs-baseline** shape, not an N-arm fan-out.

## Workflow

### 1. Pin the baseline

Identify the prior attempt you are replaying against. Capture:

- bead id (`ddx-<short>`)
- the attempt's record id (the `<attempt-id>` directory under the
  unified run substrate)
- the harness, model, profile, persona, and base revision the
  baseline ran under
- the resulting diff (merged commit, preserved iteration ref, or
  no_changes_rationale)

```bash
BEAD=ddx-abc123
BASELINE_ATTEMPT=20260420T101530-aaaaaaaa
ddx tries show "$BASELINE_ATTEMPT" > replay/baseline.txt
```

A replay without a pinned baseline is just a fresh `ddx try` — there
is nothing to diff against.

### 2. Vary exactly one condition

Pick one axis to alter. Mixing axes makes the diff uninterpretable —
you cannot tell whether the new model or the new persona drove the
difference.

| Axis | Vary | Hold constant |
|---|---|---|
| harness | `--harness <other>` | model, power bounds, persona, base rev |
| model | `--model <other>` | harness, power bounds, persona, base rev |
| power | `--min-power <n>` / `--max-power <n>` | harness, model, persona, base rev |
| persona | `--persona <other>` | harness, model, power bounds, base rev |
| base rev | `--from <rev>` (or `--at-head`) | harness, model, power bounds, persona |
| prompt | regenerated bead description / spec | harness, model, power bounds, persona, base rev |

If the comparison genuinely requires varying two axes, run two
replays sequentially against the same baseline rather than one replay
with two changes — each replay then has a clean 1-vs-baseline diff.

### 3. Dispatch the replay attempt

Run `ddx try` with the altered condition. The replay must run in an
isolated worktree so it cannot stomp the baseline result; `ddx try`
defaults to that, but pass `--no-merge` so the replay stays preserved
as an iteration ref rather than fast-forwarding into the base branch.

```bash
REPLAY_OUT=replay-$BEAD-$(date +%Y%m%d-%H%M%S)
mkdir -p "$REPLAY_OUT"

ddx try "$BEAD" \
  --from "$BASELINE_BASE_REV" \
  --harness codex \
  --min-power 10 \
  --no-merge \
  > "$REPLAY_OUT/replay.log" 2> "$REPLAY_OUT/replay.err"
```

Notes:

- `--from <rev>` pins the base revision to match the baseline. Omit
  (or use `--at-head`) only when base-rev is the axis being varied.
- `--no-merge` preserves the replay's worktree state as an iteration
  ref so its diff can be inspected without disturbing the base
  branch. The baseline already shipped (or didn't); the replay is an
  observation, not a re-merge.
- Capture stderr separately. A replay that errored is a result —
  surface it in the report rather than silently dropping it.

### 4. Locate the replay's record

`ddx try` writes a layer-2 record under the unified run substrate.
Capture its attempt id alongside the baseline's:

```bash
REPLAY_ATTEMPT=$(ddx tries list --bead "$BEAD" --limit 1 --format id)
ddx tries show "$REPLAY_ATTEMPT" > "$REPLAY_OUT/replay.txt"
```

### 5. Diff baseline vs replay

The replay aggregation shape is a direct 1-vs-1 diff, not an N-arm
roll-up. Surface differences along three dimensions:

**Outcome diff:**

| Dimension | Baseline | Replay |
|---|---|---|
| exit | merged / preserved / no_changes | merged / preserved / no_changes |
| AC items satisfied | `<n>/<m>` | `<n>/<m>` |
| token / duration | from run record | from run record |

**Code diff:**

```bash
git diff "$BASELINE_REF" "$REPLAY_REF" -- > "$REPLAY_OUT/code.diff"
```

Where `$BASELINE_REF` is the baseline merge commit (or its
preserved-iteration ref) and `$REPLAY_REF` is the replay's preserved
iteration ref. The code diff is the load-bearing artifact — the
verdict is downstream of "what actually changed in the worktree."

**Reasoning diff (if available):**

If both attempts produced session logs or written rationale, summarize
how they differ — but do not let the model self-narrative dominate
over the code diff. The code is the ground truth.

### 6. Render the replay report

```bash
cat > "$REPLAY_OUT/report.md" <<EOF
# Replay report: $BEAD

- Baseline attempt: $BASELINE_ATTEMPT
- Replay attempt:   $REPLAY_ATTEMPT
- Axis varied:      harness (claude → codex)
- Held constant:    min-power=10, persona=<same>, base-rev=<same>

## Outcome diff

<table from step 5>

## Code diff

\`\`\`diff
$(cat "$REPLAY_OUT/code.diff")
\`\`\`

## Verdict

<improved / regressed / equivalent — human judgment, not mechanical>
EOF
```

If the replay is associated with the bead's evidence trail, attach:

```bash
ddx bead evidence add "$BEAD" --type replay --body "$REPLAY_OUT/report.md"
```

Do not collapse the verdict to a single "improved/regressed" label
mechanically. Replay verdicts require human judgment on whether the
new diff is genuinely better, equivalent under noise, or regressed
on a dimension the baseline got right.

## Replacing the retired replay CLI

The retired command mapped to this workflow:

| Old | New |
|---|---|
| old replay CLI with `--harness <h>` | `ddx try <bead> --harness <h> --no-merge` (step 3) |
| old replay CLI with `--model <m>` | `ddx try <bead> --model <m> --no-merge` |
| old replay CLI with `--at-head` | `ddx try <bead> --at-head --no-merge` (varies base-rev axis) |
| old replay CLI with `--sandbox` | always implicit — `ddx try` runs in an isolated worktree |
| inline reconstructed prompt | bead description is re-resolved by `ddx try` per layer-2 |
| no diff rendering | step 5/6 produce explicit baseline-vs-replay diff |

Behavioral parity requires pinning `--from` to the baseline's base
revision (the retired command defaulted to "closing commit parent")
unless base-rev is the varied axis.

## Anti-patterns

- **Replay without a pinned baseline.** A replay against nothing is
  just a `ddx try` re-attempt; there is no diff and no insight. If
  you do not have an attempt id to compare against, you are not
  replaying — you are re-trying.
- **Varying multiple conditions at once.** A replay that swaps
  harness *and* profile cannot attribute the diff to either change.
  Run sequential single-axis replays instead.
- **Merging the replay.** Replays are observations, not re-merges.
  Always pass `--no-merge` so the baseline's already-shipped result
  is not silently overwritten by a "better-looking" replay diff.
- **Mechanical winner-picking.** "Replay produced more lines /
  closed more AC items" is not a verdict. The replay diff might be
  larger and worse. Human judgment closes the comparison.
- **Comparing only the rationale.** Self-narratives diverge between
  attempts even when the code diff is essentially identical. The
  code diff is the ground truth; the rationale is supporting
  context.
- **Skipping `--from`.** Omitting `--from` lets the replay drift to
  HEAD; the resulting diff conflates "different agent" with
  "different starting state." Pin the base rev unless that *is* the
  axis being varied.

## CLI reference

```bash
# Replay under a different harness, baseline-pinned, no merge
ddx try "$BEAD" --from "$BASELINE_BASE_REV" --harness codex --no-merge

# Replay under a different model
ddx try "$BEAD" --from "$BASELINE_BASE_REV" --model claude-opus-4-7 --no-merge

# Replay against current HEAD (varies the base-rev axis)
ddx try "$BEAD" --at-head --no-merge

# Inspect baseline and replay records
ddx tries show "$BASELINE_ATTEMPT"
ddx tries show "$REPLAY_ATTEMPT"

# Diff the worktree refs
git diff "$BASELINE_REF" "$REPLAY_REF" --

# Attach the replay report as bead evidence
ddx bead evidence add "$BEAD" --type replay --body replay-out/report.md
```

## Related skills

- `compare-prompts` — N-arm parallel dispatch when there is no
  baseline, just a comparison matrix. Replay is its 1-vs-baseline
  specialization.
- `adversarial-review` — pressure-test the replay's diff with a
  critic prompt before accepting "replay improved baseline."
- `benchmark-suite` — for replaying many beads under the same
  altered condition (e.g., "how does the new model perform across
  the last 20 closed beads"), compose this skill across a bead
  matrix.

---
ddx:
  id: TD-037
  depends_on:
    - FEAT-006
    - FEAT-010
    - FEAT-014
    - ADR-024
  status: draft
---
# Technical Design: Bead Estimated Difficulty

## Purpose

DDx needs a durable way to say how difficult a bead appears before execution
without turning the queue into route policy. The bead record must stay portable
across machines, providers, and harnesses. It must not store concrete
harness/provider/model choices, Fizeau profile names, or numeric power floors.

This design defines exactly one optional bead-level hint:

```text
triage.estimated_difficulty = easy | medium | hard
```

The hint describes task difficulty. It does not describe a model, provider,
harness, profile, or numeric power value.

## Policy

1. If a bead explicitly sets `triage.estimated_difficulty`, DDx uses it.
2. If the field is absent or invalid, DDx treats the bead as `medium`.
3. DDx maps difficulty to an abstract numeric `MinPower` only at dispatch time.
   These are DDx work-policy constants on Fizeau's public 1–10 scale, not
   catalog-derived route facts:

   | Estimated difficulty | Requested `MinPower` |
   |---|---|
   | `easy` | `0` (unset; Fizeau applies its default policy) |
   | `medium` | `7` |
   | `hard` | `9` |

4. Fizeau still owns concrete route selection at or above the requested
   `MinPower`. DDx must not persist route-side choices back onto the bead.
5. Retry escalation may raise the next attempt's requested `MinPower`, but that
   retry state is execution policy, not bead triage metadata. Numeric retry
   floors must not be written to bead `Extra`.

## Durable Hint Surface

DDx recognizes exactly one durable bead-level hint surface:

| Field | Values | Meaning |
|---|---|---|
| `triage.estimated_difficulty` | `easy`, `medium`, `hard` | Optional task-difficulty estimate used to choose the initial abstract `MinPower`. |

No other bead metadata affects power inference. In particular, DDx must not
read or write any of these as power hints:

- `triage.power_hint`;
- labels such as `power:smart`, `power:standard`, `power:cheap`, or
  `power:hint=...`;
- bead priority, issue type, kind labels, title length, description length, or
  acceptance length;
- numeric values in bead `Extra`.

Invalid `triage.estimated_difficulty` values are ignored and resolve to
`medium`.

## Difficulty Definitions

Use `easy` only for narrow mechanical work with low blast radius:

- typo, formatting, or wording fixes;
- simple docs/prose edits;
- straightforward fixture updates;
- one-file transforms where the expected edit is obvious.

Use `medium` for ordinary implementation work:

- normal build/test/code changes;
- routine bug fixes;
- localized refactors;
- tasks where the correct path is clear enough for a standard implementer.

Use `hard` only when stronger reasoning is materially useful:

- architecture, API, or data-model tradeoffs;
- ambiguous or competing requirements;
- multi-subsystem work with high blast radius;
- security, data-loss, migration, or concurrency risk;
- prior attempts producing capability-sensitive evidence at the ordinary floor.

Do not choose `hard` just because a bead is important, long, or could be
written more cleanly. Low readiness means refine, split, or block the bead; it
does not mean the bead is hard.

## Initial Power Inference

`escalation.InferInitialMinPower(bead)` must be intentionally small:

1. Read `triage.estimated_difficulty` from bead `Extra`.
2. If it is `easy`, return `0`.
3. If it is `medium`, return `7`.
4. If it is `hard`, return `9`.
5. Otherwise return `7`.

There is no heuristic fallback. There is no label fallback. There is no numeric
fallback from bead metadata. The default is the ordinary DDx work-policy floor
of `7`. DDx never queries Fizeau to discover or repair this value.

## Readiness Classification

The readiness model may estimate task difficulty as part of its structured
result. It must use the same bead-facing vocabulary:

```json
{
  "difficulty": {
    "estimated_difficulty": "easy|medium|hard",
    "reason": "short reason"
  }
}
```

Readiness score and difficulty are separate judgments. Readiness answers
whether the bead is executable as written. Difficulty answers what initial
abstract `MinPower` is appropriate if the bead is executable.

If the bead already has `triage.estimated_difficulty`, that authored metadata
takes precedence over readiness output. If the bead has no difficulty hint,
readiness output may be used for the current dispatch decision. It must not be
stored as `triage.power_hint`, a Fizeau profile, a numeric power floor, or any
second durable hint field.

## Dispatch Mapping

For `ddx try` and `ddx work`, DDx maps the internal difficulty class to a
numeric `MinPower` floor on the pinned public Fizeau power scale. The mapping
is DDx work policy, not route policy: it does not query Fizeau catalogs,
availability, candidates, providers, models, or profiles. Fizeau independently
chooses the concrete route that satisfies the request.

An operator-supplied `MinPower` or public `Policy` is passed through unchanged
and suppresses inference. Operator-supplied `MaxPower`, `Harness`, `Provider`,
and `Model` constraints are also immutable passthrough; DDx never adds, removes,
widens, or repairs them. If a raised floor conflicts with one of those
constraints, the operation returns to the operator instead of changing the
constraint.

## Precedence

Execution intent resolves in this order:

1. Explicit operator `MinPower` or public `Policy` for the current invocation.
2. Project DDx abstract-power policy.
3. Bead `triage.estimated_difficulty`.
4. Default `medium` difficulty, which maps to `MinPower=7`.

There are no additional bead-level paths.

## Retry Escalation

DDx owns new-attempt and review-strength policy. It may raise the next distinct
attempt's requested `MinPower` only after capability-sensitive DDx evidence,
and it may request a stronger reviewer with a higher floor. Transport, route,
quota, authentication, setup, operator-action, and generic failures never
raise power. In-attempt repair keeps the same power.

Retry escalation must not mutate `triage.estimated_difficulty` and must not
write numeric floors to bead `Extra`. The bead field is a triage estimate, not a
retry-state register.

## Rejected Durable Route Pins

Bead metadata must not persist concrete route choices. The lint hook should
reject or block execution when a bead includes durable fields or labels that
look like route pins, including:

- `harness`, `agent-harness`, `execution-harness`, `try-harness`;
- `provider`, `agent-provider`, `execution-provider`, `try-provider`;
- `model`, `agent-model`, `execution-model`, `try-model`;
- `model-ref`, `agent-model-ref`, `execution-model-ref`, `try-model-ref`;
- labels such as `harness:claude`, `provider:openai`, or `model:gpt-...`.

The diagnostic should say to use a one-off CLI flag for a reproduction or
explicit operator constraint:

```bash
ddx try <id> --harness claude
```

## Audit Evidence

Every `ddx try` / `ddx work` attempt should record routing-intent evidence
before execution starts. Minimum fields:

| Field | Meaning |
|---|---|
| `bead_id` | Target bead. |
| `attempt_id` | Attempt/run id. |
| `routing_intent_source` | `cli`, `project_config`, `bead_hint`, `readiness`, or `default`. |
| `estimated_difficulty` | `easy`, `medium`, `hard`, or empty when not difficulty-based. |
| `inferred_min_power` | `0`, `7`, `9`, or empty when explicit operator intent suppresses inference. |
| `requested_policy` | Public Fizeau `Policy` supplied explicitly by the operator, when present. |
| `requested_min_power` | Resolved `MinPower`, when available. |
| `requested_max_power` | Operator-supplied `MaxPower`, when present. |
| `actual_harness` | Harness reported by Fizeau/agent after execution. |
| `actual_provider` | Provider reported after execution. |
| `actual_model` | Model reported after execution. |
| `actual_power` | Actual power reported after execution. |
| `routing_intent_note` | DDx policy source or constraint-conflict note; never a DDx route decision. |

The evidence event name should be stable, for example
`execution-routing-intent`.

## Metrics

The agent metrics rollup should ingest DDx-owned request-intent fields once they
are present in run evidence. Fizeau-returned actual-route fields remain
per-invocation audit facts and are not list filters, grouping keys, or grading
dimensions. At minimum, operators need to answer:

- How many attempts started from each estimated difficulty?
- What success rate and cost did each difficulty/`MinPower` mapping produce?
- How often did CLI or project config override bead difficulty?
- Which beads carried rejected durable route pins?

Suggested counters:

- attempts by estimated difficulty and source;
- attempts by inferred `MinPower` and source;
- success rate by estimated difficulty;
- cost and token usage by estimated difficulty;
- rejected durable route-pin count;
- override/degradation count.

## Operator Reporting

`ddx try` output should stay concise, but it should include the source when a
bead difficulty affects execution:

```text
routing intent: difficulty=hard minPower=9 source=bead_hint
```

If a durable concrete pin is found:

```text
bead metadata contains execution-model=gpt-5.5; durable model pins are not
allowed. Use ddx try <id> --model ... for one-off debugging.
```

## Implementation Plan

This is one cleanup pass, not a new routing subsystem. The only durable bead
field added or read by this design is `triage.estimated_difficulty`.

### 1. Replace the bead hint parser

- Introduce one constant: `EstimatedDifficultyKey =
  "triage.estimated_difficulty"`.
- Introduce one enum/parser for `easy`, `medium`, and `hard`.
- Make `InferInitialMinPower(bead)` do only this:
  1. read `triage.estimated_difficulty`;
  2. map `easy -> 0`, `medium -> 7`, `hard -> 9`;
  3. return `7` when absent or invalid.
- Delete the production `BeadPowerHint`/`TriagePowerHint` inference helpers.
- Do not read `triage.power_hint`, `power:*` labels, priority, kind, issue type,
  text length, acceptance length, or numeric `Extra` values.

Acceptance:

1. Tests cover `easy`, `medium`, `hard`, absent, and invalid values.
2. Tests prove `triage.power_hint`, `power:*` labels, priority, kind, issue
   type, text length, acceptance length, and numeric `Extra` values are ignored.
3. No production inference path references `BeadPowerHint`,
   `TriagePowerHintKey`, or `triage.power_hint`.

### 2. Make readiness estimate difficulty, not power

- Change the readiness prompt to request:

  ```json
  {"difficulty":{"estimated_difficulty":"easy|medium|hard","reason":"..."}}
  ```

- Decode canonical and legacy readiness payloads into an in-memory
  `EstimatedDifficulty` field.
- Apply precedence in memory: bead `triage.estimated_difficulty` first,
  readiness estimate second, default `medium` last.
- Do not write readiness difficulty to any bead `Extra` key during the normal
  readiness path.
- Do not create a second durable hint field for readiness.

Acceptance:

1. Readiness prompt and tests use `easy`, `medium`, and `hard`.
2. Canonical and legacy readiness JSON decode `estimated_difficulty`.
3. A bead-authored difficulty wins over readiness difficulty.
4. Readiness can affect the current dispatch request without mutating bead
   metadata.
5. Tests assert readiness does not write `triage.power_hint`,
   `triage.estimated_difficulty`, or numeric power values to bead `Extra`.

### 3. Delete numeric bead retry floors

- Delete `numericPowerFloorHint` and remove every call to it.
- Remove all writes of numeric floors to bead `Extra`.
- Remove all writes of power-class strings to `TriagePowerHintKey`.
- Remove stale comments/log messages that describe escalating
  `TriagePowerHintKey`.
- If a distinct new attempt needs a higher `MinPower` after
  capability-sensitive evidence, carry that as execution policy outside the
  bead metadata. If that state is not already available in memory, do not add
  a replacement bead key.

Acceptance:

1. `ddx try`, investigation retry, no-changes retry, infrastructure-failure
   handling, review-block triage, and post-review escalation do not read or
   write numeric bead `Extra` values as MinPower.
2. Tests that currently assert `TriagePowerHintKey` numeric mutation are
   replaced with tests that assert bead difficulty metadata is unchanged.
3. No production code writes `triage.power_hint`.

### 4. Simplify lint and evidence

- Keep lint for durable concrete route pins only.
- Remove `SMART JUSTIFICATION` enforcement for difficulty metadata.
- Record request intent as `estimated_difficulty` plus mapped
  `inferred_min_power`.
- Remove `smart_justification` from new routing-intent evidence.
- Historical attempt parsing may keep old fields only for old evidence records;
  it must not affect current inference.

Acceptance:

1. `LintExecutionHints` rejects durable harness/provider/model pins and does not
   reject `triage.estimated_difficulty=hard` for missing prose.
2. Attempts with `triage.estimated_difficulty=hard` record
   `estimated_difficulty=hard` and `inferred_min_power=9`.
3. Metrics can report attempts, cost, and success by estimated difficulty.

### 5. Update tests and docs references

- Update active and shipped DDx skill references so they do not teach
  `TriagePowerHintKey`, `triage.power_hint`, or `power:*` as durable hint
  mechanisms.
- Update command, agent, escalation, readiness, infrastructure-failure, no-changes,
  review-block, post-review, routing-evidence, and metrics tests to match the
  single-field contract.

Acceptance:

1. `rg` over production code finds no current inference or retry path for
   `triage.power_hint`, `TriagePowerHintKey`, or `numericPowerFloorHint`.
2. `rg` over tests finds no assertions that numeric floors are written to bead
   `Extra`.
3. Bead authoring docs document only `triage.estimated_difficulty`.

## Non-Goals

- No durable bead-level concrete harness/provider/model pins.
- No DDx-side concrete route ranking or fallback.
- No change to Fizeau's routing algorithm.
- No materialized metrics store in the first implementation.
- No `triage.power_hint` compatibility path.
- No `power:*` label compatibility path.
- No heuristic power inference from priority, kind, issue type, or text length.
- No numeric retry floors stored on beads.

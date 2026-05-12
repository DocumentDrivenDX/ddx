# Bead Readiness Gate — Pre-Claim Actionability Evaluator

The intake gate is a pre-Claim phase in `ddx work` that evaluates each
candidate bead before an agent worker claims it. Its purpose is to improve beads
that can be safely clarified, prevent coarse epics from being dispatched as
monolithic work items, and hold ambiguous work for operator decision before an
implementer is forced to guess.

## When the gate triggers

The gate runs for every ready bead before `Store.Claim`. It evaluates the bead's
title, description, acceptance criteria, labels, parent, dependencies, spec-id,
and prior attempt history to classify it as:

- **actionable_atomic** — a single coherent unit of work; proceeds to Claim
  normally.
- **actionable_but_rewritten** — the gate made a validated replacement rewrite
  or other safe, intent-preserving bead update through `ddx bead update`;
  proceeds to Claim after the mutation.
- **too_large_decomposed** — multiple independent deliverables; the gate files
  child beads, records the AC map, and leaves the parent `status=open` with
  dependency edges to the children.
- **ambiguous_requires_operator** — needs human clarification; the gate moves
  the bead to `status=proposed`.

Safe rewrites may replace the bead body when the replacement is clearer,
execution-ready, and validated against durable anchors. The gate should not
append clarifying noise simply to satisfy preservation: the output bead is the
next agent's prompt, so prompt fitness is part of correctness. Some replacements
expand a vague one-line bead into a standalone task; others compress noisy or
stale prose. The original text is retained in readiness evidence with
before/after hashes.

Validated preservation is based on explicit commitments and durable context:
acceptance criteria, non-scope, governing artifact references, dependencies,
named files/tests, and still-valid root-cause evidence. The gate may replace
stale line numbers or chat-shaped prose with current section anchors or file:line
evidence. It must not invent product behavior, change scope, choose between
conflicting requirements, delete unresolved constraints, or guess a missing
governing artifact.

The same decomposition policy is also used as a post-attempt safety net. If an
implementation attempt returns `no_changes_needs_investigation` or an equivalent
structured outcome whose rationale says the bead is too large or cannot be
safely split inside the implementation worktree, `ddx work` must run the
orchestrator-level splitter on the original bead. That fallback is not operator
work unless the splitter cannot produce a lossless AC map or the configured
decomposition depth has truly been exhausted at the queue level; those cases
move the bead to `status=proposed`.

## Bypassing the gate per-bead

Add the label `triage:skip` to a bead to bypass the bead readiness gate entirely:

```
ddx bead update <id> --labels triage:skip
```

The bead will proceed directly to Claim on the next dispatch cycle. Use this
for beads you have manually reviewed and confirmed are atomic, or for beads
that the gate misclassifies.

## Decomposed parent state

When the gate decomposes a bead:

1. Child beads are filed with `parent: <id>` linking back to the epic.
2. The parent's status remains `open`; dependency waiting is derived from the
   new child dependency edges, not `status=blocked`.
3. The parent receives a `kind:triage-decomposed` event whose JSON body lists
   the `child_ids`, the splitter's `rationale`, and an `ac_map`.
4. Dependency edges are added: the parent depends on all children, ensuring it
   cannot be dispatched again until children close (should it be re-opened).

The `ac_map` is load-bearing. Every parent acceptance criterion must map to at
least one child acceptance criterion or be explicitly marked
`operator_required` or `non_scope` with a rationale. A split that drops an AC is
invalid and moves the parent to `status=proposed` instead of dispatching
children as if the work were complete.

Children re-enter the triage gate on their next dispatch cycle. If a child is
itself decomposable, the gate will split it further — up to the depth cap.

## Operator acceptance and re-readiness

When an operator moves a decomposed or overflowed bead from `status=proposed`
back to `status=open`, DDx records `triaged` as durable acceptance of the
current prompt snapshot. Re-readiness may inspect the bead again, but it must
not send the bead back to `status=proposed` for the same decomposition or
ambiguity finding unless prompt-relevant fields changed or the operator
explicitly requests re-triage.

For lossy or depth-cap splits, the existing child beads and dependency edges
remain in place; operator acceptance restores the parent to the forward-progress
lane without creating an open↔proposed downgrade loop.

## Recursion depth cap

Default depth cap: **3** levels. Configurable in `.ddx/config.yaml` at
`agent.triage.max_decomposition_depth`:

```yaml
agent:
  triage:
    max_decomposition_depth: 3
```

When a bead at the queue-level depth cap is evaluated, the gate:

1. Appends a `kind:triage-overflow` event.
2. Sets `status=proposed` and records the depth overflow in triage evidence.
3. Does **not** invoke the classifier or splitter.
4. Does **not** dispatch the bead.

Implementation-attempt depth is not the same as queue-level decomposition depth.
If a worker reports that it cannot split because the attempt prompt forbids
another child layer, the orchestrator still owns the next split decision unless
the bead itself has reached `agent.triage.max_decomposition_depth`.

The operator must manually split the bead or add `triage:skip` to bypass only
after queue-level overflow or lossy/ambiguous decomposition is recorded.

## AC-coverage metric

The `bead-split` prompt is evaluated against a held-out corpus using both:

- hard AC traceability: every parent AC maps to child ACs, `operator_required`, or
  `non_scope`;
- string-overlap metric: the fraction of unique AC tokens (alphanumeric tokens
  ≥ 3 chars) from the parent's acceptance criteria that appear in the combined
  acceptance criteria of the child specs.

A rate ≥ 90% is required for the heuristic metric, but overlap alone is not
acceptance. The hard AC map is the gate that prevents lossy decomposition.

## Regenerating the historical corpus

The frozen eval slice lives in `library/prompts/triage/eval-corpus.jsonl`. To
regenerate from live bead history:

```bash
go run ./scripts/triage/harvest-corpus.go \
  --output library/prompts/triage/eval-corpus.jsonl
```

The harvest script reads `.ddx/beads.jsonl` from this repository and includes
`../agent/.ddx/beads.jsonl` when that checkout is present. It produces a
deterministic held-out eval slice. Running it with the same inputs always
produces the same corpus file.

## Ownership boundary

DDx owns orchestration, tracking, work sizing, child bead creation, parent
dependencies, and retry decisions. The agent owns provider/model/harness
routing and execution. The triage gate must not choose or rank concrete
providers, models, or harnesses; if an operator supplied those values, DDx
passes them through as opaque constraints to the agent runtime.

Decomposition is a high-judgment routing decision. The splitter must run through
the normal `ddx work` path with Fizeau's `smart` model-ref and a strong
`MinPower` floor, defaulting to the smart/top-power tier floor when the project
has no explicit splitter override. DDx still does not pick a concrete model: it
only requests the abstract smart ref, raises the power floor, and lets the agent
route within any operator-supplied passthrough constraints. If no available
route satisfies that strong floor, DDx records readiness as unavailable
(`readiness_error` / `intake_error`) instead of marking the bead as ambiguous or
moving it to `status=proposed`. DDx must not silently fall back to weak
decomposition; the worker continues, skips, or stops according to the configured
readiness-failure mode.

## Disabling the gate

The gate is disabled when `ExecuteBeadWorker.ComplexityGate == nil`. This is
intentionally non-silent: the worker emits a one-time warning per boot:

```
warning: triage complexity gate is disabled; coarse beads may waste dispatch attempts
```

The gate should only be disabled in test fixtures or during development. In
production, always wire a `ComplexityGate` via `NewComplexityGate`.

## Tracker events

| Event kind              | When                                       | Body                          |
|-------------------------|--------------------------------------------|-------------------------------|
| `triage-rewritten`      | Safe bead replacement/update applied       | `{fields, rationale, before, after}` |
| `triage-decomposed`     | Parent split into children                 | `{child_ids, rationale, ac_map}` |
| `triage-overflow`       | Bead at depth cap, blocked                 | `{depth, max}`                |
| `triage-ambiguous`      | Gate returned ambiguous classification     | `{confidence, reasoning}`     |
| `triage.gate_disabled`  | Loop boot with nil gate (loop event log)   | `{bead_id}`                   |
| `triage.skipped`        | Gate returned shouldClaim=false            | `{bead_id}`                   |
| `triage.error`          | Gate returned a non-nil error              | `{bead_id, reason}`           |

# Triage Gate — Pre-Claim Complexity Evaluator

The triage gate is a pre-Claim phase in `ddx work` that evaluates each candidate
bead before an agent worker claims it. Its purpose is to prevent coarse epics
(beads with multiple independent deliverables) from being dispatched as
monolithic work items, which wastes attempt slots and corrupts claim semantics.

## When the gate triggers

The gate runs for every ready bead, between the routing preflight check and
`Store.Claim`. It evaluates the bead's title, description, acceptance criteria,
and prior attempt history to classify it as:

- **atomic** — a single coherent unit of work; proceeds to Claim normally.
- **decomposable** — multiple independent deliverables; the gate files child
  beads and blocks the parent.
- **ambiguous** — needs human clarification; the gate sets
  `execution-eligible=false` and adds `label=triage:needs-human`.

## Bypassing the gate per-bead

Add the label `triage:skip` to a bead to bypass the complexity gate entirely:

```
ddx bead update <id> --labels triage:skip
```

The bead will proceed directly to Claim on the next dispatch cycle. Use this
for beads you have manually reviewed and confirmed are atomic, or for beads
that the gate misclassifies.

## Decomposed parent state

When the gate decomposes a bead:

1. Child beads are filed with `parent: <id>` linking back to the epic.
2. The parent's status is set to `blocked`.
3. The parent receives a `kind:triage-decomposed` event whose JSON body lists
   the `child_ids` and the splitter's `rationale`.
4. Dependency edges are added: the parent depends on all children, ensuring it
   cannot be dispatched again until children close (should it be re-opened).

Children re-enter the triage gate on their next dispatch cycle. If a child is
itself decomposable, the gate will split it further — up to the depth cap.

## Recursion depth cap

Default depth cap: **3** levels. Configurable in `.ddx/config.yaml` at
`agent.triage.max_decomposition_depth`:

```yaml
agent:
  triage:
    max_decomposition_depth: 3
```

When a bead at the depth cap is evaluated, the gate:

1. Appends a `kind:triage-overflow` event.
2. Sets `status=blocked` with `label=needs-human-decomposition`.
3. Does **not** invoke the classifier or splitter.
4. Does **not** dispatch the bead.

The operator must manually split the bead or add `triage:skip` to bypass.

## AC-coverage metric

The `bead-split` prompt is evaluated against a held-out corpus using a
string-overlap metric: the fraction of unique AC tokens (alphanumeric tokens
≥ 3 chars) from the parent's acceptance criteria that appear in the combined
acceptance criteria of the child specs. A rate ≥ 90% is required.

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
| `triage-decomposed`     | Parent split into children                 | `{child_ids, rationale}`      |
| `triage-overflow`       | Bead at depth cap, blocked                 | `{depth, max}`                |
| `triage-ambiguous`      | Gate returned ambiguous classification     | `{confidence, reasoning}`     |
| `triage.gate_disabled`  | Loop boot with nil gate (loop event log)   | `{bead_id}`                   |
| `triage.skipped`        | Gate returned shouldClaim=false            | `{bead_id}`                   |
| `triage.error`          | Gate returned a non-nil error              | `{bead_id, reason}`           |

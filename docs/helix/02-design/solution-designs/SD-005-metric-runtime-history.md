---
ddx:
  id: SD-005
  depends_on:
    - FEAT-005
    - FEAT-010
---
# Solution Design: Metric Runtime and History

## Purpose

This design defines how metrics specialize the generic DDx execution model.
`MET-*` artifacts remain declarative graph nodes; `ddx exec` definitions and
execution runs provide the reusable runtime substrate; metric-specific
conventions define how numeric observations, units, and comparisons are carried
inside that generic model.

## Scope

- Keep metrics artifact-generic.
- Reuse `ddx exec` definitions and runs rather than introducing a separate
  metric-only runtime primitive.
- Define the metric-specific fields expected inside execution definitions and
  structured run results.
- Define how metric history and trend inspection derive from generic execution
  history.
- Preserve compatibility with the existing artifact graph and tracker model.

## Storage Boundary

Metric runtime history is stored in bead-backed execution collections, not in
the primary active-work collection.

- A metric run may carry workflow correlation such as `bead_id`, but the
  authoritative observation record lives in the execution-run collection.
- The execution metadata row uses the bead envelope plus metric-specific fields
  in preserved extras.
- Raw logs and structured result blobs live in sidecar files referenced by the
  execution record.

## Data Model

### Metric artifact

A `MET-*` artifact declares the metric, the document context that governs it,
and the identity used to link runtime and history records.

### Execution definition

Metrics use ordinary DDx execution definitions from FEAT-010. A metric-linked
definition must:

- link to exactly one governing `MET-*` artifact via artifact ID
- declare an executor kind, initially `command`
- declare how the raw execution output becomes a structured metric result
- declare any comparison or threshold policy needed to interpret the result

Minimum metric-specific definition fields:
- `artifact_ids` including one `MET-*` ID
- `executor.kind`
- `executor.command`
- `executor.cwd`
- `executor.env`
- `result.metric.unit`
- `result.metric.value_path` or equivalent parse contract
- `result.metric.samples_path` when multiple observations are captured
- `evaluation.comparison`
- `active`
- `created_at`

### Execution run

Metrics use ordinary DDx execution runs from FEAT-010. Metric history is
derived by filtering generic execution runs for those linked to a `MET-*`
artifact and reading their structured metric payload.

The authoritative execution record is a bead-backed metadata row whose
attachment references point at separate result and log files. Metric history
reads the row plus the structured metric payload; it does not depend on one
hardcoded tracker file.

Minimum metric-specific run fields inside the structured result:
- `metric.artifact_id`
- `metric.definition_id`
- `metric.observed_at`
- `metric.status`
- `metric.value`
- `metric.unit`
- `metric.samples`
- `metric.comparison`

The enclosing execution run still owns:
- `run_id`
- `definition_id`
- `artifact_ids`
- `started_at`
- `finished_at`
- `status`
- references to captured log bodies
- `provenance`

## Command Ownership

- `ddx exec validate <definition-id>` validates the linked metric artifact and
  metric-specific execution contract.
- `ddx exec run <definition-id>` executes the definition and persists an
  immutable execution run containing metric results.
- Metric history is rendered from `ddx exec history` filtered by the governing
  `MET-*` artifact or definition ID.
- Metric comparison and trend reporting are metric-specific read models over
  generic execution history. They may later gain convenience commands, but the
  owning runtime surface is `ddx exec`.

## Behavior

1. Resolve the requested `MET-*` artifact from the document graph.
2. Resolve the active `ddx exec` definition linked to that metric.
3. Execute the configured command in the configured working directory and
   environment.
4. Normalize the raw output into the metric-shaped structured result payload.
5. Persist the execution run through the generic execution history mechanism.
6. Derive comparison and trend views from filtered execution runs when
   requested.

The design does not require a central service or database. All state remains
repository-local and file-backed.

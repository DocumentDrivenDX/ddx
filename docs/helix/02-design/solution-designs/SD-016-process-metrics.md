---
ddx:
  id: SD-016
  depends_on:
    - FEAT-016
    - FEAT-004
    - FEAT-006
    - FEAT-010
    - FEAT-014
---
# Solution Design: Process Metrics

## Overview

Process metrics are a read model over existing DDx stores, not a new capture
subsystem. The design joins:

- bead lifecycle rows from `.ddx/beads.jsonl`
- agent session rows from `.ddx/agent-logs/sessions.jsonl`
- native session references and runtime evidence when present

The same logical schema should continue to work if the session collection is
later promoted to the bead-backed `agent-sessions` collection described in
TD-006. The physical file name is an implementation detail; the logical record
shape is what matters.

SD-016 answers the workflow questions FEAT-016 calls out:

- how much did a bead cost
- how much did a feature cost
- how many times did work reopen
- how long did work stay open
- how much rework happened in a window

FEAT-010 remains the operational metric substrate. SD-016 is the process
projection over bead and session facts.

## Scope

- Derive lifecycle, cost, and rework measures from existing bead and session
  records.
- Preserve provider-native session ownership for external harnesses.
- Keep process metrics separate from operational execution metrics.
- Make unknown and partial data explicit instead of guessing.

## Non-Goals

- No duplicate transcript store.
- No provider quota model.
- No workflow orchestration or staging policy.
- No requirement for a dedicated process-metrics database.

## Source Records

### Bead lifecycle record

The bead tracker provides the lifecycle anchor. Relevant fields are:

- `id`
- `status`
- `created_at`
- `updated_at`
- `labels`
- `owner`
- `dependencies`
- `Extra["spec-id"]`
- `Extra["events"]`
- `Extra["closing_commit_sha"]`
- `Extra["session_id"]`

The process-metrics projection must treat `updated_at` as mutable metadata, not
as the canonical close timestamp. A stable lifecycle fact is derived from the
earliest observed close transition instead.

### Agent session record

The session log provides cost and effort attribution. Relevant fields are:

- `id`
- `timestamp`
- `harness`
- `model`
- `duration_ms`
- `exit_code`
- `tokens`
- `input_tokens`
- `output_tokens`
- `cost_usd`
- `correlation.bead_id`
- `correlation.workflow`
- `native_session_id` or equivalent native reference
- `native_log_ref` or path
- `base_rev`
- `result_rev`

The session row is the atomic fact for cost and effort. A session without a
bead correlation can still contribute to harness-level usage, but not to bead-
or feature-level process metrics.

## Canonical Joins

Primary join key:

- `session.correlation.bead_id` -> `bead.id`

Secondary provenance keys:

- `bead.Extra["session_id"]` for the closing session that completed the bead
- `bead.Extra["closing_commit_sha"]` for audit and replay provenance
- `session.native_session_id` for native provider correlation
- `session.base_rev` and `session.result_rev` for execute-bead runs

Join precedence:

1. If `correlation.bead_id` exists, the session belongs to that bead.
2. If a bead records `session_id` on close, that session is the closing
   session for lifecycle provenance.
3. `closing_commit_sha` and native session references are audit metadata, not
   primary join keys.
4. A session with no bead correlation may still count in harness-level usage,
   but it is excluded from bead- and spec-level rollups.

## Derived Facts

### BeadLifecycleFact

`BeadLifecycleFact` is the normalized lifecycle view for one bead.

Minimum fields:

- `bead_id`
- `spec_id`
- `created_at`
- `first_closed_at`
- `last_closed_at`
- `status`
- `cycle_time_ms`
- `reopen_count`
- `revision_count`
- `time_in_open_ms`
- `time_in_in_progress_ms`
- `time_in_closed_ms`
- `provenance`

Canonical close-time rules:

1. Prefer the first explicit close event in `Extra["events"]`.
2. Else prefer the earliest tracker version that changes `status` to `closed`.
3. Else, if the row records only close commit provenance, use the commit time
   as a fallback.
4. Else mark `first_closed_at` unknown.

`last_closed_at` is the final observed close transition when a bead is reopened
and closed again. If the bead has never reopened, `last_closed_at` equals
`first_closed_at`.

`cycle_time_ms` is `first_closed_at - created_at`.

`reopen_count` is the number of closed-to-open or closed-to-in_progress
transitions after the first close. If no history is available to prove a reopen
did not happen, the value is unknown rather than inferred as zero.

`revision_count` counts distinct post-close tracker revisions that change
workflow content or evidence. Pure timestamp churn does not count. If the
history needed to establish the count is missing, the value is unknown.

### SessionUsageFact

`SessionUsageFact` is the normalized effort and cost view for one session.

Minimum fields:

- `session_id`
- `bead_id`
- `spec_id`
- `harness`
- `model`
- `observed_at`
- `duration_ms`
- `input_tokens`
- `output_tokens`
- `total_tokens`
- `cost_usd`
- `cost_state`
- `base_rev`
- `result_rev`
- `provenance`

`cost_state` is one of:

- `known` - the session row reported `cost_usd`
- `estimated` - DDx derived cost from token counts and model pricing
- `unknown` - no trustworthy cost value exists

Known-zero values are valid. A local harness can report `0` with
`cost_state=known`; that is not the same as unknown.

### ProcessMetricRollup

`ProcessMetricRollup` is the consumer-facing aggregation shape.

Minimum fields:

- `scope`
- `window`
- `bead_ids`
- `spec_ids`
- `counts`
- `lifecycle`
- `usage`
- `coverage`
- `provenance`

The rollup is a projection, not a new source of truth. If a future backend
wants to cache the projection, it must still be regenerable from the source
records above.

## Metric Definitions

### Bead lifecycle cost

Bead lifecycle cost is the sum of all correlated session costs for the bead
within the requested window.

Cost precedence:

1. Use `session.cost_usd` when provided.
2. Else estimate from known token counts and model pricing from FEAT-014.
3. Else mark the cost unknown.

If a bead has no correlated sessions, lifecycle cost is unknown, not zero.

### Feature cost

Feature cost is bead lifecycle cost grouped by `spec-id`.

- The feature key is the governing artifact ID stored in `Extra["spec-id"]`.
- If a bead has no `spec-id`, it contributes to bead-level summaries but not
  feature-level cost rollups.
- Feature totals sum the already-attributed bead totals; they do not re-scan
  sessions independently.

### Cycle time

Cycle time is the elapsed duration from `created_at` to `first_closed_at`.

- Reopened beads keep the original creation time.
- A reopened bead gets a new `last_closed_at`, but the original `cycle_time_ms`
  remains anchored to the first close.
- If `first_closed_at` cannot be established, cycle time is unknown.

### Reopen rate

Reopen rate is:

`reopened_beads / closed_beads`

within the requested window.

- A bead counts as reopened when it has at least one close-to-open or
  close-to-in_progress transition after its first close.
- The numerator counts distinct beads, not transition events.
- The denominator counts distinct beads whose first close falls within the
  window unless the caller asks for a different bucket.

### Revision count

Revision count measures the amount of post-close churn on a bead.

- A revision is a distinct tracker change after first close that alters
  lifecycle or workflow content.
- Evidence-only updates may count if they materially change the bead record
  viewed as workflow evidence.
- Automatic timestamp-only writes do not count.
- Missing history yields unknown, not a guessed zero.

### Time in status

Time in status is the sum of time spent in `open`, `in_progress`, and `closed`
intervals.

- The source of truth is the status transition timeline, not the current
  `updated_at` field.
- If a transition timestamp is absent, that interval is omitted from the
  total rather than fabricated.

## Windowing Rules

Each metric uses the time axis that best matches its source:

- session cost and effort rollups filter on `session.timestamp`
- lifecycle and reopen measures filter on the bead close timeline
- feature cost groups by `spec-id` after bead-level attribution

`--since` should always mean "include only facts observed on or after the
cutoff" for the relevant fact type.

The projection may return partial results when different sources have different
freshness. Partial is better than silent coercion.

## Output Semantics

Process-metric consumers should render three distinct states:

- `known` - the value is directly derived from stored facts
- `estimated` - the value is derived from a model or fallback estimate
- `unknown` - the value cannot be derived from trusted data

This matters most for cost:

- `0` means known zero cost
- `estimated` means the value came from FEAT-014 pricing logic
- `unknown` means no trustworthy cost path exists

The same distinction applies to lifecycle measures that depend on missing
history.

## Command Boundary

`ddx metrics` is the consumer of this projection.

- `ddx metrics summary` reads the rollup view.
- `ddx metrics cost` reads bead- and feature-level cost attribution.
- `ddx metrics cycle-time` reads lifecycle timing facts.
- `ddx metrics rework` reads reopen and revision facts.

`ddx exec` remains the operational metric surface. SD-016 does not move
process metrics into `ddx exec`, and it does not redefine operational
measurements as workflow metrics.

## Provenance

Every projected row should carry enough provenance to explain where the value
came from:

- source bead IDs
- source session IDs
- source timestamps
- native provider references when present
- close provenance when the lifecycle fact came from a close event or commit

Provenance is required for auditability and replay, but it is not part of the
metric arithmetic itself.

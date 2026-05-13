# Metric Definition Prompt

Create one reusable metric definition.

## Purpose

Metric Definition is the **single-measurement contract**. Its unique job is to
define exactly what is measured, how to collect it, what unit it uses, whether
higher or lower is better, what tolerance applies, and how dashboards,
ratchets, experiments, or monitoring should interpret it.

It is not a dashboard, alert policy, or improvement backlog item. Those artifacts
consume metric definitions.

## Reference Anchors

Use this local resource summary as grounding:

- `docs/resources/google-sre-monitoring-distributed-systems.md` grounds metric
  definitions as precise quantitative signals with clear interpretation.

## Focus
Define the metric as the authoritative source for ratchets, dashboards, experiments, and monitoring.

Keep the definition minimal: required fields are `name`, `description`, `unit`, `direction`, and `command`. Add `output_pattern`, `tolerance`, and `labels` only when needed.

The command must be deterministic, repeatable, and free of side effects or external service dependencies. Prefer `METRIC <name>=<value>` output unless an `output_pattern` is required.

## Boundary Test

| If you are writing... | Put it in... |
|---|---|
| One metric's unit, command, direction, tolerance, and labels | Metric Definition |
| A view comparing multiple metrics over time | Metrics Dashboard |
| A decision about what to improve next | Improvement Backlog |
| Production alerting or runbook behavior | Monitoring Setup / Runbook |

## Completion Criteria
- All required fields are populated.
- The command is deterministic and repeatable.
- The filename matches the `name` field.

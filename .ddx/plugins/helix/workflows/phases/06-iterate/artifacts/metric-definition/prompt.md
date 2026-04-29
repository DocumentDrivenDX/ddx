# Metric Definition Prompt

Create one reusable metric definition.

## Focus
Define the metric as the authoritative source for ratchets, dashboards, experiments, and monitoring.

Keep the definition minimal: required fields are `name`, `description`, `unit`, `direction`, and `command`. Add `output_pattern`, `tolerance`, and `labels` only when needed.

The command must be deterministic, repeatable, and free of side effects or external service dependencies. Prefer `METRIC <name>=<value>` output unless an `output_pattern` is required.

## Completion Criteria
- All required fields are populated.
- The command is deterministic and repeatable.
- The filename matches the `name` field.

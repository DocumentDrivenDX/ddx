# Autoresearch Session: {{goal}}

## Objective

{{objective_description}}

## Metrics

- **Primary:** {{metric_name}} ({{metric_unit}}, {{lower_or_higher}} is better)
- **Definition:** `docs/helix/06-iterate/metrics/{{metric_name}}.yaml`
- **Secondary:** {{secondary_metrics_or_none}}

## How to Run

```bash
./autoresearch.sh
```

Outputs `METRIC {{metric_name}}={{value}}` lines to stdout.

## Correctness Check

```bash
{{test_command}}
```

Tests must pass; failures discard the iteration.

## Files in Scope

| File | Notes |
|------|-------|
| `{{file_path}}` | {{brief_description}} |

## Off Limits

- {{off_limits_path_or_pattern}}

## Constraints

- Tests must pass after every edit; failures discard the iteration
- No new dependencies without approval
- Only modify files listed in "Files in Scope"
- {{additional_constraint}}

## What's Been Tried

_Updated every 5 iterations._

### Iterations 1-5

{{summary_of_experiments}}

### Key Patterns

{{patterns_observed}}
